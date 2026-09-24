package inbox

import (
	"context"
	"fmt"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"
	"mu/internal/persist"
	"mu/internal/service"
	"mu/internal/thread"
	"os"
	"strings"
	"time"
)

// Load owns the incoming-message projection, independent of agent execution.
// The checkpoint follows Flush, so a restart cannot acknowledge an unsaved index.
func Load() {
	go func() {
		reconcileSources()
		for {
			err := event.Consume(context.Background(), "inbox", []string{"mail.received", event.ChatRecorded, "sms.created", "sms.updated", "sms.deleted", "mail.deleted"}, indexArrival)
			app.Log("inbox", "arrival consumer stopped: %v", err)
			time.Sleep(5 * time.Second)
		}
	}()
}

func indexArrival(e event.Record) error { return projectArrival(e, true) }

func projectArrival(e event.Record, flush bool) error {
	if strings.HasSuffix(e.Type, ".deleted") {
		thread.InvalidateSource(e.Account, e.Service, e.Resource)
		return thread.Flush()
	}
	if e.Service == "sms" && e.Data["collection"] != "messages" {
		return nil
	}
	source := thread.Source{Service: e.Service, ID: e.Resource, Version: e.Version}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m, err := readSource(ctx, e.Account, source)
	if err != nil {
		return err
	}
	return cacheArrival(e, m, flush)
}

func cacheArrival(e event.Record, m *service.SourceMessage, flush bool) error {
	source := thread.Source{Service: e.Service, ID: e.Resource, Version: e.Version}
	if m == nil {
		return nil // Deleted before indexing; do not block later arrivals.
	}
	if e.Service == "sms" && m.Direction != "in" {
		return nil
	}
	client := e.Service
	if m.Channel == "whatsapp" {
		client = thread.WhatsAppClient
	}
	ref := m.Ref
	if ref == "" {
		ref = e.Service + ":" + e.Resource
	}
	th := thread.ByRef(e.Account, ref)
	if th == nil {
		th = thread.OpenAt(e.Account, client, m.Conversation, m.At)
	}
	if th == nil {
		return fmt.Errorf("could not index arrival")
	}
	subject := m.Subject
	if subject == "" {
		subject = m.From
	}
	thread.Name(e.Account, th.ID, mailSubject(subject))
	text := m.Text
	if text == "" {
		text = "(message)"
	}
	from := m.From
	if m.Direction == "out" {
		from = ""
	}
	thread.Add(thread.Message{Account: e.Account, Thread: th.ID, Role: thread.RolePerson, Text: text, Ref: ref, From: from, To: m.To, At: m.At, Source: &source, SourceHTML: m.HTML})
	if flush {
		return thread.Flush()
	}
	return nil
}

func readSource(ctx context.Context, account string, source thread.Source) (*service.SourceMessage, error) {
	var rsp service.SourceResponse
	err := service.Call(service.WithAccount(ctx, account), source.Service, "Server.Source", &service.SourceRequest{ID: source.ID}, &rsp)
	return rsp.Item, err
}

// Rebuild the derived view for older installations once at startup. It runs in
// the background; rendering continues from the existing index during migration.
func reconcileSources() {
	const marker = "inbox/projection-v1.json"
	if _, err := persist.Read(marker); err == nil {
		return
	} else if !os.IsNotExist(err) {
		app.Log("inbox", "could not read migration state: %v", err)
		return
	}
	complete := true
	for _, account := range auth.AllAccounts() {
		for _, name := range []string{"mail", "chat", "sms"} {
			for offset := 0; ; {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				var rsp service.SourcesResponse
				err := service.Call(service.WithAccount(ctx, account.ID), name, "Server.Sources", &service.SourcesRequest{Offset: offset, Limit: 100}, &rsp)
				cancel()
				if err != nil {
					complete = false
					app.Log("inbox", "source reconciliation %s failed: %v", name, err)
					break
				}
				for _, id := range rsp.IDs {
					if err := projectArrival(event.Record{Service: name, Account: account.ID, Resource: id, Data: map[string]interface{}{"collection": "messages"}}, false); err != nil {
						complete = false
						app.Log("inbox", "source reconciliation failed: %v", err)
					}
				}
				if err := thread.Flush(); err != nil {
					complete = false
					app.Log("inbox", "could not save source index: %v", err)
					break
				}
				if rsp.Next <= offset {
					break
				}
				offset = rsp.Next
			}
		}
	}
	if complete {
		if err := persist.Write(marker, []byte(`true`)); err != nil {
			app.Log("inbox", "could not save migration state: %v", err)
		}
	}

}

package server

import (
	"context"
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"
	"mu/internal/origin"
	"mu/internal/persist"
	"mu/internal/push"
	"mu/internal/service"
	"mu/service/events"
	"mu/service/mail"
	"os"
	"strings"
	"time"
)

func inviteSchedule(e *events.Event) {

	domain := mail.ConfiguredDomain()
	if domain == "" || domain == "localhost" {
		return
	}
	acc, err := auth.GetAccount(e.Owner)
	if err != nil || acc.Email == "" || !acc.EmailVerified {
		return
	}
	when := e.When.Local().Format("Mon 2 Jan 2006, 15:04 MST")
	body := fmt.Sprintf(`<p>Scheduled with Micro:</p><p class="status-icon"><strong>%s</strong><br>%s</p>`,
		html.EscapeString(e.Title), html.EscapeString(when))
	if e.Note != "" {
		body += `<p>` + html.EscapeString(e.Note) + `</p>`
	}
	if e.Prompt != "" {
		body += `<p>At this time, Micro will attempt the following instruction and send the outcome to your inbox:</p><p>` + html.EscapeString(e.Prompt) + `</p>`
	} else {
		body += `<p>At this time, Micro will send a reminder to your subscribed devices. This does not schedule agent work.</p>`
	}
	if e.Repeat != "" {
		body += `<p>Repeats: ` + html.EscapeString(e.Repeat) + `</p>`
	}
	body += `<p><a href="` + html.EscapeString(strings.TrimRight(origin.Self(), "/")+"/events?id="+e.ID) + `">View schedule</a></p>`
	body += `<p class="text-muted text-sm">You can add a copy to your calendar using the attached invite. Changes in that calendar do not change Micro's schedule.</p>`
	ics := events.ICS(e, acc.Email)
	if _, err := mail.SendCalendarInvite("Micro", "no-reply@"+domain, acc.Email, "Event: "+e.Title, body, ics); err != nil {
		app.Log("events", "calendar invite to %s failed: %v", acc.Email, err)
	}

}

func watchScheduleEvents() {
	go func() {
		for {
			err := event.Consume(context.Background(), "schedule-notifications", []string{event.ScheduleDue, "events.changed"}, func(e event.Record) error {
				// External delivery cannot be rolled back. Reserve it before
				// sending so a crash/replay cannot send the same invite twice.
				key := "work/notifications/" + e.ID + ".json"
				if _, err := persist.Read(key); err == nil {
					return nil
				} else if !os.IsNotExist(err) {
					return err
				}
				if e.Type == event.ScheduleDue {
					if e.Data["kind"] == "brief" {
						return nil
					}
					title, _ := e.Data["title"].(string)
					note, _ := e.Data["note"].(string)
					if err := persist.Write(key, []byte(`"attempted"`)); err != nil {
						return err
					}
					push.Send(e.Account, push.Notification{Title: "⏰ " + title, Body: note, URL: "/events"})
					return nil
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				var rsp events.ReadResponse
				if err := service.Call(service.WithAccount(ctx, e.Account), "events", "Server.Read", &events.ReadRequest{ID: e.Resource}, &rsp); err != nil {
					return err
				}
				if rsp.Item != nil && fmt.Sprint(rsp.Item.Sequence) == e.Version {
					if err := persist.Write(key, []byte(`"attempted"`)); err != nil {
						return err
					}
					inviteSchedule(rsp.Item)
				}
				return nil
			})
			app.Log("events", "notification subscriber stopped: %v", err)
			time.Sleep(5 * time.Second)
		}
	}()
}

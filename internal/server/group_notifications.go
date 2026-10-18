package server

import (
	"context"
	"mu/internal/app"
	"mu/internal/event"
	"mu/internal/group"
	"mu/internal/persist"
	"mu/internal/push"
	"mu/internal/service"
	"mu/service/chat"
	"os"
	"time"
)

func watchGroupMessages() {
	go func() {
		for {
			err := event.Consume(context.Background(), "group-notifications", []string{"chat.group.posted"}, func(e event.Record) error {
				key := "groups/notifications/" + e.ID + ".json"
				if _, err := persist.Read(key); err == nil {
					return nil
				} else if !os.IsNotExist(err) {
					return err
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				var rsp service.SourceResponse
				if err := (chat.Server{}).Source(service.WithAccount(ctx, e.Account), &service.SourceRequest{ID: e.Resource}, &rsp); err != nil {
					return err
				}
				if rsp.Item == nil {
					return nil
				}
				m := rsp.Item
				g, _, ok := group.ConversationFor(m.Conversation)
				if !ok || !group.Member(g.ID, e.Account) {
					return nil
				}
				if err := persist.Write(key, []byte(`"attempted"`)); err != nil {
					return err
				}
				for member := range g.Members {
					if g.Muted[member] || member == e.Account || chat.Watching(m.Conversation, member) || !group.Member(g.ID, member) {
						continue
					}
					push.Send(member, push.Notification{Title: g.Name, Body: "New message from @" + e.Account, URL: "/chat?id=" + m.Conversation, Tag: "group-" + g.ID})
				}
				return nil
			})
			app.Log("groups", "message notifications stopped: %v", err)
			time.Sleep(5 * time.Second)
		}
	}()
}

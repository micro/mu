package agent

import (
	"github.com/google/uuid"
	"mu/inbox"
	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"
	"mu/internal/thread"
	"mu/internal/usage"
	"mu/service/mail"
	"strings"
	"time"
)

// ScheduledResult identifies the owned output being released to Inbox.
type ScheduledResult struct {
	Account, ID, EventID, Title, Agent string
	Due                                time.Time
}

func DeliverScheduled(r ScheduledResult, answer string, err error) error {
	if e := Moment(r.Account); e != nil && e.ID == r.ID && (e.Paused || r.Due.IsZero() || time.Since(r.Due) > time.Hour) {
		return nil
	}
	body := strings.TrimSpace(answer)
	if err != nil {
		app.Log("work", "standing instruction %q failed for %s: %v", r.Title, r.Account, err)
		body = "This scheduled task failed: " + ai.FailureMessage(err)
	}
	if body == "" {
		return nil
	}
	acc, accErr := auth.GetAccount(r.Account)
	if accErr != nil {
		return accErr
	}
	body, tag, topic := ScheduledDelivery(r.Account, r.ID, body)
	messageID := "<" + uuid.NewString() + "@" + mail.ConfiguredDomain() + ">"
	if r.EventID != "" {
		messageID = "<schedule-" + r.EventID + "@" + mail.ConfiguredDomain() + ">"
	}
	sender := NameOf(r.Account, r.Agent)
	if sender == "" {
		sender = DefaultName()
	}
	if sender == "" {
		sender = "Micro"
	}
	if e := Research(r.Account); e != nil && e.ID == r.ID {
		r.Title = "Evening Reading: " + e.Prompt
	}
	delivery := mail.Delivery{
		Markdown: true,
		From:     sender, FromID: "agent@" + mail.ConfiguredDomain(),
		To: acc.Name, ToID: acc.ID, Tag: tag,
		Subject: r.Title, Body: body, MessageID: messageID,
	}
	if sendErr := mail.SendMessageTo(delivery); sendErr != nil {
		app.Log("agent", "delivering scheduled result for %s: %v", r.Account, sendErr)
		usage.RecordActivity(usage.Activity{Surface: "agent", Operation: "scheduled delivery", Account: r.Account, Outcome: "delivery failed"})
		return sendErr
	}
	if topic != "" && err == nil {
		link := inbox.MailURL(mail.InboundMail{Owner: acc.ID, From: delivery.FromID, FromName: delivery.From, To: acc.ID + "+" + tag + "@" + mail.ConfiguredDomain(), Subject: r.Title, Body: delivery.Body, MessageID: messageID, Tag: tag})
		if tag == "checkin" {
			if source := thread.ByRef(acc.ID, messageID); source != nil {
				if _, mergeErr := consolidateCheckin(acc.ID, source); mergeErr != nil {
					app.Log("agent", "consolidate delivered checkin: %v", mergeErr)
				}
			}
		}
		link = ScheduledNotificationURL(topic, link)
		event.Announce(topic, strings.TrimSpace(answer), link, r.Account)
	}
	return nil
}

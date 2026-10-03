package agent

import (
	"mu/internal/origin"
	"strings"
)

// ScheduledDelivery describes feature-specific presentation. Work owns transport.
func ScheduledDelivery(owner, id, body string) (text, tag, topic string) {
	if e := Checkin(owner); e != nil && e.ID == id {
		return body, "checkin", "checkin"
	}
	if e := Moment(owner); e != nil && e.ID == id {
		return body, "moment", "moment"
	}
	if e := Brief(owner); e != nil && e.ID == id {
		return body + "\n\n---\n[Manage your brief and plan](" + origin.Self() + "/agents?view=scheduled).", "brief", "brief"
	}
	return body, "scheduled", ""
}

func ScheduledNotificationURL(topic, inboxURL string) string {
	if topic == "checkin" {
		return strings.Replace(inboxURL, "/inbox?id=", "/checkin?id=", 1)
	}
	return inboxURL
}

package chat

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// MUC archives use the group's own transcript, never a member's private archive.
func (s *session) mucArchive(st stanza, room string, raw []byte) {
	if st.Type == "get" {
		s.mucResult(st, `<query xmlns='`+nsMAM+`'><x xmlns='jabber:x:data' type='form'><field var='FORM_TYPE' type='hidden'><value>`+nsMAM+`</value></field><field var='start' type='text-single'/><field var='end' type='text-single'/></x></query>`)
		return
	}
	if st.Type != "set" {
		s.mucError(st, "iq", "bad-request", "Invalid archive request")
		return
	}
	var q archiveQuery
	if xml.Unmarshal(raw, &q) != nil {
		s.mucError(st, "iq", "bad-request", "Invalid archive query")
		return
	}
	var start, end time.Time
	for _, f := range q.Form.Fields {
		if f.Var == "FORM_TYPE" {
			continue
		}
		if len(f.Values) != 1 || (f.Var != "start" && f.Var != "end") {
			s.mucError(st, "iq", "bad-request", "Unsupported group archive filter")
			return
		}
		at, err := time.Parse(time.RFC3339Nano, f.Values[0])
		if err != nil {
			s.mucError(st, "iq", "bad-request", "Invalid archive date")
			return
		}
		if f.Var == "start" {
			start = at
		} else {
			end = at
		}
	}
	if !start.IsZero() && !end.IsZero() && end.Before(start) {
		s.mucError(st, "iq", "bad-request", "Invalid archive interval")
		return
	}
	roomsMutex.RLock()
	r := rooms[room]
	roomsMutex.RUnlock()
	var messages []RoomMessage
	if r != nil {
		r.mutex.RLock()
		messages = append(messages, r.Messages...)
		r.mutex.RUnlock()
	} else {
		messages = loadRoomMessages(room)
	}
	filtered := make([]RoomMessage, 0, len(messages))
	for _, m := range messages {
		if !m.System && (start.IsZero() || !m.Timestamp.Before(start)) && (end.IsZero() || !m.Timestamp.After(end)) {
			filtered = append(filtered, m)
		}
	}
	low, high := 0, len(filtered)
	if q.Set.Before != nil && q.Set.After != "" {
		s.mucError(st, "iq", "bad-request", "Choose one archive cursor")
		return
	}
	cursor := ""
	if q.Set.Before != nil {
		cursor = q.Set.Before.ID
	} else {
		cursor = q.Set.After
	}
	if cursor != "" {
		found := -1
		for i, m := range filtered {
			if m.ID == cursor {
				found = i
				break
			}
		}
		if found < 0 {
			s.mucError(st, "iq", "item-not-found", "Archive cursor not found")
			return
		}
		if q.Set.Before != nil {
			high = found
		} else {
			low = found + 1
		}
	}
	limit := q.page()
	complete := true
	if high-low > limit {
		complete = false
		if q.Set.Before != nil {
			low = high - limit
		} else {
			high = low + limit
		}
	}
	if q.Set.Max == "0" {
		high = low
	}
	page := filtered[low:high]
	for _, m := range page {
		if !Member(room, s.acc.ID) {
			s.mucError(st, "iq", "forbidden", "Group membership ended")
			return
		}
		body := mucMessageXML(room, m, s, false)
		s.send(`<message from='%s' to='%s'><result xmlns='%s' queryid='%s' id='%s'><forwarded xmlns='%s'><delay xmlns='%s' stamp='%s'/>%s</forwarded></result></message>`, xmlAttr(mucJID(room)), xmlAttr(s.jid()), nsMAM, xmlAttr(q.QueryID), xmlAttr(m.ID), nsForward, nsDelay, m.Timestamp.UTC().Format(time.RFC3339Nano), strings.Replace(body, "<message ", "<message xmlns='jabber:client' ", 1))
	}
	bounds := ""
	if len(page) > 0 {
		bounds = fmt.Sprintf(`<first index='%d'>%s</first><last>%s</last>`, low, xmlAttr(page[0].ID), xmlAttr(page[len(page)-1].ID))
	}
	s.mucResult(st, fmt.Sprintf(`<fin xmlns='%s' complete='%t' stable='true'><set xmlns='%s'>%s<count>%d</count></set></fin>`, nsMAM, complete, nsRSM, bounds, len(filtered)))
}

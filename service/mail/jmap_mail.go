package mail

import (
	"encoding/base64"
	"fmt"
	netmail "net/mail"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type jmapView struct {
	messages   []*Message
	boxes      []jmapObject
	membership map[string]map[string]bool
	state      string
}

func jmapSnapshot(owner string) jmapView {
	v := jmapView{membership: map[string]map[string]bool{}, boxes: []jmapObject{}, messages: []*Message{}}
	seen := map[string]bool{}
	for i, folder := range imapFolders(owner) {
		list, _ := imapFolder(owner, folder)
		id := jmapID(folder)
		unread := 0
		threads := map[string]bool{}
		unreadThreads := map[string]bool{}
		mutex.RLock()
		for _, m := range list {
			cp := *m
			if !seen[m.ID] {
				v.messages = append(v.messages, &cp)
				seen[m.ID] = true
			}
			if v.membership[m.ID] == nil {
				v.membership[m.ID] = map[string]bool{}
			}
			v.membership[m.ID][id] = true
			tid := jmapThread(m)
			threads[tid] = true
			if !m.Read {
				unread++
				unreadThreads[tid] = true
			}
		}
		mutex.RUnlock()
		var role any
		switch folder {
		case imapInbox:
			role = "inbox"
		case imapSent:
			role = "sent"
		case imapJunk:
			role = "junk"
		}
		v.boxes = append(v.boxes, jmapObject{"id": id, "name": folder, "parentId": nil, "role": role, "sortOrder": i, "totalEmails": len(list), "unreadEmails": unread, "totalThreads": len(threads), "unreadThreads": len(unreadThreads), "isSubscribed": true, "myRights": jmapObject{"mayReadItems": true, "mayAddItems": false, "mayRemoveItems": false, "maySetSeen": folder != imapSent, "maySetKeywords": false, "mayCreateChild": false, "mayRename": false, "mayDelete": false, "maySubmit": false}})
	}
	sort.Slice(v.messages, func(i, j int) bool {
		a, b := v.messages[i], v.messages[j]
		if a.CreatedAt.Equal(b.CreatedAt) {
			return a.ID < b.ID
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})
	v.state = jmapHash([]any{v.messages, v.membership})
	return v
}
func jmapThread(m *Message) string {
	if m.ThreadID != "" {
		return jmapID(m.ThreadID)
	}
	return jmapID(m.ID)
}
func jmapAddresses(name, address string) []jmapObject {
	if address == "" {
		return []jmapObject{}
	}
	if !strings.Contains(address, "@") {
		address = EmailForUser(address, ConfiguredDomain())
	}
	if parsed, err := netmail.ParseAddress(address); err == nil {
		if parsed.Name != "" {
			name = parsed.Name
		}
		address = parsed.Address
	}
	return []jmapObject{{"name": name, "email": address}}
}
func jmapEmail(m *Message, boxes map[string]bool, args jmapObject) jmapObject {
	keywords := map[string]bool{}
	if m.Read {
		keywords["$seen"] = true
	}
	text := m.Body
	bodyType := "text/plain"
	textBody := []any{}
	htmlBody := []any{}
	if imapLooksHTML(m.Body) || m.Markdown {
		bodyType = "text/html"
		text = Rendered(m)
	}
	part := jmapObject{"partId": "1", "blobId": jmapID("body:" + m.ID), "size": len(text), "name": nil, "type": bodyType, "charset": "utf-8", "disposition": nil, "cid": nil, "language": nil, "location": nil}
	if bodyType == "text/html" {
		htmlBody = append(htmlBody, part)
	} else {
		textBody = append(textBody, part)
	}
	attachments := []any{}
	if m.Attachment != "" {
		b, _ := base64.StdEncoding.DecodeString(m.Attachment)
		attachments = append(attachments, jmapObject{"partId": "2", "blobId": jmapID("attachment:" + m.ID), "size": len(b), "name": m.AttachmentName, "type": m.AttachmentType, "disposition": "attachment", "charset": nil, "cid": nil, "language": nil, "location": nil})
	}
	truncated := false
	if n, ok := args["maxBodyValueBytes"].(float64); ok && n >= 0 && n < float64(len(text)) {
		text = text[:int(n)]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
		truncated = true
	}
	preview := record(m, false).Snippet
	return jmapObject{"id": jmapID(m.ID), "blobId": jmapID("message:" + m.ID), "threadId": jmapThread(m), "mailboxIds": boxes, "keywords": keywords, "size": len(imapRender(m)), "receivedAt": m.CreatedAt.UTC().Format(time.RFC3339), "sentAt": m.CreatedAt.UTC().Format(time.RFC3339), "messageId": []string{strings.Trim(m.MessageID, "<>")}, "inReplyTo": []string{}, "references": []string{}, "sender": nil, "from": jmapAddresses(m.From, m.FromID), "to": jmapAddresses(m.To, m.ToID), "cc": nil, "bcc": nil, "replyTo": nil, "subject": decodeMIMEHeader(m.Subject), "hasAttachment": m.Attachment != "", "preview": preview, "textBody": textBody, "htmlBody": htmlBody, "attachments": attachments, "bodyValues": jmapObject{"1": jmapObject{"value": text, "isEncodingProblem": false, "isTruncated": truncated}}, "bodyStructure": part}
}
func jmapMethod(owner, name string, args jmapObject) jmapObject {
	v := jmapSnapshot(owner)
	switch name {
	case "Mailbox/get", "Email/get", "Thread/get":
		objects := []jmapObject{}
		switch name {
		case "Mailbox/get":
			objects = v.boxes
		case "Email/get":
			ids, valid := jmapStrings(args["ids"])
			if !valid || len(ids) > 500 {
				return jmapError("invalidArguments")
			}
			wanted := map[string]bool{}
			for _, id := range ids {
				wanted[id] = true
			}
			if args["ids"] == nil && len(v.messages) > 500 {
				return jmapError("requestTooLarge")
			}
			total := 0
			for _, m := range v.messages {
				if args["ids"] != nil && !wanted[jmapID(m.ID)] {
					continue
				}
				total += len(m.Body)
				if total > 16<<20 {
					return jmapError("requestTooLarge")
				}
				objects = append(objects, jmapEmail(m, v.membership[m.ID], args))
			}
		case "Thread/get":
			threads := map[string][]any{}
			for _, m := range v.messages {
				tid := jmapThread(m)
				threads[tid] = append(threads[tid], jmapID(m.ID))
			}
			keys := []string{}
			for id := range threads {
				keys = append(keys, id)
			}
			sort.Strings(keys)
			for _, id := range keys {
				objects = append(objects, jmapObject{"id": id, "emailIds": threads[id]})
			}
		}
		ids, valid := jmapStrings(args["ids"])
		if !valid || len(ids) > 500 {
			return jmapError("invalidArguments")
		}
		byID := map[string]jmapObject{}
		for _, obj := range objects {
			byID[obj["id"].(string)] = obj
		}
		if args["ids"] == nil {
			if len(objects) > 500 {
				return jmapError("requestTooLarge")
			}
			for _, obj := range objects {
				ids = append(ids, obj["id"].(string))
			}
		}
		props, valid := jmapStrings(args["properties"])
		if !valid {
			return jmapError("invalidArguments")
		}
		list := []any{}
		missing := []any{}
		for _, id := range ids {
			obj := byID[id]
			if obj == nil {
				missing = append(missing, id)
				continue
			}
			if args["properties"] != nil {
				filtered := jmapObject{"id": id}
				for _, p := range props {
					if val, ok := obj[p]; ok {
						filtered[p] = val
					} else {
						return jmapError("invalidArguments")
					}
				}
				obj = filtered
			}
			list = append(list, obj)
		}
		return jmapObject{"accountId": owner, "state": v.state, "list": list, "notFound": missing}
	case "Email/query":
		return jmapQuery(owner, v, args)
	case "Email/changes", "Mailbox/changes", "Thread/changes":
		if args["sinceState"] != v.state {
			return jmapError("cannotCalculateChanges")
		}
		return jmapObject{"accountId": owner, "oldState": v.state, "newState": v.state, "hasMoreChanges": false, "created": []any{}, "updated": []any{}, "destroyed": []any{}}
	case "Email/queryChanges":
		return jmapError("cannotCalculateChanges")
	case "Email/set":
		return jmapSet(owner, v, args)
	default:
		return jmapError("unknownMethod")
	}
}
func jmapStrings(value any) ([]string, bool) {
	if value == nil {
		return nil, true
	}
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := []string{}
	for _, v := range items {
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}
func jmapQuery(owner string, v jmapView, args jmapObject) jmapObject {
	filter := args["filter"]
	if !jmapValidFilter(filter, 0) {
		return jmapError("unsupportedFilter")
	}
	sorted := append([]*Message{}, v.messages...)
	property, ascending := "receivedAt", false
	if raw, exists := args["sort"]; exists && raw != nil {
		sorters, ok := raw.([]any)
		if !ok || len(sorters) > 1 {
			return jmapError("unsupportedSort")
		}
		if len(sorters) == 1 {
			obj, ok := sorters[0].(map[string]any)
			if !ok {
				return jmapError("unsupportedSort")
			}
			property, _ = obj["property"].(string)
			ascending = true
			if x, ok := obj["isAscending"].(bool); ok {
				ascending = x
			}
			if property != "receivedAt" && property != "subject" {
				return jmapError("unsupportedSort")
			}
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		cmp := a.CreatedAt.Compare(b.CreatedAt)
		if property == "subject" {
			cmp = strings.Compare(strings.ToLower(a.Subject), strings.ToLower(b.Subject))
		}
		if cmp == 0 {
			cmp = strings.Compare(a.ID, b.ID)
		}
		if ascending {
			return cmp < 0
		}
		return cmp > 0
	})
	ids := []any{}
	threads := map[string]bool{}
	collapse, _ := args["collapseThreads"].(bool)
	for _, m := range sorted {
		if !jmapMatches(m, v.membership[m.ID], filter) {
			continue
		}
		tid := jmapThread(m)
		if collapse && threads[tid] {
			continue
		}
		threads[tid] = true
		ids = append(ids, jmapID(m.ID))
	}
	position := 0
	if n, ok := args["position"].(float64); ok {
		if n != float64(int(n)) {
			return jmapError("invalidArguments")
		}
		position = int(n)
		if position < 0 {
			position = len(ids) + position
		}
	}
	if anchor, ok := args["anchor"].(string); ok {
		position = -1
		for i, id := range ids {
			if id == anchor {
				position = i
				break
			}
		}
		if position < 0 {
			return jmapError("anchorNotFound")
		}
		if offset, ok := args["anchorOffset"].(float64); ok {
			position += int(offset)
		}
	}
	if position < 0 {
		position = 0
	}
	if position > len(ids) {
		position = len(ids)
	}
	limit := 500
	if n, ok := args["limit"].(float64); ok {
		if n < 0 || n != float64(int(n)) {
			return jmapError("invalidArguments")
		}
		if n < 500 {
			limit = int(n)
		}
	}
	end := position + limit
	if end > len(ids) {
		end = len(ids)
	}
	return jmapObject{"accountId": owner, "queryState": v.state, "canCalculateChanges": false, "position": position, "ids": ids[position:end], "total": len(ids), "collapseThreads": collapse, "limit": limit}
}
func jmapValidFilter(value any, depth int) bool {
	if value == nil {
		return true
	}
	if depth > 10 {
		return false
	}
	f, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for k, v := range f {
		switch k {
		case "operator":
			if v != "AND" && v != "OR" && v != "NOT" {
				return false
			}
		case "conditions":
			items, ok := v.([]any)
			if !ok {
				return false
			}
			for _, c := range items {
				if !jmapValidFilter(c, depth+1) {
					return false
				}
			}
		case "inMailbox", "text", "from", "to", "subject", "hasKeyword", "notKeyword":
			if _, ok := v.(string); !ok {
				return false
			}
		case "before", "after":
			s, ok := v.(string)
			if !ok {
				return false
			}
			if _, err := time.Parse(time.RFC3339, s); err != nil {
				return false
			}
		case "hasAttachment":
			if _, ok := v.(bool); !ok {
				return false
			}
		default:
			return false
		}
	}
	if f["operator"] != nil {
		_, ok := f["conditions"].([]any)
		return ok && len(f) == 2
	}
	return f["conditions"] == nil
}
func jmapMatches(m *Message, boxes map[string]bool, value any) bool {
	if value == nil {
		return true
	}
	f := value.(map[string]any)
	if op, ok := f["operator"].(string); ok {
		conditions := f["conditions"].([]any)
		matches := 0
		for _, c := range conditions {
			if jmapMatches(m, boxes, c) {
				matches++
			}
		}
		switch op {
		case "OR":
			return matches > 0
		case "NOT":
			return matches == 0
		default:
			return matches == len(conditions)
		}
	}
	for k, v := range f {
		switch k {
		case "inMailbox":
			if !boxes[v.(string)] {
				return false
			}
		case "hasKeyword":
			if v != "$seen" || !m.Read {
				return false
			}
		case "notKeyword":
			if v == "$seen" && m.Read {
				return false
			}
		case "hasAttachment":
			if (m.Attachment != "") != v.(bool) {
				return false
			}
		case "before", "after":
			at, _ := time.Parse(time.RFC3339, v.(string))
			if k == "before" && !m.CreatedAt.Before(at) || k == "after" && m.CreatedAt.Before(at) {
				return false
			}
		default:
			haystack := m.Subject + " " + m.Body + " " + m.FromID + " " + m.ToID
			switch k {
			case "from":
				haystack = m.From + " " + m.FromID
			case "to":
				haystack = m.To + " " + m.ToID
			case "subject":
				haystack = m.Subject
			}
			if !strings.Contains(strings.ToLower(haystack), strings.ToLower(v.(string))) {
				return false
			}
		}
	}
	return true
}
func jmapSet(owner string, v jmapView, args jmapObject) jmapObject {
	if state := args["ifInState"]; state != nil && state != v.state {
		return jmapError("stateMismatch")
	}
	updates, ok := args["update"].(map[string]any)
	if args["update"] != nil && !ok {
		return jmapError("invalidArguments")
	}
	creates, ok := args["create"].(map[string]any)
	if args["create"] != nil && !ok {
		return jmapError("invalidArguments")
	}
	destroy, ok := jmapStrings(args["destroy"])
	if !ok || len(updates)+len(creates)+len(destroy) > 100 {
		return jmapError("invalidArguments")
	}
	updated, notUpdated, notCreated, notDestroyed := jmapObject{}, jmapObject{}, jmapObject{}, jmapObject{}
	destroyed := []any{}
	byID := map[string]*Message{}
	for _, m := range v.messages {
		byID[jmapID(m.ID)] = m
	}
	for id := range creates {
		notCreated[id] = jmapError("forbidden")
	}
	for id, raw := range updates {
		m := byID[id]
		if m == nil {
			notUpdated[id] = jmapError("notFound")
			continue
		}
		patch, ok := raw.(map[string]any)
		if !ok || len(patch) != 1 {
			notUpdated[id] = jmapError("invalidProperties")
			continue
		}
		read := false
		valid := false
		if value, exists := patch["keywords/$seen"]; exists {
			valid = value == true || value == nil
			read = value == true
		}
		if value, exists := patch["keywords"]; exists {
			keys, ok := value.(map[string]any)
			valid = ok && len(keys) == 0
			if ok && len(keys) == 1 && keys["$seen"] == true {
				read = true
				valid = true
			}
		}
		if !valid {
			notUpdated[id] = jmapError("invalidProperties")
			continue
		}
		if !m.Bridged && m.ToID != owner {
			notUpdated[id] = jmapError("forbidden")
			continue
		}
		s := imapSession{account: owner}
		if err := s.setRead(m, read); err != nil {
			notUpdated[id] = jmapError("serverFail")
		} else {
			updated[id] = nil
		}
	}
	for _, id := range destroy {
		m := byID[id]
		if m == nil {
			notDestroyed[id] = jmapError("notFound")
			continue
		}
		var err error
		if m.Bridged {
			err = imapBridgeChange(owner, m, nil, true)
		} else if m.ToID == owner {
			err = DeleteMessage(m.ID, owner)
		} else {
			err = fmt.Errorf("not owned")
		}
		if err != nil {
			notDestroyed[id] = jmapError("forbidden")
		} else {
			destroyed = append(destroyed, id)
		}
	}
	return jmapObject{"accountId": owner, "oldState": v.state, "newState": jmapSnapshot(owner).state, "created": nil, "updated": updated, "destroyed": destroyed, "notCreated": notCreated, "notUpdated": notUpdated, "notDestroyed": notDestroyed}
}

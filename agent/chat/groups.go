package chat

import (
	"context"
	"fmt"
	"mu/agent"
	"mu/internal/ai"
	"mu/internal/group"
	"mu/internal/service"
	"mu/service/chat"
)

func answerGroup(id, room, actor, ref, text string) {
	g, c, ok := group.ConversationFor(room)
	if !ok || g.ID != id || !group.AllowedAI(g, actor) || g.Encrypted || c.Agent == "none" {
		return
	}
	var messages chat.MessagesResponse
	if err := (chat.Server{}).Messages(service.WithAccount(context.Background(), actor), &chat.MessagesRequest{Room: room}, &messages); err != nil {
		return
	}
	history := []agent.QueryMessage{}
	for _, m := range messages.Messages {
		if m.ID == ref {
			break
		}
		role := "user"
		if m.IsAgent {
			role = "assistant"
		}
		history = append(history, agent.QueryMessage{Role: role, Text: m.Author + ": " + m.Content})
	}
	if len(history) > 40 {
		history = history[len(history)-40:]
	}
	scope := group.Scope(id)
	if c.Agent != "" && agent.For(scope, c.Agent) == nil {
		_ = chat.DeliverGroup(room, "micro", "This conversation's assistant is no longer available. Ask a group admin to select another.")
		return
	}
	res, err := agent.Ask(agent.AskRequest{Account: scope, Client: Client, Thread: room, Text: text, MessageRef: ref, From: actor, Agent: c.Agent, History: history, System: fmt.Sprintf("You are the shared assistant for %s. All members can read this conversation. Use only this group's resources. Never claim access to a member's personal data. Address the request from @%s.", g.Name, actor)})
	reply := res.Text
	if err != nil {
		reply = "I couldn't complete that request. " + ai.FailureMessage(err)
	}
	if !group.Member(id, actor) {
		return
	}
	name := "micro"
	if c.Agent != "" {
		if a := agent.For(scope, c.Agent); a != nil {
			name = a.Name
		}
	}
	_ = chat.DeliverGroup(room, name, reply)
}

package agent

import "context"

// Public voice uses a fixed instruction and a narrow service allow-list. Never
// inherit an account's agent prompt, history, device location or private memory.
func publicVoiceOptions(ctx context.Context) QueryOpts {
	return QueryOpts{RunContext: ctx, Public: true, anonymousTools: true, RawReply: true, Tools: []string{"weather"},
		System: "You are Mu in locked-phone public mode. Give a brief spoken answer to public factual questions. You can only look up weather. Ask for a town or city when no place is given; you do not know the user's location. For personal questions, messages, calendar, saved content or any action, ask the person to unlock the phone. Do not claim access to another agent or private context. Never invent current conditions; use weather tools for current weather."}
}

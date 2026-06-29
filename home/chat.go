package home

import "mu/internal/app"

// chatComponent and jsString now delegate to the shared implementation in
// internal/app so the home assistant, the guest landing page and the /agent
// chat surface all render the exact same chat UI.

func chatComponent(guest bool) string {
	return app.ChatComponent(app.ChatConfig{Guest: guest})
}

func jsString(s string) string {
	return app.JSString(s)
}

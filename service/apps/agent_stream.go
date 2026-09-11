package apps

import _ "embed"

// The parent owns the connection; app code never receives session credentials.
//
//go:embed static/agent-stream.js
var appAgentStreamJS string

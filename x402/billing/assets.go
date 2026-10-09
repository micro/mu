package billing

import _ "embed"

// Script is the shared credit checkout client, served in the single app bundle.
//
//go:embed client.js
var Script string

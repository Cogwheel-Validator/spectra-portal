// Package web holds the signer page shared by the MCP inline view and the
// local pathfinder-sign program.
package web

import _ "embed"

// SignerHTML is a single self-contained page: no external scripts, styles or
// network access. It signs with the user's Keplr.
//
//go:embed signer.html
var SignerHTML string

// Package web embeds the built Svelte SPA (web/dist) into the binary so every
// node can serve it at "/" (§10). The embed directive lives here, not in
// internal/api, because go:embed cannot reference parent directories; the API
// piece takes DistFS via its config (DECISIONS.md D15). web/dist/index.html is
// a committed placeholder so the embed always compiles before the UI is built.
package web

import "embed"

//go:embed all:dist
var distFS embed.FS

// DistFS is the embedded web/dist filesystem (rooted at "dist").
var DistFS = distFS

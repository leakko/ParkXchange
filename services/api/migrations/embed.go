// Package migrations embeds the SQL schema migrations into the binary.
//
// Embedding them means the same artefact that serves traffic can also migrate
// the database it is about to serve, so a deployment needs no init container,
// no separate migration image, and no chance of the two drifting apart.
package migrations

import "embed"

// FS holds every migration file, in the layout goose expects.
//
//go:embed *.sql
var FS embed.FS

// Package web embeds the single-page client that ships with the binary.
package web

import "embed"

// FS holds the static assets served at the site root.
//
//go:embed index.html style.css app.js
var FS embed.FS

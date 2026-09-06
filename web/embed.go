package web

import "embed"

//go:embed index.html styles.css app.js
var FS embed.FS

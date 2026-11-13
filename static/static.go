package static

import "embed"

// Files contains the embedded static assets.
//
//go:embed index.html
var Files embed.FS

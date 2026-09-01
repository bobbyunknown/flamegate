package static

import "embed"

const DistDir = "dist"

// Files contains the dashboard build copied by frontend bundle scripts.
//
//go:embed all:dist
var Files embed.FS

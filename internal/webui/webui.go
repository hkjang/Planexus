package webui

import "embed"

// Dist is replaced by the production frontend build before compiling the server.
//
//go:embed dist/*
var Dist embed.FS

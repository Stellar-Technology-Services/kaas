package ui

import (
	"embed"
	"io/fs"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticEmbed embed.FS

func staticFS() fs.FS {
	sub, err := fs.Sub(staticEmbed, "static")
	if err != nil {
		return staticEmbed
	}
	return sub
}

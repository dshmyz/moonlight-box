package main

import (
	"embed"
	"io/fs"
)

//go:embed dist
var embeddedFrontend embed.FS

func GetEmbeddedFrontend() fs.FS {
	sub, err := fs.Sub(embeddedFrontend, "dist")
	if err != nil {
		panic("failed to access embedded frontend: " + err.Error())
	}
	return sub
}

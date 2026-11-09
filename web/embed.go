package web

import (
	"embed"
	"io/fs"
)

//go:embed all:build
var build embed.FS

func FS() fs.FS {
	sub, err := fs.Sub(build, "build")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}

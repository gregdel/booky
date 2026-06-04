package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var rawFiles embed.FS

var Files fs.FS = mustSub(rawFiles, "dist")

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

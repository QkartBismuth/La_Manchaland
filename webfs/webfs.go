package webfs

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/ui/*
var embeddedFS embed.FS

func Get() http.FileSystem {
	sub, _ := fs.Sub(embeddedFS, "web/ui")
	return http.FS(sub)
}

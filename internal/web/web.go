package web

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var staticFiles embed.FS

// StaticFS returns the embedded static files as an fs.FS rooted at "static".
// This can be passed to http.FS() for serving.
func StaticFS() fs.FS {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("web: failed to create sub filesystem: " + err.Error())
	}
	return sub
}

// IndexHTML returns the raw bytes of the embedded index.html file.
func IndexHTML() []byte {
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		panic("web: failed to read embedded index.html: " + err.Error())
	}
	return data
}

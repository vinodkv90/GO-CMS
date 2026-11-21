package admin

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var DistFS embed.FS

// HasEmbeddedAdmin returns true if the embedded filesystem contains an index.html file.
func HasEmbeddedAdmin() bool {
	if _, err := DistFS.ReadFile("dist/index.html"); err == nil {
		return true
	}
	return false
}

// SubFS returns an http.FileSystem rooted at the embedded "dist" directory.
func SubFS() (http.FileSystem, error) {
	sub, err := fs.Sub(DistFS, "dist")
	if err != nil {
		return nil, err
	}
	return http.FS(sub), nil
}

// ReadIndexHTML returns the raw bytes of the embedded admin index.html.
func ReadIndexHTML() ([]byte, error) {
	return DistFS.ReadFile("dist/index.html")
}


// Package studio serves the embedded Koda Studio web application.
package studio

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var assets embed.FS

// NewHandler returns an HTTP handler for the embedded Studio single-page app.
// Requests that do not match a static asset fall back to index.html so client
// side routes can be opened and refreshed directly.
func NewHandler() (http.Handler, error) {
	content, err := fs.Sub(assets, "dist")
	if err != nil {
		return nil, fmt.Errorf("studio: open embedded assets: %w", err)
	}
	if _, err := fs.Stat(content, "index.html"); err != nil {
		return nil, fmt.Errorf("studio: find embedded index.html: %w", err)
	}
	files := http.FileServer(http.FS(content))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		path := strings.TrimPrefix(request.URL.Path, "/")
		if path != "" {
			if info, err := fs.Stat(content, path); err == nil && !info.IsDir() {
				files.ServeHTTP(response, request)
				return
			}
		}
		fallback := request.Clone(request.Context())
		fallback.URL.Path = "/"
		files.ServeHTTP(response, fallback)
	}), nil
}

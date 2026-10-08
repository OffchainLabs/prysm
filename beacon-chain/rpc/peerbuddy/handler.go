// Package peerbuddy serves PeerBuddy, the beacon node's peer connectivity and scoring dashboard.
// The dashboard is a static page embedded in the binary that polls the node's own REST API.
package peerbuddy

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// Path is the URL path the dashboard is served from; the trailing slash makes it a subtree route.
const Path = "/peerbuddy/"

//go:embed static
var staticFiles embed.FS

// Handler serves the embedded dashboard: the index page at Path and its assets directly beneath it.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, Path)
		if name == "" {
			name = "index.html"
		}
		// Assets live in one flat directory; anything deeper is not ours.
		if strings.Contains(name, "/") {
			http.NotFound(w, r)
			return
		}
		file := "static/" + name
		if _, err := fs.Stat(staticFiles, file); err != nil {
			http.NotFound(w, r)
			return
		}
		// Assets are tiny and change with every release, so always revalidate.
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, staticFiles, file)
	})
}

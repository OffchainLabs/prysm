// Package doctorpeer serves DoctorPeer, the beacon node's peer connectivity and scoring dashboard.
// The dashboard is a static page embedded in the binary that polls the node's own REST API.
package doctorpeer

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// Path is the URL path the dashboard is served from; the trailing slash makes it a subtree route.
const Path = "/doctorpeer/"

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
		if strings.HasSuffix(name, ".woff2") {
			// Go's mime table has no woff2 entry, and fonts never change within a release.
			w.Header().Set("Content-Type", "font/woff2")
			w.Header().Set("Cache-Control", "max-age=86400")
		} else {
			// Assets are tiny and change with every release, so always revalidate.
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFileFS(w, r, staticFiles, file)
	})
}

package api

import (
	"embed"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed static/admin.html
var staticAssets embed.FS

func registerStaticRoutes(router interface {
	Get(string, http.HandlerFunc)
}) {
	router.Get("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		spec, err := loadOpenAPISpec()
		if err != nil {
			http.Error(w, "failed to load openapi spec", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(spec)
	})
	router.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})
	router.Get("/admin/", func(w http.ResponseWriter, r *http.Request) {
		content, err := staticAssets.ReadFile("static/admin.html")
		if err != nil {
			http.Error(w, "failed to load admin ui", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(content)
	})
}

func loadOpenAPISpec() ([]byte, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, os.ErrNotExist
	}
	rootDir := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	return os.ReadFile(filepath.Join(rootDir, "docs", "openapi", "openapi.yaml"))
}

package main

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
)

//go:embed web/templates/*.html web/static
var webFS embed.FS

func main() {
	templates, err := template.ParseFS(webFS, "web/templates/*.html")
	if err != nil {
		log.Fatal(err)
	}

	staticFS, err := fs.Sub(webFS, "web/static")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := templates.ExecuteTemplate(w, "index.html", nil); err != nil {
			log.Printf("render overview: %v", err)
		}
	})

	addr := ":8080"
	log.Printf("listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

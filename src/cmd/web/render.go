package main

import (
	"bytes"
	"html/template"
	"log/slog"
	"net/http"
)

// renderTemplate executes tmpl into a buffer. On success it writes the buffered
// HTML to w. On failure it logs the error and sends a 500 response, preventing
// partial output from reaching the client.
func renderTemplate(w http.ResponseWriter, tmpl *template.Template, data any) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		slog.Error("template execute failed", "template", tmpl.Name(), "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

package main

import (
	"bytes"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/refinement-systems/BorealValley/src/internal/assets"
)

var layoutTmpl = template.Must(template.New("layout").Parse(assets.HtmlLayout))

// parseWithLayout clones the shared layout template and registers the page's
// {{define "title"}} and {{define "content"}} blocks into the clone's template set.
func parseWithLayout(pageHTML string) *template.Template {
	t := template.Must(layoutTmpl.Clone())
	template.Must(t.New("_page").Parse(pageHTML))
	return t
}

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

var errorTmpl = parseWithLayout(assets.HtmlError)

type errorPageData struct {
	StatusCode int
	StatusText string
	Message    string
}

// renderError renders a styled HTML error page with the given status code and
// message. If the error template itself fails, it falls back to plain-text.
func renderError(w http.ResponseWriter, statusCode int, message string) {
	data := errorPageData{
		StatusCode: statusCode,
		StatusText: http.StatusText(statusCode),
		Message:    message,
	}
	var buf bytes.Buffer
	if err := errorTmpl.Execute(&buf, data); err != nil {
		slog.Error("error template execute failed", "err", err)
		http.Error(w, message, statusCode)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	buf.WriteTo(w)
}

// renderNotFound renders a styled 404 page.
func renderNotFound(w http.ResponseWriter) {
	renderError(w, http.StatusNotFound, "page not found")
}

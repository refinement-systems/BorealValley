package main

import (
	"errors"
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderTemplateWritesOnSuccess(t *testing.T) {
	t.Parallel()
	tmpl := template.Must(template.New("ok").Parse("Hello {{.Name}}"))
	rec := httptest.NewRecorder()
	renderTemplate(rec, tmpl, struct{ Name string }{"World"})
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Hello World") {
		t.Fatalf("expected rendered output, got %q", rec.Body.String())
	}
}

func TestRenderTemplateReturns500OnError(t *testing.T) {
	t.Parallel()
	// Template calls a function that always errors.
	tmpl := template.Must(template.New("bad").Funcs(template.FuncMap{
		"fail": func() (string, error) { return "", errors.New("forced error") },
	}).Parse(`Before{{fail}}After`))
	rec := httptest.NewRecorder()
	renderTemplate(rec, tmpl, nil)
	if rec.Code != 500 {
		t.Fatalf("expected 500 on template error, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "Before") {
		t.Fatalf("partial template output should not be written to client")
	}
}

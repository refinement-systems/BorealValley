package main

import (
	"errors"
	"html/template"
	"net/http"
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

func TestRenderErrorSetsStatusCode(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	renderError(rec, http.StatusNotFound, "page not found")
	if rec.Code != 404 {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html content type, got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "404") {
		t.Fatal("expected body to contain status code")
	}
	if !strings.Contains(body, "Not Found") {
		t.Fatal("expected body to contain status text")
	}
	if !strings.Contains(body, "page not found") {
		t.Fatal("expected body to contain error message")
	}
}

func TestRenderErrorVariousStatuses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		code int
		msg  string
	}{
		{http.StatusBadRequest, "bad request"},
		{http.StatusUnauthorized, "authentication error"},
		{http.StatusForbidden, "permission error"},
		{http.StatusMethodNotAllowed, "method not allowed"},
		{http.StatusTooManyRequests, "too many requests"},
		{http.StatusInternalServerError, "internal error"},
	} {
		rec := httptest.NewRecorder()
		renderError(rec, tc.code, tc.msg)
		if rec.Code != tc.code {
			t.Errorf("renderError(%d, %q): got status %d", tc.code, tc.msg, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), tc.msg) {
			t.Errorf("renderError(%d, %q): body missing message", tc.code, tc.msg)
		}
	}
}

func TestRenderNotFound(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	renderNotFound(rec)
	if rec.Code != 404 {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "page not found") {
		t.Fatal("expected body to contain 'page not found'")
	}
}

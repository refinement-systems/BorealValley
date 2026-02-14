// Permission to use, copy, modify, and/or distribute this software for
// any purpose with or without fee is hereby granted.
//
// THE SOFTWARE IS PROVIDED “AS IS” AND THE AUTHOR DISCLAIMS ALL
// WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES
// OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE
// FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY
// DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN
// AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT
// OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

package main

import "net/http"

// login handles the login form. GET renders the form; POST reads the username and
// password fields, verifies credentials via the store, renews the session token on
// success, and redirects to /web/admin. Invalid credentials re-render the form with an
// error message.
func (app *application) login(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		returnTo := sanitizeReturnTo(r.URL.Query().Get("return_to"))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = loginTmpl.Execute(w, struct {
			Err      string
			ReturnTo string
		}{Err: "", ReturnTo: returnTo})

	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		username := r.PostForm.Get("username")
		password := r.PostForm.Get("password")
		returnTo := sanitizeReturnTo(r.PostForm.Get("return_to"))

		id, ok, err := app.store.VerifyUser(r.Context(), username, password)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = loginTmpl.Execute(w, struct {
				Err      string
				ReturnTo string
			}{Err: "Invalid credentials", ReturnTo: returnTo})
			return
		}

		if err := app.sessionManager.RenewToken(r.Context()); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		app.sessionManager.Put(r.Context(), "user_id", id)
		if returnTo == "" {
			returnTo = "/web/admin"
		}
		http.Redirect(w, r, returnTo, http.StatusSeeOther)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func sanitizeReturnTo(raw string) string {
	if raw == "" {
		return ""
	}
	if raw[0] != '/' {
		return ""
	}
	if len(raw) >= 2 && raw[1] == '/' {
		return ""
	}
	return raw
}

// logout destroys the current session and redirects the user to /web/login.
// Only POST is accepted.
func (app *application) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = app.sessionManager.Destroy(r.Context())
	http.Redirect(w, r, "/web/login", http.StatusSeeOther)
}

// requireAuth is a middleware that redirects unauthenticated requests to /web/login.
// A request is considered authenticated when "user_id" is present in the session.
func (app *application) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !app.sessionManager.Exists(r.Context(), "user_id") {
			http.Redirect(w, r, "/web/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

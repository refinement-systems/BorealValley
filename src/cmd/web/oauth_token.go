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

import (
	"net/http"

	"github.com/ory/fosite"
)

func (app *application) oauthToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	accessRequest, err := app.oauth.Provider.NewAccessRequest(r.Context(), r, &fosite.DefaultSession{})
	if err != nil {
		app.oauth.Provider.WriteAccessError(r.Context(), w, nil, err)
		return
	}

	response, err := app.oauth.Provider.NewAccessResponse(r.Context(), accessRequest)
	if err != nil {
		app.oauth.Provider.WriteAccessError(r.Context(), w, accessRequest, err)
		return
	}
	app.oauth.Provider.WriteAccessResponse(r.Context(), w, accessRequest, response)
}

func (app *application) oauthRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	err := app.oauth.Provider.NewRevocationRequest(r.Context(), r)
	app.oauth.Provider.WriteRevocationResponse(r.Context(), w, err)
}

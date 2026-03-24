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
	"context"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ory/fosite"
	"github.com/refinement-systems/BorealValley/src/internal/assets"
)

type oauthAuthorizePageData struct {
	RawQuery          string
	ClientName        string
	ClientDescription string
	RequestedScopes   []string
	PreApproved       map[string]bool
	Err               string
}

var oauthAuthorizeTmpl = template.Must(template.New("oauth-consent").Parse(assets.HtmlOAuthConsent))

type authorizeErrorWriter interface {
	WriteAuthorizeError(ctx context.Context, rw http.ResponseWriter, ar fosite.AuthorizeRequester, err error)
}

func writeAuthorizeError(ctx context.Context, w http.ResponseWriter, writer authorizeErrorWriter, requester fosite.AuthorizeRequester, err error) {
	if requester == nil {
		requester = fosite.NewAuthorizeRequest()
	}
	writer.WriteAuthorizeError(ctx, w, requester, err)
}

func (app *application) oauthAuthorizeGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if app.oauthAuthorizeMustReauthenticate(r) || !app.sessionManager.Exists(r.Context(), "user_id") {
		http.Redirect(w, r, app.oauthAuthorizeLoginRedirectURL(r), http.StatusSeeOther)
		return
	}
	authorizeRequester, err := app.oauth.Provider.NewAuthorizeRequest(r.Context(), r)
	if err != nil {
		writeAuthorizeError(r.Context(), w, app.oauth.Provider, authorizeRequester, err)
		return
	}

	userID, _, ok := app.oauthSessionUser(r)
	if !ok {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}

	clientName := authorizeRequester.GetClient().GetID()
	clientDescription := ""
	if client, found, err := app.store.GetOAuthClient(r.Context(), authorizeRequester.GetClient().GetID()); err == nil && found {
		clientName = client.Name
		clientDescription = client.Description
	}

	preApproved := map[string]bool{}
	if grant, found, err := app.store.GetLatestActiveOAuthConsentGrant(r.Context(), userID, authorizeRequester.GetClient().GetID()); err == nil && found {
		for _, scope := range grant.GrantedScopes {
			preApproved[scope] = true
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = oauthAuthorizeTmpl.Execute(w, oauthAuthorizePageData{
		RawQuery:          r.URL.RawQuery,
		ClientName:        clientName,
		ClientDescription: clientDescription,
		RequestedScopes:   append([]string(nil), authorizeRequester.GetRequestedScopes()...),
		PreApproved:       preApproved,
		Err:               "",
	})

}

func (app *application) oauthAuthorizeMustReauthenticate(r *http.Request) bool {
	for _, prompt := range strings.Fields(r.URL.Query().Get("prompt")) {
		if strings.EqualFold(strings.TrimSpace(prompt), "login") {
			return true
		}
	}
	return false
}

func (app *application) oauthAuthorizeLoginRedirectURL(r *http.Request) string {
	return "/web/login?return_to=" + url.QueryEscape(oauthAuthorizeReturnTo(r))
}

func oauthAuthorizeReturnTo(r *http.Request) string {
	u := *r.URL
	query := u.Query()
	prompts := strings.Fields(query.Get("prompt"))
	if len(prompts) > 0 {
		kept := make([]string, 0, len(prompts))
		for _, prompt := range prompts {
			if !strings.EqualFold(strings.TrimSpace(prompt), "login") {
				kept = append(kept, strings.TrimSpace(prompt))
			}
		}
		if len(kept) == 0 {
			query.Del("prompt")
		} else {
			query.Set("prompt", strings.Join(kept, " "))
		}
	}
	u.RawQuery = query.Encode()
	return u.RequestURI()
}

func (app *application) oauthAuthorizePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !app.sessionManager.Exists(r.Context(), "user_id") {
		http.Redirect(w, r, "/web/login?return_to="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	rawQuery := strings.TrimSpace(r.PostFormValue("query"))
	authorizeRequester, err := app.oauthAuthorizeRequestFromRawQuery(r, rawQuery)
	if err != nil {
		writeAuthorizeError(r.Context(), w, app.oauth.Provider, authorizeRequester, err)
		return
	}

	action := strings.TrimSpace(r.PostFormValue("action"))
	if action != "approve" {
		app.oauth.Provider.WriteAuthorizeError(r.Context(), w, authorizeRequester, fosite.ErrAccessDenied.WithHint("resource owner denied the authorization request"))
		return
	}

	requestedScopes := append([]string(nil), authorizeRequester.GetRequestedScopes()...)
	approvedScopes := approvedScopesSubset(r.PostForm["scope"], requestedScopes)
	if len(approvedScopes) == 0 {
		app.oauth.Provider.WriteAuthorizeError(r.Context(), w, authorizeRequester, fosite.ErrAccessDenied.WithHint("at least one scope must be approved"))
		return
	}
	for _, scope := range approvedScopes {
		authorizeRequester.GrantScope(scope)
	}

	userID, username, ok := app.oauthSessionUser(r)
	if !ok {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}

	grant, err := app.store.UpsertOAuthConsentGrant(r.Context(), authorizeRequester.GetClient().GetID(), userID, requestedScopes, approvedScopes)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	session := &fosite.DefaultSession{
		Subject:  strconv.FormatInt(userID, 10),
		Username: username,
		Extra: map[string]interface{}{
			"grant_id":          grant.GrantID,
			"refresh_family_id": grant.GrantID,
		},
	}

	response, err := app.oauth.Provider.NewAuthorizeResponse(r.Context(), authorizeRequester, session)
	if err != nil {
		app.oauth.Provider.WriteAuthorizeError(r.Context(), w, authorizeRequester, err)
		return
	}
	app.oauth.Provider.WriteAuthorizeResponse(r.Context(), w, authorizeRequester, response)
}

func (app *application) oauthAuthorizeRequestFromRawQuery(r *http.Request, rawQuery string) (fosite.AuthorizeRequester, error) {
	u := *r.URL
	u.RawQuery = rawQuery
	req := r.Clone(r.Context())
	req.Method = http.MethodGet
	req.URL = &u
	req.Form = nil
	req.PostForm = nil
	return app.oauth.Provider.NewAuthorizeRequest(r.Context(), req)
}

func approvedScopesSubset(selected []string, requested []string) []string {
	allowed := map[string]struct{}{}
	for _, scope := range requested {
		allowed[scope] = struct{}{}
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, raw := range selected {
		scope := strings.TrimSpace(raw)
		if scope == "" {
			continue
		}
		if _, ok := allowed[scope]; !ok {
			continue
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}

func (app *application) oauthSessionUser(r *http.Request) (int64, string, bool) {
	userID, ok := sessionUserIDFromContext(app, r)
	if !ok || userID <= 0 {
		return 0, "", false
	}
	username, found, err := app.store.GetUsernameByID(r.Context(), userID)
	if err != nil || !found {
		return 0, "", false
	}
	return userID, username, true
}

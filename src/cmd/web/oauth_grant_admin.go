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
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"strings"

	"github.com/refinement-systems/BorealValley/src/internal/assets"
	"github.com/refinement-systems/BorealValley/src/internal/common"
)

type oauthGrantAdminRow struct {
	GrantID       string
	ClientID      string
	ClientName    string
	GrantedScopes []string
	CreatedAt     string
	RevokedAt     *string
}

type oauthGrantAdminData struct {
	Grants []oauthGrantAdminRow
	Err    string
}

var oauthGrantAdminTmpl = template.Must(template.New("oauth-grant-list").Parse(assets.HtmlOAuthGrantList))

func (app *application) oauthGrantAdminList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := sessionUserIDFromContext(app, r)
	if !ok || userID <= 0 {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}
	grants, err := app.store.ListOAuthConsentGrantsByUser(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	rows := oauthGrantRowsForDisplay(grants)
	renderTemplate(w, oauthGrantAdminTmpl, oauthGrantAdminData{Grants: rows})
}

func (app *application) oauthGrantAdminRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := sessionUserIDFromContext(app, r)
	if !ok || userID <= 0 {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}
	grantID := strings.TrimSpace(r.PathValue("grant"))
	if err := app.store.RevokeOAuthConsentGrant(r.Context(), userID, grantID); err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/web/oauth/grant", http.StatusSeeOther)
}

func (app *application) apiV1OAuthGrantList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}
	grants, err := app.store.ListOAuthConsentGrantsByUser(r.Context(), principal.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"grants": grants})
}

func (app *application) apiV1OAuthGrantRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}
	grantID := strings.TrimSpace(r.PathValue("grant"))
	if err := app.store.RevokeOAuthConsentGrant(r.Context(), principal.UserID, grantID); err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "revoked", "grant": grantID})
}

func oauthGrantRowsForDisplay(grants []common.OAuthConsentGrantRecord) []oauthGrantAdminRow {
	rows := make([]oauthGrantAdminRow, 0, len(grants))
	for _, grant := range grants {
		row := oauthGrantAdminRow{
			GrantID:       grant.GrantID,
			ClientID:      grant.ClientID,
			ClientName:    grant.ClientName,
			GrantedScopes: append([]string(nil), grant.GrantedScopes...),
			CreatedAt:     grant.CreatedAt.UTC().Format("2006-01-02 15:04:05Z"),
		}
		if grant.RevokedAt != nil {
			t := grant.RevokedAt.UTC().Format("2006-01-02 15:04:05Z")
			row.RevokedAt = &t
		}
		rows = append(rows, row)
	}
	return rows
}

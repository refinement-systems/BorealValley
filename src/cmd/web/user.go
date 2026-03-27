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
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/refinement-systems/BorealValley/src/internal/assets"
)

var userCtlTmpl = template.Must(template.New("ctl-user").Parse(assets.HtmlCtlUser))

const apMediaType = `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`

type userCtlTemplateData struct {
	Username  string
	ActorJSON string
}

func (app *application) userCtl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requestedUsername := strings.TrimSpace(r.PathValue("name"))
	if requestedUsername == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	sessionUserID, ok := sessionUserIDFromContext(app, r)
	if !ok {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}

	username, ok, err := app.store.GetUsernameByID(r.Context(), sessionUserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}
	if username != requestedUsername {
		http.Error(w, "permission error", http.StatusForbidden)
		return
	}

	record, ok, err := app.store.GetUserActorByUsername(r.Context(), requestedUsername)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	actorJSON, err := sanitizeActorJSON(record.ActorJSON)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	accept := strings.ToLower(r.Header.Get("Accept"))
	if wantsUserActorJSON(accept) {
		w.Header().Set("Content-Type", userActorJSONContentType(accept))
		_, _ = w.Write(actorJSON)
		return
	}

	pretty, err := prettyJSON(actorJSON)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	renderTemplate(w, userCtlTmpl, userCtlTemplateData{
		Username:  record.Username,
		ActorJSON: string(pretty),
	})
}

func sessionUserIDFromContext(app *application, r *http.Request) (int64, bool) {
	value := app.sessionManager.Get(r.Context(), "user_id")
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	default:
		return 0, false
	}
}

func sessionUserIsAdmin(app *application, r *http.Request) (bool, error) {
	userID, ok := sessionUserIDFromContext(app, r)
	if !ok {
		return false, nil
	}
	return app.store.IsUserAdmin(r.Context(), userID)
}

func wantsUserActorJSON(accept string) bool {
	return strings.Contains(accept, "application/json") ||
		strings.Contains(accept, "application/activity+json") ||
		strings.Contains(accept, "application/ld+json")
}

func userActorJSONContentType(accept string) string {
	if strings.Contains(accept, "application/activity+json") || strings.Contains(accept, "application/ld+json") {
		return apMediaType
	}
	return "application/json"
}

func sanitizeActorJSON(raw []byte) ([]byte, error) {
	var actor map[string]any
	if err := json.Unmarshal(raw, &actor); err != nil {
		return nil, err
	}
	delete(actor, "privateKey")
	delete(actor, "privateKeyMultibase")
	delete(actor, "private_key_multibase")
	return json.Marshal(actor)
}

func prettyJSON(raw []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

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

package common

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrValidation = errors.New("validation error")

var objectTypeToTable = map[string]string{
	"Person":                 "as_person",
	"Application":            "as_application",
	"OrderedCollection":      "as_ordered_collection",
	"Note":                   "as_note",
	"Update":                 "as_update",
	"TeamMembership":         "ff_team_membership",
	"Team":                   "ff_team",
	"Repository":             "ff_repository",
	"TicketTracker":          "ff_ticket_tracker",
	"PatchTracker":           "ff_patch_tracker",
	"Ticket":                 "ff_ticket",
	"Patch":                  "ff_patch",
	"Commit":                 "ff_commit",
	"Branch":                 "ff_branch",
	"Factory":                "ff_factory",
	"Enum":                   "ff_enum",
	"EnumValue":              "ff_enum_value",
	"Field":                  "ff_field",
	"Milestone":              "ff_milestone",
	"Release":                "ff_release",
	"Review":                 "ff_review",
	"ReviewThread":           "ff_review_thread",
	"CodeQuote":              "ff_code_quote",
	"Suggestion":             "ff_suggestion",
	"Approval":               "ff_approval",
	"OrganizationMembership": "ff_organization_membership",
	"SshPublicKey":           "ff_ssh_public_key",
	"TicketDependency":       "ff_ticket_dependency",
	"Edit":                   "ff_edit",
	"Grant":                  "ff_grant",
	"Revoke":                 "ff_revoke",
	"Assign":                 "ff_assign",
	"Apply":                  "ff_apply",
	"Resolve":                "ff_resolve",
	"Push":                   "ff_push",
}

var objectTables = []string{
	"ap_object_unknown",
	"as_application",
	"as_note",
	"as_ordered_collection",
	"as_person",
	"as_update",
	"ff_apply",
	"ff_approval",
	"ff_assign",
	"ff_branch",
	"ff_code_quote",
	"ff_commit",
	"ff_edit",
	"ff_enum",
	"ff_enum_value",
	"ff_factory",
	"ff_field",
	"ff_grant",
	"ff_milestone",
	"ff_organization_membership",
	"ff_patch",
	"ff_patch_tracker",
	"ff_push",
	"ff_release",
	"ff_repository",
	"ff_resolve",
	"ff_review",
	"ff_review_thread",
	"ff_revoke",
	"ff_ssh_public_key",
	"ff_suggestion",
	"ff_team",
	"ff_team_membership",
	"ff_ticket",
	"ff_ticket_dependency",
	"ff_ticket_tracker",
}

func (s *Store) StoreObjectJSON(ctx context.Context, raw []byte) (objectID string, objectType string, table string, err error) {
	m, id, typ, err := parseObjectIdentityType(raw)
	if err != nil {
		return "", "", "", err
	}

	tx, err := s.UserStore.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", "", err
	}
	defer tx.Rollback()

	table, err = upsertObjectTx(ctx, tx, id, typ, m)
	if err != nil {
		return "", "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", "", err
	}
	return id, typ, table, nil
}

func parseObjectIdentityType(raw []byte) (map[string]any, string, string, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, "", "", fmt.Errorf("%w: invalid json: %v", ErrValidation, err)
	}

	id, err := requiredStringField(m, "id")
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: %v", ErrValidation, err)
	}
	typ, err := parseObjectType(m["type"])
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return m, id, typ, nil
}

func parseObjectType(raw any) (string, error) {
	switch v := raw.(type) {
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return "", errors.New("type cannot be empty")
		}
		return v, nil
	case []any:
		if len(v) == 0 {
			return "", errors.New("type cannot be empty")
		}
		candidate := ""
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if _, ok := objectTypeToTable[s]; ok {
				return s, nil
			}
			if candidate == "" {
				candidate = s
			}
		}
		if candidate == "" {
			return "", errors.New("type cannot be parsed")
		}
		return candidate, nil
	default:
		return "", errors.New("type must be a string or array")
	}
}

func requiredStringField(m map[string]any, field string) (string, error) {
	v, ok := m[field]
	if !ok {
		return "", fmt.Errorf("missing %s", field)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", field)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%s cannot be empty", field)
	}
	return s, nil
}

var allowedTableSet = func() map[string]bool {
	m := make(map[string]bool, len(objectTables))
	for _, t := range objectTables {
		m[t] = true
	}
	return m
}()

var tableToTypeName = func() map[string]string {
	m := make(map[string]string, len(objectTypeToTable)+1)
	for typeName, table := range objectTypeToTable {
		m[table] = typeName
	}
	m["ap_object_unknown"] = "Unknown"
	return m
}()

// objectTableCountQueries maps each allowed table to its static COUNT query,
// computed once at init from the hardcoded objectTables slice so that
// ListObjectTypeCounts never constructs SQL at runtime.
var objectTableCountQueries = func() map[string]string {
	m := make(map[string]string, len(objectTables))
	for _, t := range objectTables {
		m[t] = `SELECT COUNT(*) FROM "` + t + `"`
	}
	return m
}()

func allowedTable(name string) bool {
	return allowedTableSet[name]
}

func upsertObjectTx(ctx context.Context, tx *sql.Tx, id, objectType string, rawObject map[string]any) (string, error) {
	rawJSON, err := json.Marshal(rawObject)
	if err != nil {
		return "", err
	}

	table, ok := objectTypeToTable[objectType]
	if !ok {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO ap_object_unknown (primary_key, ap_type, body, created_at, updated_at)
		         VALUES ($1, $2, $3::jsonb, now(), now())
		      ON CONFLICT (primary_key)
		      DO UPDATE SET
		         ap_type = EXCLUDED.ap_type,
		         body = EXCLUDED.body,
		         updated_at = now()`,
			id, objectType, string(rawJSON),
		)
		if err != nil {
			return "", err
		}
		return "ap_object_unknown", nil
	}
	if !allowedTable(table) {
		return "", fmt.Errorf("disallowed table name: %q", table)
	}

	query := fmt.Sprintf(`INSERT INTO "%s" (primary_key, body, created_at, updated_at)
        VALUES ($1, $2::jsonb, now(), now())
        ON CONFLICT (primary_key)
        DO UPDATE SET
           body = EXCLUDED.body,
           updated_at = now()`, table)
	if _, err := tx.ExecContext(ctx, query, id, string(rawJSON)); err != nil {
		return "", err
	}
	return table, nil
}

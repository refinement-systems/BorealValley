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
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func parseOptionalPositiveInt64(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid positive int")
	}
	return value, nil
}

func parseRequiredPositiveInt64(raw string) (int64, error) {
	value, err := parseOptionalPositiveInt64(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid positive int")
	}
	return value, nil
}

func parseLimitQuery(raw string, defaultValue int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}
	if value > 100 {
		return 100
	}
	return value
}

func buildNotificationNextLink(r *http.Request, lastID int64, limit int) string {
	if lastID <= 0 {
		return ""
	}
	q := url.Values{}
	for key, values := range r.URL.Query() {
		for _, value := range values {
			q.Add(key, value)
		}
	}
	q.Del("min_id")
	q.Set("max_id", strconv.FormatInt(lastID, 10))
	q.Set("limit", strconv.Itoa(limit))
	return fmt.Sprintf("</api/v1/notification?%s>; rel=\"next\"", q.Encode())
}

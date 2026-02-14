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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

type apiClient struct {
	ServerURL   string
	AccessToken string
	HTTPClient  *http.Client
}

const (
	defaultHTTPTimeout  = 30 * time.Second
	lmStudioHTTPTimeout = 5 * time.Minute
)

func newAPIClient(serverURL, accessToken string) *apiClient {
	return &apiClient{
		ServerURL:   strings.TrimRight(strings.TrimSpace(serverURL), "/"),
		AccessToken: strings.TrimSpace(accessToken),
		HTTPClient:  defaultHTTPClient(),
	}
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: defaultHTTPTimeout}
}

func lmStudioHTTPClient() *http.Client {
	return &http.Client{Timeout: lmStudioHTTPTimeout}
}

func (c *apiClient) getAssignedTickets(ctx context.Context, limit int) ([]common.AssignedTicket, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("agent_completion_pending", strconv.FormatBool(true))
	reqURL := c.ServerURL + "/api/v1/ticket/assigned?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Ticket []common.AssignedTicket `json:"ticket"`
	}
	if err := c.doJSON(req, nil, &payload); err != nil {
		return nil, err
	}
	return payload.Ticket, nil
}

func (c *apiClient) createTicketComment(ctx context.Context, trackerSlug, ticketSlug, content, agentCommentKind string) (common.TicketComment, error) {
	body := map[string]any{
		"content": strings.TrimSpace(content),
	}
	if strings.TrimSpace(agentCommentKind) != "" {
		body["agent_comment_kind"] = strings.TrimSpace(agentCommentKind)
	}
	req, err := c.newJSONRequest(ctx, http.MethodPost, c.ServerURL+"/api/v1/ticket-tracker/"+url.PathEscape(trackerSlug)+"/ticket/"+url.PathEscape(ticketSlug)+"/comment", body)
	if err != nil {
		return common.TicketComment{}, err
	}
	var comment common.TicketComment
	if err := c.doJSON(req, []int{http.StatusCreated}, &comment); err != nil {
		return common.TicketComment{}, err
	}
	return comment, nil
}

func (c *apiClient) createTicketCommentUpdate(ctx context.Context, trackerSlug, ticketSlug, commentSlug, content string) error {
	body := map[string]any{
		"content": strings.TrimSpace(content),
	}
	req, err := c.newJSONRequest(ctx, http.MethodPost, c.ServerURL+"/api/v1/ticket-tracker/"+url.PathEscape(trackerSlug)+"/ticket/"+url.PathEscape(ticketSlug)+"/comment/"+url.PathEscape(commentSlug)+"/update", body)
	if err != nil {
		return err
	}
	return c.doJSON(req, []int{http.StatusCreated}, nil)
}

func (c *apiClient) newJSONRequest(ctx context.Context, method, target string, payload any) (*http.Request, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (c *apiClient) doJSON(req *http.Request, expectedStatus []int, out any) error {
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if len(expectedStatus) == 0 {
		expectedStatus = []int{http.StatusOK}
	}
	if !containsStatus(expectedStatus, resp.StatusCode) {
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	return nil
}

func containsStatus(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func fetchProfile(ctx context.Context, client *http.Client, serverURL, accessToken string) (profileState, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(strings.TrimSpace(serverURL), "/")+"/api/v1/profile", nil)
	if err != nil {
		return profileState{}, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	resp, err := client.Do(req)
	if err != nil {
		return profileState{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return profileState{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return profileState{}, fmt.Errorf("profile request failed: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		ActorID   string `json:"actor_id"`
		MainKeyID string `json:"main_key_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return profileState{}, err
	}
	return profileState{
		UserID:    payload.ID,
		Username:  strings.TrimSpace(payload.Username),
		ActorID:   strings.TrimSpace(payload.ActorID),
		MainKeyID: strings.TrimSpace(payload.MainKeyID),
	}, nil
}

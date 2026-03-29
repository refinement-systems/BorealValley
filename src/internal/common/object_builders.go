// Permission to use, copy, modify, and/or distribute this software for
// any purpose with or without fee is hereby granted.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL
// WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES
// OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE
// FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY
// DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN
// AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT
// OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

package common

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type apMediaSource struct {
	MediaType string `json:"mediaType"`
	Content   string `json:"content"`
}

type apRepositoryObject struct {
	Context    []string `json:"@context"`
	ID         string   `json:"id"`
	Type       string   `json:"type"`
	Name       string   `json:"name"`
	Inbox      string   `json:"inbox"`
	Outbox     string   `json:"outbox"`
	MainBranch string   `json:"mainBranch"`
}

type apTicketTrackerObject struct {
	Context []string `json:"@context"`
	ID      string   `json:"id"`
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Inbox   string   `json:"inbox"`
	Outbox  string   `json:"outbox"`
	Summary string   `json:"summary,omitempty"`
}

type apTicketObject struct {
	Context       []string      `json:"@context"`
	ID            string        `json:"id"`
	Type          string        `json:"type"`
	TicketContext string        `json:"context"`
	Target        string        `json:"target"`
	AttributedTo  string        `json:"attributedTo"`
	Summary       string        `json:"summary"`
	Content       string        `json:"content"`
	MediaType     string        `json:"mediaType"`
	Source        apMediaSource `json:"source"`
	Published     string        `json:"published"`
	IsResolved    bool          `json:"isResolved"`
	Followers     string        `json:"followers"`
	Replies       string        `json:"replies"`
	Team          string        `json:"team"`
	Dependencies  string        `json:"dependencies"`
	Dependants    string        `json:"dependants"`
}

type apNoteObject struct {
	Context          []string      `json:"@context"`
	ID               string        `json:"id"`
	Type             string        `json:"type"`
	NoteContext      string        `json:"context"`
	InReplyTo        string        `json:"inReplyTo"`
	AttributedTo     string        `json:"attributedTo"`
	To               []string      `json:"to"`
	Content          string        `json:"content"`
	MediaType        string        `json:"mediaType"`
	Source           apMediaSource `json:"source"`
	Published        string        `json:"published"`
	AgentCommentKind string        `json:"borealValleyAgentCommentKind,omitempty"`
}

type apUpdateObject struct {
	Context   []string `json:"@context"`
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	Actor     string   `json:"actor"`
	Object    string   `json:"object"`
	Content   string   `json:"content"`
	MediaType string   `json:"mediaType"`
	Published string   `json:"published"`
}

func buildLocalRepositoryObject(actorID, name string) apRepositoryObject {
	return apRepositoryObject{
		Context:    []string{"https://www.w3.org/ns/activitystreams", "https://forgefed.org/ns"},
		ID:         actorID,
		Type:       "Repository",
		Name:       name,
		Inbox:      actorID + "/inbox",
		Outbox:     actorID + "/outbox",
		MainBranch: actorID + "/branches/main",
	}
}

func buildLocalTicketTrackerObject(actorID, name, summary string) apTicketTrackerObject {
	return apTicketTrackerObject{
		Context: []string{"https://www.w3.org/ns/activitystreams", "https://forgefed.org/ns"},
		ID:      actorID,
		Type:    "TicketTracker",
		Name:    name,
		Inbox:   actorID + "/inbox",
		Outbox:  actorID + "/outbox",
		Summary: summary,
	}
}

func buildLocalTicketObject(actorID, trackerActorID, repoActorID, authorActorID, summary, content string, published time.Time) apTicketObject {
	return apTicketObject{
		Context:       []string{"https://www.w3.org/ns/activitystreams", "https://forgefed.org/ns"},
		ID:            actorID,
		Type:          "Ticket",
		TicketContext: trackerActorID,
		Target:        repoActorID,
		AttributedTo:  authorActorID,
		Summary:       summary,
		Content:       content,
		MediaType:     "text/plain",
		Source:        apMediaSource{MediaType: "text/plain", Content: content},
		Published:     published.Format(time.RFC3339Nano),
		IsResolved:    false,
		Followers:     actorID + "/followers",
		Replies:       actorID + "/replies",
		Team:          actorID + "/team",
		Dependencies:  actorID + "/dependencies",
		Dependants:    actorID + "/dependants",
	}
}

func buildLocalTicketCommentObject(actorID, ticketActorID, inReplyToActorID, authorActorID, recipientActorID, content, agentCommentKind string, published time.Time) apNoteObject {
	return apNoteObject{
		Context:          []string{"https://www.w3.org/ns/activitystreams", "https://forgefed.org/ns"},
		ID:               actorID,
		Type:             "Note",
		NoteContext:      ticketActorID,
		InReplyTo:        inReplyToActorID,
		AttributedTo:     authorActorID,
		To:               []string{recipientActorID},
		Content:          content,
		MediaType:        "text/plain",
		Source:           apMediaSource{MediaType: "text/plain", Content: content},
		Published:        published.Format(time.RFC3339Nano),
		AgentCommentKind: agentCommentKind,
	}
}

func buildLocalUpdateObject(actorID, authorActorID, objectActorID, content string, published time.Time) apUpdateObject {
	return apUpdateObject{
		Context:   []string{"https://www.w3.org/ns/activitystreams", "https://forgefed.org/ns"},
		ID:        actorID,
		Type:      "Update",
		Actor:     authorActorID,
		Object:    objectActorID,
		Content:   content,
		MediaType: "text/plain",
		Published: published.Format(time.RFC3339Nano),
	}
}

func appendPlainTextUpdate(bodyRaw []byte, updateContent string, published time.Time) ([]byte, error) {
	body, err := parseBody(bodyRaw)
	if err != nil {
		return nil, err
	}

	updateContent = strings.TrimSpace(updateContent)
	if updateContent == "" {
		return nil, fmt.Errorf("%w: content is required", ErrValidation)
	}

	existing := strings.TrimSpace(stringFromAny(body["content"]))
	merged := updateContent
	if existing != "" {
		merged = existing + "\n\n" + updateContent
	}
	body["content"] = merged
	body["mediaType"] = "text/plain"
	body["updated"] = published.Format(time.RFC3339Nano)

	if src, ok := body["source"].(map[string]any); ok {
		src["content"] = merged
		if strings.TrimSpace(stringFromAny(src["mediaType"])) == "" {
			src["mediaType"] = "text/plain"
		}
		body["source"] = src
	} else {
		body["source"] = map[string]any{
			"mediaType": "text/plain",
			"content":   merged,
		}
	}

	return json.Marshal(body)
}

func parseBody(raw []byte) (map[string]any, error) {
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

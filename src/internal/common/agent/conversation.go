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

package agent

import "sync"

// ChatTurn stores one prompt/answer exchange.
type ChatTurn struct {
	Prompt string
	Answer string
	Err    string
}

// Conversation holds state for one session.
type Conversation struct {
	mu    sync.Mutex
	Turns []ChatTurn
}

// Store maps session tokens to conversations.
type Store struct {
	mu    sync.Mutex
	convs map[string]*Conversation
}

func NewStore() *Store {
	return &Store{convs: make(map[string]*Conversation)}
}

func (s *Store) GetOrCreate(token string) *Conversation {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.convs[token]; ok {
		return c
	}
	c := &Conversation{}
	s.convs[token] = c
	return c
}

func (s *Store) Clear(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.convs, token)
}

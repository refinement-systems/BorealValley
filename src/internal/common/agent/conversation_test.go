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

import "testing"

func TestStoreGetOrCreate(t *testing.T) {
	s := NewStore()
	c1 := s.GetOrCreate("tok1")
	c2 := s.GetOrCreate("tok1")
	if c1 != c2 {
		t.Fatal("same token must return same conversation")
	}
	c3 := s.GetOrCreate("tok2")
	if c1 == c3 {
		t.Fatal("different tokens must return different conversations")
	}
}

func TestStoreClear(t *testing.T) {
	s := NewStore()
	c := s.GetOrCreate("tok1")
	c.mu.Lock()
	c.Turns = append(c.Turns, ChatTurn{Prompt: "hello"})
	c.mu.Unlock()
	s.Clear("tok1")
	c2 := s.GetOrCreate("tok1")
	c2.mu.Lock()
	n := len(c2.Turns)
	c2.mu.Unlock()
	if n != 0 {
		t.Fatal("expected empty conversation after clear")
	}
}

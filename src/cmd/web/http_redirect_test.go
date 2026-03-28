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

package main

import (
	"bufio"
	"crypto/tls"
	"net"
	"net/http"
	"testing"
)

// dialAndSend opens a raw TCP connection to addr, writes rawRequest, and
// returns a buffered reader over the response.
func dialAndSend(t *testing.T, addr, rawRequest string) *bufio.Reader {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	if _, err := c.Write([]byte(rawRequest)); err != nil {
		t.Fatalf("write: %v", err)
	}
	return bufio.NewReader(c)
}

// newTestListener starts a tlsOrRedirectListener on a random port and returns
// it along with the address it is listening on.
func newTestListener(t *testing.T, httpsBase string) (*tlsOrRedirectListener, string) {
	t.Helper()
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	l := &tlsOrRedirectListener{
		Listener:  raw,
		tlsConfig: &tls.Config{MinVersion: tls.VersionTLS13},
		httpsBase: httpsBase,
	}
	t.Cleanup(func() { l.Close() })
	return l, raw.Addr().String()
}

func TestRedirectHTTP_BasicGet(t *testing.T) {
	l, addr := newTestListener(t, "https://bv.local:4000")

	// Run Accept loop in the background; HTTP connections are redirected
	// internally so Accept will block waiting for a TLS conn.
	go func() {
		c, _ := l.Accept()
		if c != nil {
			c.Close()
		}
	}()

	br := dialAndSend(t, addr, "GET /foo?bar=1 HTTP/1.1\r\nHost: localhost:4000\r\n\r\n")
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	want := "https://localhost:4000/foo?bar=1"
	if loc != want {
		t.Fatalf("Location = %q, want %q", loc, want)
	}
}

func TestRedirectHTTP_NoHostFallsBackToBase(t *testing.T) {
	l, addr := newTestListener(t, "https://bv.local:4000")

	go func() {
		c, _ := l.Accept()
		if c != nil {
			c.Close()
		}
	}()

	// HTTP/1.0 request without a Host header
	br := dialAndSend(t, addr, "GET /path HTTP/1.0\r\n\r\n")
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	want := "https://bv.local:4000/path"
	if loc != want {
		t.Fatalf("Location = %q, want %q", loc, want)
	}
}

func TestRedirectHTTP_EmptyConnectionClosesGracefully(t *testing.T) {
	l, addr := newTestListener(t, "https://bv.local:4000")

	go func() {
		c, _ := l.Accept()
		if c != nil {
			c.Close()
		}
	}()

	// Connect and immediately close without sending anything.
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	c.Close() // no data sent — listener should not panic
}

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
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// bufferedConn prepends already-read bytes in front of a net.Conn's read stream.
type bufferedConn struct {
	net.Conn
	r io.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) { return c.r.Read(b) }

// tlsOrRedirectListener wraps a net.Listener. For each accepted connection it
// peeks at the first byte to determine the protocol:
//   - 0x16 (TLS ClientHello): wraps the connection with TLS and returns it
//   - anything else (plain HTTP): parses the request and sends a 301 redirect
//
// This allows HTTP and HTTPS to share the same port without a second listener.
type tlsOrRedirectListener struct {
	net.Listener
	tlsConfig *tls.Config
	httpsBase string // e.g. "https://bv.local:4000"
}

// newTLSOrRedirectListener creates a tlsOrRedirectListener that loads the TLS
// key pair from certFile/keyFile and clones baseCfg to avoid mutating it.
func newTLSOrRedirectListener(ln net.Listener, certFile, keyFile string, baseCfg *tls.Config, httpsBase string) (*tlsOrRedirectListener, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	cfg := baseCfg.Clone()
	cfg.Certificates = []tls.Certificate{cert}
	return &tlsOrRedirectListener{Listener: ln, tlsConfig: cfg, httpsBase: httpsBase}, nil
}

// Accept waits for and returns the next connection. Plain HTTP connections are
// redirected internally; only TLS connections are returned to the caller.
func (l *tlsOrRedirectListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}

		first := make([]byte, 1)
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, err = io.ReadFull(c, first)
		_ = c.SetReadDeadline(time.Time{})
		if err != nil {
			c.Close()
			continue
		}

		if first[0] == 0x16 { // TLS ClientHello record type
			bc := &bufferedConn{Conn: c, r: io.MultiReader(bytes.NewReader(first), c)}
			return tls.Server(bc, l.tlsConfig), nil
		}

		go l.redirectHTTP(c, first[0])
	}
}

// redirectHTTP reads an HTTP request from c (with firstByte prepended) and
// responds with a 301 redirect to the equivalent HTTPS URL.
func (l *tlsOrRedirectListener) redirectHTTP(c net.Conn, firstByte byte) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))

	r := io.MultiReader(bytes.NewReader([]byte{firstByte}), c)
	req, err := http.ReadRequest(bufio.NewReader(r))
	if err != nil {
		return
	}

	host := req.Host
	if host == "" {
		host = strings.TrimPrefix(l.httpsBase, "https://")
	}
	target := "https://" + host + req.URL.RequestURI()
	fmt.Fprintf(c, "HTTP/1.1 301 Moved Permanently\r\nLocation: %s\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", target)
}

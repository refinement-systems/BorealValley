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
	"net"
	"net/http"
	"net/url"
	"strings"
)

type CSRFConfig struct {
	TrustedProxyCIDRs []*net.IPNet
	AllowedSchemes    map[string]bool
	AllowInsecure     bool
}

func mustParseCIDRs(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

func parseForwardedFirst(v string) (proto, host string, ok bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", "", false
	}
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ";")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.Trim(strings.TrimSpace(kv[1]), `"`)
		switch k {
		case "proto":
			proto = strings.ToLower(val)
		case "host":
			host = val
		}
	}
	return proto, host, (proto != "" || host != "")
}

func hostHasPort(host string) bool {
	if host == "" {
		return false
	}
	if strings.HasPrefix(host, "[") {
		return strings.Contains(host, "]:")
	}
	return strings.Count(host, ":") == 1
}

func splitHostPortDefault(host string, defaultPort string) (string, string) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", defaultPort
	}
	if h, p, err := net.SplitHostPort(host); err == nil {
		return h, p
	}
	if hostHasPort(host) {
		return host, defaultPort
	}
	return host, defaultPort
}

func normalizeHostPortForScheme(host, scheme string) string {
	h, p := splitHostPortDefault(host, "")
	if h == "" {
		return ""
	}
	if p == "" {
		return strings.ToLower(h)
	}
	if (scheme == "http" && p == "80") || (scheme == "https" && p == "443") {
		return strings.ToLower(h)
	}
	return strings.ToLower(net.JoinHostPort(h, p))
}

func isClientIPTrusted(clientIP string, trusted []*net.IPNet) bool {
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}
	for _, cidr := range trusted {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func effectiveSchemeHost(r *http.Request, cfg CSRFConfig) (scheme, host string) {
	scheme = "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host = r.Host

	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}
	if !isClientIPTrusted(remoteHost, cfg.TrustedProxyCIDRs) {
		return strings.ToLower(scheme), strings.ToLower(host)
	}

	if fProto, fHost, ok := parseForwardedFirst(r.Header.Get("Forwarded")); ok {
		if fProto != "" {
			scheme = fProto
		}
		if fHost != "" {
			host = fHost
		}
		return strings.ToLower(scheme), strings.ToLower(host)
	}
	if xfProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); xfProto != "" {
		scheme = strings.ToLower(strings.Split(xfProto, ",")[0])
	}
	if xfHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); xfHost != "" {
		host = strings.TrimSpace(strings.Split(xfHost, ",")[0])
	}
	return strings.ToLower(scheme), strings.ToLower(host)
}

func originOrRefererURL(r *http.Request) (*url.URL, bool) {
	if o := strings.TrimSpace(r.Header.Get("Origin")); o != "" {
		u, err := url.Parse(o)
		if err != nil {
			return nil, false
		}
		if u.Scheme == "" || u.Host == "" {
			return nil, false
		}
		return u, true
	}
	if rf := strings.TrimSpace(r.Header.Get("Referer")); rf != "" {
		u, err := url.Parse(rf)
		if err != nil {
			return nil, false
		}
		if u.Scheme == "" || u.Host == "" {
			return nil, false
		}
		return u, true
	}
	return nil, false
}

func OriginRefererCSRF(cfg CSRFConfig, next http.Handler) http.Handler {
	unsafeMethod := map[string]bool{
		http.MethodPost:   true,
		http.MethodPut:    true,
		http.MethodPatch:  true,
		http.MethodDelete: true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !unsafeMethod[r.Method] {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/") || r.URL.Path == "/oauth/token" || r.URL.Path == "/oauth/revoke" {
			next.ServeHTTP(w, r)
			return
		}

		effScheme, effHost := effectiveSchemeHost(r, cfg)
		if !cfg.AllowInsecure && effScheme != "https" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if !cfg.AllowedSchemes[effScheme] {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		srcURL, ok := originOrRefererURL(r)
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		srcScheme := strings.ToLower(srcURL.Scheme)
		srcHost := normalizeHostPortForScheme(srcURL.Host, srcScheme)
		effHostNorm := normalizeHostPortForScheme(effHost, effScheme)

		if srcScheme != effScheme || srcHost != effHostNorm {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

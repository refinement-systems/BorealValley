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
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/refinement-systems/BorealValley/src/internal/assets"
	"github.com/refinement-systems/BorealValley/src/internal/common"
	commonoauth "github.com/refinement-systems/BorealValley/src/internal/common/oauth"
)

var homeTmpl = template.Must(template.New("home").Parse(assets.HtmlHome))
var loginTmpl = template.Must(template.New("login").Parse(assets.HtmlLogin))

type CSRFConfig struct {
	TrustedProxyCIDRs []*net.IPNet
	AllowedSchemes    map[string]bool
	AllowInsecure     bool
}

type application struct {
	store          *common.Store
	oauth          *commonoauth.Runtime
	sessionManager *scs.SessionManager
	repoRoot       string
	repoPathMapper *repoPathMapper
}

const (
	defaultLocalCertFile = "cert/bv.local+3.pem"
	defaultLocalKeyFile  = "cert/bv.local+3-key.pem"
)

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintf(os.Stderr, "  %q serve [OPTIONS]\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  --root PATH")
	fmt.Fprintf(os.Stderr, "    Root directory path. (default: %q)\n", common.RootPathDefault())
	fmt.Fprintln(os.Stderr, "  --pg-dsn DSN")
	fmt.Fprintf(os.Stderr, "    PostgreSQL DSN (or set %s).\n", common.PostgresDSNEnv)
	fmt.Fprintln(os.Stderr, "  --env ENV")
	fmt.Fprintln(os.Stderr, "    Environment mode: dev or prod. (default: dev)")
	fmt.Fprintln(os.Stderr, "  --cert FILE")
	fmt.Fprintln(os.Stderr, "    TLS certificate file. If set along with --key, enables HTTPS (TLS 1.3+).")
	fmt.Fprintln(os.Stderr, "  --key FILE")
	fmt.Fprintln(os.Stderr, "    TLS key file. If set along with --cert, enables HTTPS (TLS 1.3+).")
	fmt.Fprintln(os.Stderr, "  --verbosity N")
	fmt.Fprintln(os.Stderr, "    Log verbosity: 4=debug, 3=info, 2=warning, 1=error, 0=silent. (default: 3)")
}

func newHandler(rootDir string, pgDSN string, isDev bool) (http.Handler, *common.Store, error) {
	repoPathMapper, err := loadRepoPathMapperFromEnv()
	if err != nil {
		return nil, nil, err
	}

	store, err := common.StoreInit(pgDSN, rootDir)
	if err != nil {
		return nil, nil, err
	}
	if err := store.ResyncFromFilesystem(context.Background()); err != nil {
		store.Close()
		return nil, nil, err
	}

	sm := scs.New()
	sm.Lifetime = 24 * time.Hour
	sm.IdleTimeout = 30 * time.Minute
	sm.Cookie.Path = "/"
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteStrictMode

	trusted := mustParseCIDRs([]string{"127.0.0.1/32", "::1/128"})
	cfg := CSRFConfig{
		TrustedProxyCIDRs: trusted,
		AllowedSchemes:    map[string]bool{"https": true},
		AllowInsecure:     false,
	}

	if isDev {
		sm.Cookie.Secure = false
		sm.Cookie.Name = "session"
		cfg.AllowedSchemes = map[string]bool{"http": true, "https": true}
		cfg.AllowInsecure = true
	} else {
		sm.Cookie.Secure = true
		sm.Cookie.Name = "__Host-session"
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		store.Close()
		return nil, nil, fmt.Errorf("getwd: %w", err)
	}

	oauthRuntime, err := commonoauth.NewRuntime(context.Background(), rootDir, store.BaseURL(), store)
	if err != nil {
		store.Close()
		return nil, nil, err
	}

	app := &application{store: store, oauth: oauthRuntime, sessionManager: sm, repoRoot: repoRoot, repoPathMapper: repoPathMapper}
	mux := http.NewServeMux()
	registerRoutes(mux, app)

	var handler http.Handler = mux
	handler = OriginRefererCSRF(cfg, handler)
	handler = sm.LoadAndSave(handler)
	return handler, store, nil
}

func registerRoutes(mux *http.ServeMux, app *application) {
	// Restrict embedded assets to GET/HEAD and avoid method-pattern conflicts with root.
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(assets.StaticFiles))))
	mux.HandleFunc("/", app.root)
	mux.HandleFunc("GET /web", app.web)
	mux.HandleFunc("/web/login", app.login)
	mux.HandleFunc("/web/logout", app.logout)

	mux.HandleFunc("GET /.well-known/oauth-authorization-server", app.oauthAuthorizationServerMetadata)
	mux.HandleFunc("GET /oauth/authorize", app.oauthAuthorizeGet)
	mux.HandleFunc("POST /oauth/authorize", app.oauthAuthorizePost)
	mux.HandleFunc("POST /oauth/token", app.oauthToken)
	mux.HandleFunc("POST /oauth/revoke", app.oauthRevoke)

	mux.HandleFunc("/web/admin", app.requireAuth(app.home))
	mux.HandleFunc("GET /users/{name}", app.requireAuth(app.objectUser))
	mux.HandleFunc("GET /repo/{repo}", app.requireAuth(app.objectRepo))
	mux.HandleFunc("GET /ticket-tracker/{tracker}", app.requireAuth(app.objectTicketTracker))
	mux.HandleFunc("GET /ticket-tracker/{tracker}/ticket/{ticket}", app.requireAuth(app.objectTicket))
	mux.HandleFunc("GET /ticket-tracker/{tracker}/ticket/{ticket}/comment/{comment}", app.requireAuth(app.objectTicketComment))
	mux.HandleFunc("GET /web/user/{name}", app.requireAuth(app.userCtl))
	mux.HandleFunc("GET /web/repo", app.requireAuth(app.dataList))
	mux.HandleFunc("GET /web/repo/{repo}", app.requireAuth(app.dataRepo))
	mux.HandleFunc("/web/repo/{repo}/ticket-tracker", app.requireAuth(app.dataRepoTicketTracker))
	mux.HandleFunc("POST /web/repo/{repo}/member", app.requireAuth(app.dataRepoMember))
	mux.HandleFunc("/web/ticket-tracker", app.requireAuth(app.dataTicketTrackerList))
	mux.HandleFunc("GET /web/ticket-tracker/{tracker}", app.requireAuth(app.dataTicketTrackerDetail))
	mux.HandleFunc("POST /web/ticket-tracker/{tracker}/ticket", app.requireAuth(app.dataTicketTrackerTicket))
	mux.HandleFunc("POST /web/ticket-tracker/{tracker}/ticket/{ticket}/comment", app.requireAuth(app.dataTicketComment))
	mux.HandleFunc("POST /web/ticket-tracker/{tracker}/ticket/{ticket}/assignee", app.requireAuth(app.dataTicketAssignee))
	mux.HandleFunc("GET /web/ticket", app.requireAuth(app.dataTicketList))
	mux.HandleFunc("GET /web/notification", app.requireAuth(app.dataNotificationList))
	mux.HandleFunc("POST /web/notification/clear", app.requireAuth(app.dataNotificationClear))
	mux.HandleFunc("POST /web/notification/reset", app.requireAuth(app.dataNotificationReset))
	mux.HandleFunc("POST /web/notification/{notification}", app.requireAuth(app.dataNotificationUpdate))
	mux.HandleFunc("GET /web/oauth/grant", app.requireAuth(app.oauthGrantAdminList))
	mux.HandleFunc("POST /web/oauth/grant/{grant}/revoke", app.requireAuth(app.oauthGrantAdminRevoke))

	mux.HandleFunc("GET /api/v1/profile", app.requireOAuthBearer(app.apiV1Profile, "profile:read"))
	mux.HandleFunc("GET /api/v1/repo", app.requireOAuthBearer(app.apiV1RepoList, "repo:read"))
	mux.HandleFunc("GET /api/v1/repo/{repo}", app.requireOAuthBearer(app.apiV1RepoDetail, "repo:read"))
	mux.HandleFunc("GET /api/v1/ticket-tracker", app.requireOAuthBearer(app.apiV1TicketTrackerList, "tracker:read"))
	mux.HandleFunc("POST /api/v1/ticket-tracker", app.requireOAuthBearer(app.apiV1TicketTrackerCreate, "tracker:write"))
	mux.HandleFunc("GET /api/v1/ticket-tracker/{tracker}", app.requireOAuthBearer(app.apiV1TicketTrackerDetail, "tracker:read"))
	mux.HandleFunc("POST /api/v1/repo/{repo}/ticket-tracker", app.requireOAuthBearer(app.apiV1RepoTicketTrackerAssign, "repo:write", "tracker:write"))
	mux.HandleFunc("GET /api/v1/ticket-tracker/{tracker}/ticket", app.requireOAuthBearer(app.apiV1TicketList, "ticket:read"))
	mux.HandleFunc("GET /api/v1/ticket/assigned", app.requireOAuthBearer(app.apiV1TicketAssignedList, "ticket:read"))
	mux.HandleFunc("POST /api/v1/ticket-tracker/{tracker}/ticket", app.requireOAuthBearer(app.apiV1TicketCreate, "ticket:write"))
	mux.HandleFunc("POST /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/update", app.requireOAuthBearer(app.apiV1TicketUpdateCreate, "ticket:write"))
	mux.HandleFunc("GET /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/version", app.requireOAuthBearer(app.apiV1TicketVersionList, "ticket:read"))
	mux.HandleFunc("GET /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/comment", app.requireOAuthBearer(app.apiV1TicketCommentList, "ticket:read"))
	mux.HandleFunc("POST /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/comment", app.requireOAuthBearer(app.apiV1TicketCommentCreate, "ticket:write"))
	mux.HandleFunc("POST /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/comment/{comment}/update", app.requireOAuthBearer(app.apiV1TicketCommentUpdateCreate, "ticket:write"))
	mux.HandleFunc("GET /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/comment/{comment}/version", app.requireOAuthBearer(app.apiV1TicketCommentVersionList, "ticket:read"))
	mux.HandleFunc("GET /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/assignee", app.requireOAuthBearer(app.apiV1TicketAssigneeList, "ticket:read"))
	mux.HandleFunc("POST /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/assignee", app.requireOAuthBearer(app.apiV1TicketAssigneeUpdate, "ticket:write"))
	mux.HandleFunc("GET /api/v1/notification", app.requireOAuthBearer(app.apiV1NotificationList, "notification:read"))
	mux.HandleFunc("POST /api/v1/notification/clear", app.requireOAuthBearer(app.apiV1NotificationClear, "notification:write"))
	mux.HandleFunc("POST /api/v1/notification/reset", app.requireOAuthBearer(app.apiV1NotificationReset, "notification:write"))
	mux.HandleFunc("POST /api/v1/notification/{notification}", app.requireOAuthBearer(app.apiV1NotificationUpdate, "notification:write"))
	mux.HandleFunc("GET /api/v1/repo/{repo}/member", app.requireOAuthBearer(app.apiV1RepoMemberList, "repo:write"))
	mux.HandleFunc("POST /api/v1/repo/{repo}/member", app.requireOAuthBearer(app.apiV1RepoMemberUpdate, "repo:write"))
	mux.HandleFunc("GET /api/v1/object-count", app.requireOAuthBearer(app.apiV1ObjectCount, "repo:read"))
	mux.HandleFunc("GET /api/v1/oauth/grant", app.requireOAuthBearer(app.apiV1OAuthGrantList, "profile:read"))
	mux.HandleFunc("POST /api/v1/oauth/grant/{grant}/revoke", app.requireOAuthBearer(app.apiV1OAuthGrantRevoke, "profile:read"))
}

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	rootDir := fs.String("root", common.RootPathDefault(), "root directory")
	pgDSNFlag := fs.String("pg-dsn", "", "postgres dsn")
	env := fs.String("env", "dev", "dev or prod")
	certFile := fs.String("cert", "", "TLS certificate file (enables HTTPS)")
	keyFile := fs.String("key", "", "TLS key file (enables HTTPS)")
	verbosity := fs.Int("verbosity", 3, "log verbosity (4=debug, 3=info, 2=warning, 1=error, 0=silent)")
	if err := fs.Parse(args); err != nil {
		usage()
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}

	if err := common.ConfigureLogging(*verbosity); err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage()
		os.Exit(2)
	}

	pgDSN, err := common.ResolvePostgresDSN(*pgDSNFlag)
	if err != nil {
		slog.Error("postgres dsn missing", "err", err)
		os.Exit(2)
	}

	handler, store, err := newHandler(*rootDir, pgDSN, *env == "dev")
	if err != nil {
		slog.Error("failed to initialize store", "root", *rootDir, "err", err)
		os.Exit(1)
	}
	defer store.Close()

	cfg := store.Config()
	addr := ":" + strconv.Itoa(cfg.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
		},
	}

	certPath, keyPath, useTLS := resolveTLSFiles(*certFile, *keyFile, *env == "dev", pathExists)
	slog.Info("listening", "addr", addr, "env", *env, "root", *rootDir, "hostname", cfg.Hostname, "tls", useTLS)
	var serverErr error
	if useTLS {
		serverErr = srv.ListenAndServeTLS(certPath, keyPath)
	} else {
		serverErr = srv.ListenAndServe()
	}
	if serverErr != nil {
		slog.Error("server stopped", "err", serverErr)
		os.Exit(1)
	}
}

func (app *application) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if app.sessionManager.Exists(r.Context(), "user_id") {
		http.Redirect(w, r, "/web/admin", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/web/login", http.StatusSeeOther)
}

func (app *application) web(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.Redirect(w, r, "/web/admin", http.StatusSeeOther)
}

func resolveTLSFiles(certFlag string, keyFlag string, isDev bool, fileExists func(path string) bool) (certFile string, keyFile string, useTLS bool) {
	certFlag = strings.TrimSpace(certFlag)
	keyFlag = strings.TrimSpace(keyFlag)
	if certFlag != "" || keyFlag != "" {
		return certFlag, keyFlag, true
	}
	if isDev && fileExists(defaultLocalCertFile) && fileExists(defaultLocalKeyFile) {
		return defaultLocalCertFile, defaultLocalKeyFile, true
	}
	return "", "", false
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	repositories, err := app.store.ListRepositories(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	counts, err := app.store.ListObjectTypeCounts(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = homeTmpl.Execute(w, struct {
		Repositories []common.Repository
		Counts       []common.ObjectTypeCount
	}{Repositories: repositories, Counts: counts})
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

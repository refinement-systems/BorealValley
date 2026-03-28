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
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

type repeatedStringFlag []string

func (f *repeatedStringFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *repeatedStringFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func oauthAppCmd(args []string) {
	if len(args) == 0 {
		oauthAppUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "create":
		oauthAppCreateCmd(args[1:])
	case "rotate-secret":
		oauthAppRotateSecretCmd(args[1:])
	case "enable":
		oauthAppEnableCmd(args[1:])
	case "disable":
		oauthAppDisableCmd(args[1:])
	case "list":
		oauthAppListCmd(args[1:])
	case "show":
		oauthAppShowCmd(args[1:])
	default:
		oauthAppUsage()
		os.Exit(2)
	}
}

func oauthAppUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintf(os.Stderr, "  %q oauth-app ACTION [OPTIONS]\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Actions:")
	fmt.Fprintln(os.Stderr, "  create --name NAME --redirect-uri URI [--redirect-uri URI] --scope SCOPE [--scope SCOPE] [--description TEXT]")
	fmt.Fprintln(os.Stderr, "  rotate-secret --client-id ID")
	fmt.Fprintln(os.Stderr, "  enable --client-id ID")
	fmt.Fprintln(os.Stderr, "  disable --client-id ID")
	fmt.Fprintln(os.Stderr, "  list")
	fmt.Fprintln(os.Stderr, "  show --client-id ID")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Common options:")
	fmt.Fprintln(os.Stderr, "  --root PATH")
	fmt.Fprintf(os.Stderr, "    Root directory path. (default: %q)\n", common.RootPathDefault())
	fmt.Fprintln(os.Stderr, "  --pg-dsn DSN")
	fmt.Fprintf(os.Stderr, "    PostgreSQL DSN (or set %s).\n", common.PostgresDSNEnv)
	fmt.Fprintln(os.Stderr, "  --verbosity N")
	fmt.Fprintln(os.Stderr, "    Log verbosity: 4=debug, 3=info, 2=warning, 1=error, 0=silent. (default: 3)")
}

func oauthAppCreateCmd(args []string) {
	fs := flag.NewFlagSet("oauth-app create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", common.RootPathDefault(), "root path")
	pgDSNFlag := fs.String("pg-dsn", "", "postgres dsn")
	verbosity := fs.Int("verbosity", 3, "log verbosity")
	name := fs.String("name", "", "client display name")
	description := fs.String("description", "", "client description")
	var redirectURIs repeatedStringFlag
	var scopes repeatedStringFlag
	fs.Var(&redirectURIs, "redirect-uri", "allowed redirect uri")
	fs.Var(&scopes, "scope", "allowed scope")
	if err := fs.Parse(args); err != nil {
		oauthAppUsage()
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}
	if err := common.ConfigureLogging(*verbosity); err != nil {
		fmt.Fprintln(os.Stderr, err)
		oauthAppUsage()
		os.Exit(2)
	}
	pgDSN, err := common.ResolvePostgresDSN(*pgDSNFlag)
	if err != nil {
		slog.Error("postgres dsn missing", "err", err)
		os.Exit(2)
	}
	store, err := common.StoreInit(pgDSN, *root)
	if err != nil {
		slog.Error("failed to initialize store", "root", *root, "err", err)
		os.Exit(1)
	}
	defer store.Close()
	client, err := store.CreateOAuthClient(context.Background(), *name, *description, redirectURIs, scopes)
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		slog.Error("oauth app create failed", "err", err)
		os.Exit(1)
	}
	fmt.Printf("client_id=%s\n", client.ClientID)
	fmt.Printf("client_secret=%s\n", client.ClientSecret)
	fmt.Println("warning: client secret is shown only once")
}

func oauthAppRotateSecretCmd(args []string) {
	clientID, store := oauthAppStoreWithClientID(args)
	defer store.Close()
	secret, err := store.RotateOAuthClientSecret(context.Background(), clientID)
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		slog.Error("oauth app rotate-secret failed", "client_id", clientID, "err", err)
		os.Exit(1)
	}
	fmt.Printf("client_id=%s\n", clientID)
	fmt.Printf("client_secret=%s\n", secret)
	fmt.Println("warning: client secret is shown only once")
}

func oauthAppEnableCmd(args []string) {
	clientID, store := oauthAppStoreWithClientID(args)
	defer store.Close()
	if err := store.SetOAuthClientEnabled(context.Background(), clientID, true); err != nil {
		if errors.Is(err, common.ErrValidation) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		slog.Error("oauth app enable failed", "client_id", clientID, "err", err)
		os.Exit(1)
	}
	fmt.Printf("enabled client_id=%s\n", clientID)
}

func oauthAppDisableCmd(args []string) {
	clientID, store := oauthAppStoreWithClientID(args)
	defer store.Close()
	if err := store.SetOAuthClientEnabled(context.Background(), clientID, false); err != nil {
		if errors.Is(err, common.ErrValidation) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		slog.Error("oauth app disable failed", "client_id", clientID, "err", err)
		os.Exit(1)
	}
	fmt.Printf("disabled client_id=%s\n", clientID)
}

func oauthAppListCmd(args []string) {
	store := oauthAppStoreNoClientID(args)
	defer store.Close()
	clients, err := store.ListOAuthClients(context.Background())
	if err != nil {
		slog.Error("oauth app list failed", "err", err)
		os.Exit(1)
	}
	sort.Slice(clients, func(i, j int) bool { return clients[i].ClientID < clients[j].ClientID })
	for _, client := range clients {
		fmt.Printf("%s\t%s\tenabled=%t\n", client.ClientID, client.Name, client.Enabled)
	}
}

func oauthAppShowCmd(args []string) {
	clientID, store := oauthAppStoreWithClientID(args)
	defer store.Close()
	client, found, err := store.GetOAuthClient(context.Background(), clientID)
	if err != nil {
		slog.Error("oauth app show failed", "client_id", clientID, "err", err)
		os.Exit(1)
	}
	if !found {
		slog.Error("oauth app not found", "client_id", clientID)
		os.Exit(1)
	}
	fmt.Printf("client_id=%s\n", client.ClientID)
	fmt.Printf("name=%s\n", client.Name)
	fmt.Printf("description=%s\n", client.Description)
	fmt.Printf("enabled=%t\n", client.Enabled)
	fmt.Printf("redirect_uris=%s\n", strings.Join(client.RedirectURIs, ","))
	fmt.Printf("allowed_scopes=%s\n", strings.Join(client.AllowedScopes, ","))
	fmt.Printf("created_at=%s\n", client.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"))
	fmt.Printf("updated_at=%s\n", client.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"))
}

func oauthAppStoreWithClientID(args []string) (string, *common.Store) {
	fs := flag.NewFlagSet("oauth-app", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", common.RootPathDefault(), "root path")
	pgDSNFlag := fs.String("pg-dsn", "", "postgres dsn")
	verbosity := fs.Int("verbosity", 3, "log verbosity")
	clientID := fs.String("client-id", "", "oauth client id")
	if err := fs.Parse(args); err != nil {
		oauthAppUsage()
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}
	if err := common.ConfigureLogging(*verbosity); err != nil {
		fmt.Fprintln(os.Stderr, err)
		oauthAppUsage()
		os.Exit(2)
	}
	if strings.TrimSpace(*clientID) == "" {
		oauthAppUsage()
		os.Exit(2)
	}
	pgDSN, err := common.ResolvePostgresDSN(*pgDSNFlag)
	if err != nil {
		slog.Error("postgres dsn missing", "err", err)
		os.Exit(2)
	}
	store, err := common.StoreInit(pgDSN, *root)
	if err != nil {
		slog.Error("failed to initialize store", "root", *root, "err", err)
		os.Exit(1)
	}
	return strings.TrimSpace(*clientID), store
}

func oauthAppStoreNoClientID(args []string) *common.Store {
	fs := flag.NewFlagSet("oauth-app", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", common.RootPathDefault(), "root path")
	pgDSNFlag := fs.String("pg-dsn", "", "postgres dsn")
	verbosity := fs.Int("verbosity", 3, "log verbosity")
	if err := fs.Parse(args); err != nil {
		oauthAppUsage()
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}
	if err := common.ConfigureLogging(*verbosity); err != nil {
		fmt.Fprintln(os.Stderr, err)
		oauthAppUsage()
		os.Exit(2)
	}
	pgDSN, err := common.ResolvePostgresDSN(*pgDSNFlag)
	if err != nil {
		slog.Error("postgres dsn missing", "err", err)
		os.Exit(2)
	}
	store, err := common.StoreInit(pgDSN, *root)
	if err != nil {
		slog.Error("failed to initialize store", "root", *root, "err", err)
		os.Exit(1)
	}
	return store
}

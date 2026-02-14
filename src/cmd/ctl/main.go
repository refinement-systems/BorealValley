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

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "-h", "-help", "--help":
		usage()
		os.Exit(0)
	case "init-root":
		initRootCmd(os.Args[2:])
	case "resync":
		resyncCmd(os.Args[2:])
	case "adduser":
		addUserCmd(os.Args[2:])
	case "oauth-app":
		oauthAppCmd(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintf(os.Stderr, "  %q COMMAND [OPTIONS]\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  init-root")
	fmt.Fprintln(os.Stderr, "    Initialize $ROOT with config.json and repo directory.")
	fmt.Fprintln(os.Stderr, "  resync")
	fmt.Fprintln(os.Stderr, "    Scan $ROOT/repo and upsert local repository objects.")
	fmt.Fprintln(os.Stderr, "  adduser USER PASSWORD")
	fmt.Fprintln(os.Stderr, "    Add a local user with host-based actor ID.")
	fmt.Fprintln(os.Stderr, "  oauth-app ACTION")
	fmt.Fprintln(os.Stderr, "    Manage OAuth third-party app registrations.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Common options:")
	fmt.Fprintln(os.Stderr, "  --root PATH")
	fmt.Fprintf(os.Stderr, "    Root directory path. (default: %q)\n", common.RootPathDefault())
	fmt.Fprintln(os.Stderr, "  --pg-dsn DSN")
	fmt.Fprintf(os.Stderr, "    PostgreSQL DSN (or set %s).\n", common.PostgresDSNEnv)
	fmt.Fprintln(os.Stderr, "  --verbosity N")
	fmt.Fprintln(os.Stderr, "    Log verbosity: 4=debug, 3=info, 2=warning, 1=error, 0=silent. (default: 3)")
}

func initRootCmd(args []string) {
	fs := flag.NewFlagSet("init-root", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", common.RootPathDefault(), "root path")
	verbosity := fs.Int("verbosity", 3, "log verbosity (4=debug, 3=info, 2=warning, 1=error, 0=silent)")
	if err := fs.Parse(args); err != nil {
		initRootUsage()
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}
	if err := common.ConfigureLogging(*verbosity); err != nil {
		fmt.Fprintln(os.Stderr, err)
		initRootUsage()
		os.Exit(2)
	}

	if err := common.InitRoot(*root); err != nil {
		slog.Error("failed to initialize root", "root", *root, "err", err)
		os.Exit(1)
	}

	slog.Info("initialized root", "root", *root)
}

func initRootUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintf(os.Stderr, "  %q init-root [OPTIONS]\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  --root PATH")
	fmt.Fprintf(os.Stderr, "    Root directory path. (default: %q)\n", common.RootPathDefault())
	fmt.Fprintln(os.Stderr, "  --verbosity N")
	fmt.Fprintln(os.Stderr, "    Log verbosity: 4=debug, 3=info, 2=warning, 1=error, 0=silent. (default: 3)")
}

func resyncCmd(args []string) {
	fs := flag.NewFlagSet("resync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", common.RootPathDefault(), "root path")
	pgDSNFlag := fs.String("pg-dsn", "", "postgres dsn")
	verbosity := fs.Int("verbosity", 3, "log verbosity (4=debug, 3=info, 2=warning, 1=error, 0=silent)")
	if err := fs.Parse(args); err != nil {
		resyncUsage()
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}

	if err := common.ConfigureLogging(*verbosity); err != nil {
		fmt.Fprintln(os.Stderr, err)
		resyncUsage()
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

	if err := store.ResyncFromFilesystem(context.Background()); err != nil {
		slog.Error("resync failed", "root", *root, "err", err)
		os.Exit(1)
	}

	slog.Info("resync complete", "root", *root)
}

func resyncUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintf(os.Stderr, "  %q resync [OPTIONS]\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  --root PATH")
	fmt.Fprintf(os.Stderr, "    Root directory path. (default: %q)\n", common.RootPathDefault())
	fmt.Fprintln(os.Stderr, "  --pg-dsn DSN")
	fmt.Fprintf(os.Stderr, "    PostgreSQL DSN (or set %s).\n", common.PostgresDSNEnv)
	fmt.Fprintln(os.Stderr, "  --verbosity N")
	fmt.Fprintln(os.Stderr, "    Log verbosity: 4=debug, 3=info, 2=warning, 1=error, 0=silent. (default: 3)")
}

func addUserCmd(args []string) {
	fs := flag.NewFlagSet("adduser", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", common.RootPathDefault(), "root path")
	pgDSNFlag := fs.String("pg-dsn", "", "postgres dsn")
	isAdmin := fs.Bool("admin", false, "create admin user")
	verbosity := fs.Int("verbosity", 3, "log verbosity (4=debug, 3=info, 2=warning, 1=error, 0=silent)")
	if err := fs.Parse(args); err != nil {
		addUserUsage()
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}

	if err := common.ConfigureLogging(*verbosity); err != nil {
		fmt.Fprintln(os.Stderr, err)
		addUserUsage()
		os.Exit(2)
	}

	username := fs.Arg(0)
	password := fs.Arg(1)
	if username == "" || password == "" {
		addUserUsage()
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

	if err := store.CreateUserWithAdmin(context.Background(), username, password, *isAdmin); err != nil {
		slog.Error("failed to create user", "username", username, "err", err)
		os.Exit(1)
	}

	slog.Info("created user", "username", username, "admin", *isAdmin)
}

func addUserUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintf(os.Stderr, "  %q adduser [OPTIONS] USER PASSWORD\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  --root PATH")
	fmt.Fprintf(os.Stderr, "    Root directory path. (default: %q)\n", common.RootPathDefault())
	fmt.Fprintln(os.Stderr, "  --pg-dsn DSN")
	fmt.Fprintf(os.Stderr, "    PostgreSQL DSN (or set %s).\n", common.PostgresDSNEnv)
	fmt.Fprintln(os.Stderr, "  --admin")
	fmt.Fprintln(os.Stderr, "    Create the user with admin privileges.")
	fmt.Fprintln(os.Stderr, "  --verbosity N")
	fmt.Fprintln(os.Stderr, "    Log verbosity: 4=debug, 3=info, 2=warning, 1=error, 0=silent. (default: 3)")
}

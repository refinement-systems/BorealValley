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
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "-h", "--help", "help":
		usage()
		os.Exit(0)
	case "init":
		initCmd(os.Args[2:])
	case "run":
		runCmd(os.Args[2:])
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
	fmt.Fprintln(os.Stderr, "  init")
	fmt.Fprintln(os.Stderr, "    Perform OAuth login and save agent state.")
	fmt.Fprintln(os.Stderr, "  run")
	fmt.Fprintln(os.Stderr, "    Process one assigned ticket and publish updates.")
}

func initCmd(args []string) {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	serverURL := fs.String("server-url", "", "BorealValley server URL")
	clientID := fs.String("client-id", "", "OAuth client id")
	clientSecret := fs.String("client-secret", "", "OAuth client secret")
	redirectURI := fs.String("redirect-uri", "", "OAuth redirect URI")
	mode := fs.String("mode", agentModeLMStudio, "agent mode (lmstudio or test-counter)")
	model := fs.String("model", "", "LM Studio model name")
	lmstudioURL := fs.String("lmstudio-url", "http://127.0.0.1:1234", "LM Studio base URL")
	stateFile := fs.String("state-file", "", "state file path")
	noOpenBrowser := fs.Bool("no-open-browser", false, "print authorize URL but do not open a browser automatically")
	reuseSession := fs.Bool("reuse-session", false, "reuse the current browser login session instead of forcing a fresh login")
	verbosity := fs.Int("verbosity", 3, "log verbosity")
	if err := fs.Parse(args); err != nil {
		initUsage()
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}
	if err := common.ConfigureLogging(*verbosity); err != nil {
		fmt.Fprintln(os.Stderr, err)
		initUsage()
		os.Exit(2)
	}

	statePath, err := resolveStatePath(*stateFile)
	if err != nil {
		slog.Error("resolve state path failed", "err", err)
		os.Exit(2)
	}

	cfg := initConfig{
		ServerURL:     *serverURL,
		ClientID:      *clientID,
		ClientSecret:  *clientSecret,
		RedirectURI:   *redirectURI,
		Mode:          *mode,
		Model:         *model,
		LMStudioURL:   *lmstudioURL,
		StatePath:     statePath,
		NoOpenBrowser: *noOpenBrowser,
		ReuseSession:  *reuseSession,
	}
	if err := runInit(cfg); err != nil {
		slog.Error("agent init failed", "err", err)
		os.Exit(1)
	}
	fmt.Printf("initialized agent state: %s\n", statePath)
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	stateFile := fs.String("state-file", "", "state file path")
	workspace := fs.String("workspace", "", "workspace path for tool sandbox")
	maxIter := fs.Int("max-iter", 3, "max model round-trips")
	mode := fs.String("mode", "", "override mode (lmstudio or test-counter)")
	model := fs.String("model", "", "override model name")
	lmstudioURL := fs.String("lmstudio-url", "", "override LM Studio base URL")
	verbosity := fs.Int("verbosity", 3, "log verbosity")
	if err := fs.Parse(args); err != nil {
		runUsage()
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}
	if err := common.ConfigureLogging(*verbosity); err != nil {
		fmt.Fprintln(os.Stderr, err)
		runUsage()
		os.Exit(2)
	}
	if *maxIter <= 0 {
		slog.Error("max-iter must be positive")
		os.Exit(2)
	}

	statePath, err := resolveStatePath(*stateFile)
	if err != nil {
		slog.Error("resolve state path failed", "err", err)
		os.Exit(2)
	}
	workspacePath := *workspace
	if workspacePath == "" {
		workspacePath, err = os.Getwd()
		if err != nil {
			slog.Error("getwd failed", "err", err)
			os.Exit(1)
		}
	}
	workspacePath, err = filepath.Abs(workspacePath)
	if err != nil {
		slog.Error("resolve workspace failed", "err", err)
		os.Exit(2)
	}

	cfg := runConfig{
		StatePath:   statePath,
		Workspace:   workspacePath,
		MaxIter:     *maxIter,
		Mode:        *mode,
		Model:       *model,
		LMStudioURL: *lmstudioURL,
	}
	if err := runAgentOnce(cfg); err != nil {
		slog.Error("agent run failed", "err", err)
		os.Exit(1)
	}
}

func initUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintf(os.Stderr, "  %q init [OPTIONS]\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  --server-url URL")
	fmt.Fprintln(os.Stderr, "  --client-id ID")
	fmt.Fprintln(os.Stderr, "  --client-secret SECRET")
	fmt.Fprintln(os.Stderr, "  --redirect-uri URI")
	fmt.Fprintln(os.Stderr, "  --mode MODE (default lmstudio; values: lmstudio, test-counter)")
	fmt.Fprintln(os.Stderr, "  --model NAME")
	fmt.Fprintln(os.Stderr, "  --lmstudio-url URL (default http://127.0.0.1:1234)")
	fmt.Fprintln(os.Stderr, "  --state-file PATH")
	fmt.Fprintln(os.Stderr, "  --no-open-browser")
	fmt.Fprintln(os.Stderr, "  --reuse-session")
	fmt.Fprintln(os.Stderr, "  --verbosity N")
}

func runUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintf(os.Stderr, "  %q run [OPTIONS]\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  --state-file PATH")
	fmt.Fprintln(os.Stderr, "  --workspace PATH")
	fmt.Fprintln(os.Stderr, "  --max-iter N (default 3)")
	fmt.Fprintln(os.Stderr, "  --mode MODE")
	fmt.Fprintln(os.Stderr, "  --model NAME")
	fmt.Fprintln(os.Stderr, "  --lmstudio-url URL")
	fmt.Fprintln(os.Stderr, "  --verbosity N")
}

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/Edwin-metcalf/MetCode/internal/agent"
	"github.com/Edwin-metcalf/MetCode/internal/prompt"
	"github.com/Edwin-metcalf/MetCode/internal/ui"
	"github.com/Edwin-metcalf/MetCode/internal/verify"
	"github.com/joho/godotenv"
)

const defaultHost = "http://localhost:11434"

// version is set at build time with:
//
//	go build -ldflags "-X main.version=v0.1.0" ./cmd/metcode
var version = "dev"

// wantsVersion reports whether --version or -v was passed.
func wantsVersion(args []string) bool {
	return slices.Contains(args, "--version") || slices.Contains(args, "-v")
}

// Resolve Version prefers the ldflags-stamped verison, then the module version
// version Go embeds for go install ...@vX.Y.Z, if those are not there defaults to "dev"
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return version
}
func printVersion(w io.Writer) {
	fmt.Fprintln(w, resolveVersion())
}

type cliFlags struct {
	Host   string
	Verify string
}

// parseFlags reads --host and --verify. Go's flag package also accepts
// --host=value and the single-dash spelling.
func parseFlags(args []string) (cliFlags, error) {
	fs := flag.NewFlagSet("metcode", flag.ContinueOnError)
	host := fs.String("host", "", "ollama URL (default: OLLAMA_HOST, then "+defaultHost+")")
	verifyCmd := fs.String("verify", "", "command to run after an edit, e.g. \"go test ./...\"")
	if err := fs.Parse(args); err != nil {
		return cliFlags{}, err
	}
	return cliFlags{Host: *host, Verify: *verifyCmd}, nil
}

func parseHostFlag(args []string) (string, error) {
	flags, err := parseFlags(args)
	return flags.Host, err
}

func parseVerifyCommand(value string) ([]string, error) {
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return nil, errors.New("verify command is empty")
	}
	return parts, nil
}

// resolveHost applies the documented precedence: --host, then OLLAMA_HOST
// (including values loaded from .env), then the local default.
func resolveHost(flagHost, envHost string) string {
	if flagHost != "" {
		return flagHost
	}
	if envHost != "" {
		return envHost
	}
	return defaultHost
}

func main() {
	args := os.Args[1:]
	if wantsVersion(args) {
		printVersion(os.Stdout)
		return
	}

	flags, err := parseFlags(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		os.Exit(2)
	}

	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using default local host")
	}

	host := resolveHost(flags.Host, os.Getenv("OLLAMA_HOST"))

	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working directory: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("could not determine home directory: %v", err)
	}

	sysContent, err := prompt.LoadOrCreateSystemPrompt(home)
	if err != nil {
		log.Fatalf("failed to load system prompt: %v", err)
	}
	a := &agent.Agent{Host: host, Root: workingDir}
	var verifyCommand []string
	if flags.Verify != "" {
		verifyCommand, err = parseVerifyCommand(flags.Verify)
		if err != nil {
			log.Fatalf("invalid --verify value: %v", err)
		}
		a.Verify = verify.Runner{Command: verifyCommand}.Run
	}

	cli := ui.New(a, sysContent)
	cli.VerifyCommand = verifyCommand
	cli.Run()

}

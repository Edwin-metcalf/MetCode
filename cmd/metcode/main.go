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

const systemPromptPath = "~/.metcode/system_prompt.md"

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
	return parseFlagsWithOutput(args, os.Stderr)
}

func parseFlagsWithOutput(args []string, output io.Writer) (cliFlags, error) {
	fs := flag.NewFlagSet("metcode", flag.ContinueOnError)
	fs.SetOutput(output)
	host := fs.String("host", "", "ollama URL (default: OLLAMA_HOST, then "+defaultHost+")")
	verifyCmd := fs.String("verify", "", "command to run after an edit, e.g. \"go test ./...\"")
	help := fs.Bool("help", false, "print usage and exit")
	shortHelp := fs.Bool("h", false, "print usage and exit")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: metcode [options]")
		fmt.Fprintln(fs.Output(), "\nStart the local coding assistant using an Ollama server.")
		fmt.Fprintln(fs.Output(), "\nOptions:")
		fs.PrintDefaults()
		fmt.Fprintln(fs.Output(), "\nOther options:")
		fmt.Fprintln(fs.Output(), "  --help, -h")
		fmt.Fprintln(fs.Output(), "      print this help and exit")
		fmt.Fprintln(fs.Output(), "  --version, -v")
		fmt.Fprintln(fs.Output(), "      print the MetCode version and exit")
		fmt.Fprintln(fs.Output(), "\nEnvironment:")
		fmt.Fprintln(fs.Output(), "  OLLAMA_HOST")
		fmt.Fprintln(fs.Output(), "      Ollama server URL; used when --host is not supplied (default: "+defaultHost+")")
		fmt.Fprintln(fs.Output(), "\nConfiguration:")
		fmt.Fprintln(fs.Output(), "  System prompt: "+systemPromptPath)
		fmt.Fprintln(fs.Output(), "      created with a default prompt on first run; edit it to customize MetCode")
		fmt.Fprintln(fs.Output(), "\nInteractive commands:")
		fmt.Fprintln(fs.Output(), "  /help          show available commands")
		fmt.Fprintln(fs.Output(), "  /save          save the current conversation")
		fmt.Fprintln(fs.Output(), "  /load          load a saved conversation")
		fmt.Fprintln(fs.Output(), "  /clear         clear the current conversation and plan")
		fmt.Fprintln(fs.Output(), "  /compact       summarize the conversation")
		fmt.Fprintln(fs.Output(), "  /change-model  switch the Ollama model")
	}
	if err := fs.Parse(args); err != nil {
		return cliFlags{}, err
	}
	if *help || *shortHelp {
		fs.Usage()
		return cliFlags{}, flag.ErrHelp
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

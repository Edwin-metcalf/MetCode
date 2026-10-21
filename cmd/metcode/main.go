package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"
	"slices"

	"github.com/Edwin-metcalf/MetCode/internal/agent"
	"github.com/Edwin-metcalf/MetCode/internal/prompt"
	"github.com/Edwin-metcalf/MetCode/internal/ui"
	"github.com/joho/godotenv"
)

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

func main() {
	if wantsVersion(os.Args[1:]) {
		printVersion(os.Stdout)
		return
	}

	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using default local host")
	}

	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://localhost:11434"
	}

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
	cli := ui.New(
		&agent.Agent{Host: host, Root: workingDir}, sysContent,
	)

	cli.Run()

}

package main

import (
	"log"
	"os"

	"github.com/Edwin-metcalf/MetCode/internal/agent"
	"github.com/Edwin-metcalf/MetCode/internal/prompt"
	"github.com/Edwin-metcalf/MetCode/internal/ui"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using default local host")
	}

	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://localhost:11434"
	}

	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working directory")
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

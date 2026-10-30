// main package for the bench marking
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Edwin-metcalf/MetCode/internal/bench"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}

	switch args[0] {
	case "validate":
		return runValidate(args[1:])

	case "run":
		return runRun(args[1:])
	case "report":
		fmt.Fprintln(os.Stderr, "report: not implemented yet")
		return 2
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", args[0])
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: metbench <validate|run|report> [flags]")
}

func runValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)

	taskDir := fs.String("tasks", "bench/tasks", "directory containing task folders")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	tasks, err := bench.LoadTasks(*taskDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading tasks: %v\n", err)
		return 1
	}

	failed, skipped := 0, 0
	for _, t := range tasks {
		if t.NoTools {
			fmt.Printf("skip %s (no_tools)\n", t.Name)
			skipped += 1
			continue
		}

		if err := bench.ValidateTask(t); err != nil {
			fmt.Printf("FAIL  %v\n", err)
			failed++
			continue
		}
		fmt.Printf("ok		%s\n", t.Name)
	}

	fmt.Printf("%d tasks: %d ok, %d failed, %d skipped\n", len(tasks), len(tasks)-failed-skipped, failed, skipped)
	if failed > 0 {
		return 1
	}
	return 0
}

func runRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)

	taskDir := fs.String("tasks", "bench/tasks", "directory containing task folders")
	taskName := fs.String("task", "", "name of the task to run (required)")
	model := fs.String("model", "", "ollama model name (required)")
	host := fs.String("host", "", "ollama URL, e.g. http://<tailscale-name>:11434 (default: OLLAMA_HOST, then localhost)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *taskName == "" {
		fmt.Fprint(os.Stderr, "-task is required\n")
		return 2
	}
	if *model == "" {
		fmt.Fprint(os.Stderr, "-model is requried\n")
		return 2
	}

	tasks, err := bench.LoadTasks(*taskDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Loading tasks: %v\n", err)
	}

	var task bench.Task
	found := false
	var names []string
	for _, t := range tasks {
		names = append(names, t.Name)

		if t.Name == *taskName {
			found = true
			task = t
		}
	}

	if !found {
		fmt.Fprintf(os.Stderr, "no task %q; available: %s\n", *taskName, strings.Join(names, ", "))
		return 2
	}

	if err := bench.ValidateTask(task); err != nil {
		fmt.Fprintf(os.Stderr, "validating Task: %v\n", err)
		return 1
	}

	cfg := bench.Config{Model: *model, Host: *host, RunIndex: 0, GitCommit: gitCommit()}

	res, err := bench.Run(task, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "running task: %v\n", err)
		return 1
	}

	b, err := json.Marshal(res)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encoding result: %v\n", err)
		return 1
	}

	fmt.Println(string(b))
	return 0
}

// gitCommit returns HEAD with "-dirty" appended if tracked files have uncommited changes.
func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}

	outString := strings.TrimSpace(string(out))

	dirtyOut, err := exec.Command("git", "status", "--porcelain", "--untracked-files=no").Output()
	if err != nil {
		//tree might not be clean
		fmt.Fprintln(os.Stderr, "warning: could not check git status")
		return outString + "-unchecked"
	}

	if len(strings.TrimSpace(string(dirtyOut))) > 0 {
		outString += "-dirty"
	}

	return outString
}

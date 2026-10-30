// main package for the bench marking
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	taskName := fs.String("task", "", "name of the task to run (default: all tasks)")
	models := fs.String("models", "", "comma-separated ollama model names (required)")
	n := fs.Int("n", 1, "repeats per model and task")
	out := fs.String("out", "", "results file (default: bench/results/<time>-<commit>.jsonl)")
	host := fs.String("host", "", "ollama URL, e.g. http://<tailscale-name>:11434 (default: OLLAMA_HOST, then localhost)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *models == "" {
		fmt.Fprint(os.Stderr, "-models are requried (comma-separated)\n")
		return 2
	}

	var modelList []string
	for _, m := range strings.Split(*models, ",") {
		m = strings.TrimSpace(m)
		if m == "" {
			fmt.Fprintln(os.Stderr, "-models: empty model name (stray comma?)")
			return 2
		}
		modelList = append(modelList, m)
	}
	if *n < 1 {
		fmt.Fprintln(os.Stderr, "-n must be at least 1\n")
	}

	tasks, err := bench.LoadTasks(*taskDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Loading tasks: %v\n", err)
	}

	if *taskName != "" {
		var names []string
		var picked []bench.Task

		for _, t := range tasks {
			names = append(names, t.Name)
			if t.Name == *taskName {
				picked = append(picked, t)
			}
		}

		if len(picked) == 0 {
			fmt.Fprintf(os.Stderr, "no task %q; available: %s\n", *taskName, strings.Join(names, ", "))
			return 2
		}
		tasks = picked
	}

	if problems := bench.ValidateAll(tasks); len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "FAIL %v\n", p)
		}
		fmt.Fprintln(os.Stderr, "refusing to run: fix the fixtures above (see `metbench validate`)")
		return 1
	}

	commit := gitCommit()
	path := *out
	if path == "" {
		short := commit
		if len(short) > 8 {
			short = short[:8]
		}
		path = filepath.Join("bench", "results", time.Now().Format("20060102-150405")+"-"+short+".jsonl")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "creating results dir: %v\n", err)
		return 1
	}

	//use O_EXCL: never overwrite an existing results fiel
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating %s: %v\n", path, err)
		return 1
	}
	defer f.Close()
	fmt.Fprintf(os.Stderr, "writing results to %s\n", path)

	mc := bench.MatrixConfig{Models: modelList, N: *n, Host: *host, GitCommit: commit}
	progress := func(done, total int, r bench.Result) {
		fmt.Fprintf(os.Stderr, "[%d/%d] %s %s #%d ended=%s passed=%v\n",
			done, total, r.Model, r.Task, r.RunIndex, r.Ended, r.Passed)
	}

	if err := bench.RunMatrix(tasks, mc, f, progress); err != nil {
		fmt.Fprintf(os.Stderr, "run stopped: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "done: %s\n", path)
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

// main package for the bench marking
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/Edwin-metcalf/MetCode/internal/bench"
)

func main() {
	os.Exit(run(os.Args[1:]))
	//will need to import set up validate it then run it
	//then grade the output
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
		fmt.Fprintln(os.Stderr, "run: not implemented yet")
		return 2
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

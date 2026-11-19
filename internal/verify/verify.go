// Package verify runs a command and reports whether it passed.
package verify

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

const (
	DefaultTimeout   = 90 * time.Second
	DefaultMaxOutput = 8000
)

type Result struct {
	Passed   bool
	Output   string
	Duration time.Duration
	Err      error
}

type Runner struct {
	Command   []string
	Timeout   time.Duration
	MaxOutput int
}

func (r Runner) Run(workdir string) Result {
	start := time.Now()

	if len(r.Command) == 0 {
		return Result{Err: errors.New("verify: no command"), Duration: time.Since(start)}
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	maxOutput := r.MaxOutput
	if maxOutput <= 0 {
		maxOutput = DefaultMaxOutput
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.Command[0], r.Command[1:]...)
	cmd.Dir = workdir
	out, err := cmd.CombinedOutput()

	res := Result{Output: clip(string(out), maxOutput), Duration: time.Since(start)}
	// Timeouts and non-zero exits are verdicts. Only a start failure is an Err.
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		res.Passed = true
	case ctx.Err() == context.DeadlineExceeded, errors.As(err, &exitErr):
	default:
		res.Err = err
	}
	return res
}

func clip(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

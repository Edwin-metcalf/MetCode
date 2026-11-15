package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// sampleJSONL is a tracked results file with real rows, used as input. It is
// data, not a golden: the tests below assert behavior, not exact table bytes.
const sampleJSONL = "../../bench/results/published/baseline/baseline.jsonl"

// capture redirects os.Stdout and os.Stderr around fn and returns what each got.
// It swaps the real streams, so tests using it must not call t.Parallel().
func capture(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()

	fn()

	outW.Close()
	errW.Close()

	var outBuf, errBuf bytes.Buffer
	if _, err := io.Copy(&outBuf, outR); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(&errBuf, errR); err != nil {
		t.Fatal(err)
	}
	outR.Close()
	errR.Close()

	return outBuf.String(), errBuf.String()
}

func TestReportSubcommand_NoArgsIsUsageError(t *testing.T) {
	var code int
	_, stderr := capture(t, func() { code = run([]string{"report"}) })

	if code != 2 {
		t.Errorf("report with no files exited %d, want 2", code)
	}
	if !strings.Contains(stderr, "usage:") {
		t.Errorf("stderr should say usage, got %q", stderr)
	}
}

func TestReportSubcommand_MissingFileFails(t *testing.T) {
	const missing = "bench/results/does-not-exist.jsonl"

	var code int
	_, stderr := capture(t, func() { code = run([]string{"report", missing}) })

	if code != 1 {
		t.Errorf("report on a missing file exited %d, want 1", code)
	}
	if !strings.Contains(stderr, missing) {
		t.Errorf("stderr should name the missing file, got %q", stderr)
	}
}

// report takes several files and groups them together, so the same rows twice
// double N per group instead of printing two tables.
func TestReportSubcommand_GroupsMultipleFiles(t *testing.T) {
	var code int
	stdout, _ := capture(t, func() { code = run([]string{"report", sampleJSONL, sampleJSONL}) })

	if code != 0 {
		t.Fatalf("report exited %d, want 0", code)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 16 { // header + separator + 14 groups, same as the single run
		t.Fatalf("got %d table lines, want 16", len(lines))
	}
	for _, line := range lines[2:] {
		if !strings.Contains(line, " | 20 | ") {
			t.Errorf("group should have N=20 when the file is passed twice, got: %q", line)
		}
	}
}

package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

const (
	publishedJSONL = "../../bench/results/published/baseline/baseline.jsonl"
	publishedTable = "../../bench/results/published/baseline/table.md"
)

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

// firstDiffLine returns the 1-based number of the first line that differs
// between got and want, with both lines. 0 means they are identical.
func firstDiffLine(got, want string) (int, string, string) {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			return i + 1, gl, wl
		}
	}
	return 0, "", ""
}

// The issue's done-when, as a test: the subcommand, not just the formatter.
func TestReportSubcommand_ReproducesPublishedTable(t *testing.T) {
	want, err := os.ReadFile(publishedTable)
	if err != nil {
		t.Fatalf("reading the golden table: %v", err)
	}

	var code int
	stdout, stderr := capture(t, func() { code = run([]string{"report", publishedJSONL}) })

	if code != 0 {
		t.Fatalf("report exited %d, want 0 (stderr: %s)", code, stderr)
	}
	if stderr != "" {
		t.Errorf("a clean published run should warn about nothing, got: %s", stderr)
	}
	if line, gotLine, wantLine := firstDiffLine(stdout, string(want)); line != 0 {
		t.Fatalf("output differs from %s at line %d:\n got %q\nwant %q", publishedTable, line, gotLine, wantLine)
	}
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
	stdout, _ := capture(t, func() { code = run([]string{"report", publishedJSONL, publishedJSONL}) })

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

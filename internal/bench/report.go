package bench

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

type Group struct {
	Model string
	Task  string
	Rows  []Result
}

type Stats struct {
	N, Passed, Tampered, EditApplied, ClaimedNoEdit int
	Ended                                           map[string]int
	MeanCalls, MeanUnnecessary, MeanPromptTokens    float64
	ToolErrors                                      int
	ToolErrorsByTool                                map[string]int
	EditFails                                       map[string]int
	WallMedianMS, WallMinMS, WallMaxMS              int64
	NoTools                                         bool
}

// loadResults reads JSONL rows. An unreadable row is an error naming its position
func LoadResults(r io.Reader) ([]Result, error) {
	var results []Result
	decoder := json.NewDecoder(r)
	for {
		var result Result
		if err := decoder.Decode(&result); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("row %d: %w", len(results)+1, err)
		}

		results = append(results, result)
	}
	if len(results) == 0 {
		return nil, errors.New("no results found")
	}

	return results, nil
}

// GroupResults groups by (model, task), sorted by model then task
// Rows keep their input order inside a group.
func GroupResults(rows []Result) []Group {
	type key struct{ model, task string }
	index := map[key]int{}
	var groups []Group
	for _, row := range rows {
		k := key{row.Model, row.Task}
		i, ok := index[k]
		if !ok {
			i = len(groups)
			index[k] = i
			groups = append(groups, Group{Model: row.Model, Task: row.Task})
		}
		groups[i].Rows = append(groups[i].Rows, row)
	}
	slices.SortStableFunc(groups, func(a, b Group) int {
		if c := strings.Compare(a.Model, b.Model); c != 0 {
			return c
		}
		return strings.Compare(a.Task, b.Task)
	})
	return groups
}

func sumMap(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

// median works on a copy, so callers keep their order. An empty slice gives 0
func median(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}

	s := slices.Clone(xs)
	slices.Sort(s)

	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

func (g Group) Stats() Stats {
	s := Stats{
		N:                len(g.Rows),
		Ended:            map[string]int{},
		ToolErrorsByTool: map[string]int{},
		EditFails:        map[string]int{},
	}
	if s.N == 0 {
		return s
	}
	s.NoTools = g.Rows[0].NoTools

	var calls, unnecessary, promptTok int
	walls := make([]int64, 0, s.N)
	for _, r := range g.Rows {
		if r.Passed {
			s.Passed++
		}
		if r.Tampered {
			s.Tampered++
		}
		if r.EditApplied {
			s.EditApplied++
		}
		// said it was done, but nothing changed (only meaningful where an edit was expected)
		if !r.NoTools && r.Ended == "ok" && !r.EditApplied {
			s.ClaimedNoEdit++
		}
		s.Ended[r.Ended]++
		calls += sumMap(r.ToolCalls)
		s.ToolErrors += sumMap(r.ToolErrors)
		for tool, n := range r.ToolErrors {
			s.ToolErrorsByTool[tool] += n
		}
		for reason, n := range r.EditFails() {
			s.EditFails[reason] += n
		}
		unnecessary += r.UnnecessaryCalls
		promptTok += r.PromptTokens
		walls = append(walls, r.WallMS-r.LoadMS)
	}

	n := float64(s.N)
	s.MeanCalls = float64(calls) / n
	s.MeanUnnecessary = float64(unnecessary) / n
	s.MeanPromptTokens = float64(promptTok) / n

	sorted := slices.Clone(walls)
	slices.Sort(sorted)
	s.WallMinMS, s.WallMaxMS = sorted[0], sorted[len(sorted)-1]
	s.WallMedianMS = median(walls)
	return s
}

// Warnings returns one line per field that differs inside the group, plus one
// if any commit is not reproducible. Averaging across those is usually a mistake.
func (g Group) Warnings() []string {
	var out []string

	mixed := func(field string, get func(Result) string) {
		seen := map[string]bool{}
		var vals []string
		for _, r := range g.Rows {
			if v := get(r); !seen[v] {
				seen[v] = true
				vals = append(vals, v)
			}
		}
		if len(vals) > 1 {
			slices.Sort(vals)
			out = append(out, fmt.Sprintf("%s / %s: mixed %s (%s)", g.Model, g.Task, field, strings.Join(vals, ", ")))
		}
	}
	mixed("git_commit", func(r Result) string { return r.GitCommit })
	mixed("prompt_hash", func(r Result) string { return r.PromptHash })
	mixed("num_ctx", func(r Result) string { return strconv.Itoa(r.NumCtx) })
	mixed("temperature", func(r Result) string { return strconv.FormatFloat(r.Temperature, 'g', -1, 64) })

	bad := 0
	for _, r := range g.Rows {
		c := r.GitCommit
		if strings.HasSuffix(c, "-dirty") || strings.HasSuffix(c, "-unchecked") || c == "unknown" {
			bad++
		}
	}
	if bad > 0 {
		out = append(out, fmt.Sprintf("%s / %s: %d row(s) from a dirty, unchecked or unknown commit: not reproducible", g.Model, g.Task, bad))
	}
	return out
}

func secs(ms int64) string { return fmt.Sprintf("%.1f", float64(ms)/1000) }

// formatToolErrors is "total (tool count, ...)" listing only the tools that
// errored, biggest first, ties by name so two reports of the same rows match.
// Names are not filtered against the schema: a model calling a tool that does
// not exist shows up here under whatever it invented.
func formatToolErrors(byTool map[string]int) string {
	total := sumMap(byTool)
	if total == 0 {
		return "0"
	}

	names := make([]string, 0, len(byTool))
	for name, n := range byTool {
		if n > 0 {
			names = append(names, name)
		}
	}
	slices.SortFunc(names, func(a, b string) int {
		if byTool[a] != byTool[b] {
			if byTool[a] > byTool[b] {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s %d", name, byTool[name]))
	}
	return fmt.Sprintf("%d (%s)", total, strings.Join(parts, ", "))
}

// formatEditFails is mal/anc/oth, the same shape as ok/max/fail/err.
func formatEditFails(fails map[string]int) string {
	return fmt.Sprintf("%d/%d/%d", fails[EditFailMalformed], fails[EditFailAnchor], fails[EditFailOther])
}

func FormatMarkdown(groups []Group) string {
	var b strings.Builder
	b.WriteString("| Model | Task | N | Pass | ok/max/fail/err | timeout/unreach | No-edit claims | Tampered | Calls/run | Tool errors | Edit fails mal/anc/oth | Unnecessary/run | Wall s (median, range) | Prompt tok/run |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, g := range groups {
		s := g.Stats()
		pass := fmt.Sprintf("%d/%d", s.Passed, s.N)
		noEdit := strconv.Itoa(s.ClaimedNoEdit)
		unnecessary := "-"
		if s.NoTools { // never graded, and "no edit" is the right behavior
			pass, noEdit = "n/a", "n/a"
			unnecessary = fmt.Sprintf("%.1f", s.MeanUnnecessary)
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %d/%d/%d/%d | %d/%d | %s | %d | %.1f | %s | %s | %s | %s (%s-%s) | %.0f |\n",
			g.Model, g.Task, s.N, pass,
			s.Ended["ok"], s.Ended["max_rounds"], s.Ended["too_many_failures"], s.Ended["model_error"],
			s.Ended["timeout"], s.Ended["server_unreachable"],
			noEdit, s.Tampered, s.MeanCalls, formatToolErrors(s.ToolErrorsByTool), formatEditFails(s.EditFails),
			unnecessary, secs(s.WallMedianMS), secs(s.WallMinMS), secs(s.WallMaxMS), s.MeanPromptTokens)
	}
	return b.String()
}

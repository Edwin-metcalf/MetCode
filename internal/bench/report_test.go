package bench

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func mkResult(model, task string, edits ...func(*Result)) Result {
	r := Result{
		Model: model, Task: task, Ended: "ok",
		NumCtx: 8192, Temperature: 0.2, PromptHash: "p1", GitCommit: "c1",
	}
	for _, edit := range edits {
		edit(&r)
	}
	return r
}

func withRun(i int) func(*Result) { return func(r *Result) { r.RunIndex = i } }

func toJSONL(t *testing.T, rows ...Result) string {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			t.Fatal(err)
		}
	}
	return buf.String()
}

// parseTable turns a markdown table into one header->cell map per data row.
func parseTable(t *testing.T, md string) []map[string]string {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(md), "\n")
	if len(lines) < 2 {
		t.Fatalf("table has no header and separator: %q", md)
	}
	split := func(line string) []string {
		parts := strings.Split(strings.Trim(line, "|"), "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}
	header := split(lines[0])
	var rows []map[string]string
	for _, line := range lines[2:] {
		cells := split(line)
		if len(cells) != len(header) {
			t.Fatalf("row has %d cells, header has %d: %q", len(cells), len(header), line)
		}
		row := map[string]string{}
		for i, h := range header {
			row[h] = cells[i]
		}
		rows = append(rows, row)
	}
	return rows
}

// ---------- LoadResults ----------

func TestLoadResults_RoundTripsRows(t *testing.T) {
	want := []Result{
		mkResult("m1", "a", withRun(0), func(r *Result) {
			r.Passed = true
			r.ToolCalls = map[string]int{"read_file": 2}
		}),
		mkResult("m1", "a", withRun(1), func(r *Result) { r.Ended = "max_rounds" }),
	}

	got, err := LoadResults(strings.NewReader(toJSONL(t, want...)))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows changed in transit:\n got  %+v\n want %+v", got, want)
	}
}

// bufio.Scanner would fail on a line over 64KB; transcripts can be that big.
func TestLoadResults_HugeRowLoads(t *testing.T) {
	big := mkResult("m", "t", func(r *Result) { r.FinalReply = strings.Repeat("x", 200_000) })

	got, err := LoadResults(strings.NewReader(toJSONL(t, big, mkResult("m", "t"))))
	if err != nil {
		t.Fatalf("a large row must load: %v", err)
	}
	if len(got) != 2 || len(got[0].FinalReply) != 200_000 {
		t.Errorf("got %d rows, first reply length %d", len(got), len(got[0].FinalReply))
	}
}

func TestLoadResults_BadRowNamesItsPosition(t *testing.T) {
	good := toJSONL(t, mkResult("m", "t"))
	cases := map[string]struct{ input, want string }{
		"garbage first row":  {"not json\n" + good, "row 1"},
		"garbage second row": {good + "not json\n", "row 2"},
		"truncated last row": {good + `{"model": "m", "task"`, "row 2"},
		"wrong field type":   {`{"passed": "yes"}` + "\n", "row 1"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rows, err := LoadResults(strings.NewReader(tc.input))
			if err == nil {
				t.Fatalf("want an error, got %d rows", len(rows))
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error should say %q, got %q", tc.want, err)
			}
		})
	}
}

func TestLoadResults_EmptyInputIsAnError(t *testing.T) {
	for name, input := range map[string]string{"empty": "", "blank lines": "\n\n  \n"} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadResults(strings.NewReader(input)); err == nil {
				t.Error("no rows must be an error, so a wrong path does not print an empty table")
			}
		})
	}
}

// ---------- GroupResults ----------

func TestGroupResults_GroupsAndSorts(t *testing.T) {
	rows := []Result{
		mkResult("m2", "b", withRun(0)),
		mkResult("m1", "b", withRun(0)),
		mkResult("m2", "a", withRun(0)),
		mkResult("m1", "a", withRun(0)),
		mkResult("m1", "b", withRun(1)),
	}

	groups := GroupResults(rows)

	type want struct {
		model, task string
		runs        []int
	}
	wants := []want{{"m1", "a", []int{0}}, {"m1", "b", []int{0, 1}}, {"m2", "a", []int{0}}, {"m2", "b", []int{0}}}
	if len(groups) != len(wants) {
		t.Fatalf("got %d groups, want %d", len(groups), len(wants))
	}
	for i, w := range wants {
		g := groups[i]
		if g.Model != w.model || g.Task != w.task {
			t.Errorf("group %d = %s/%s, want %s/%s", i, g.Model, g.Task, w.model, w.task)
		}
		var runs []int
		for _, r := range g.Rows {
			runs = append(runs, r.RunIndex)
		}
		if !slices.Equal(runs, w.runs) {
			t.Errorf("group %s/%s rows in order %v, want %v", g.Model, g.Task, runs, w.runs)
		}
	}
}

func TestGroupResults_KeyIsNotAmbiguous(t *testing.T) {
	// "a"+"bc" and "ab"+"c" are the same string, but not the same group
	groups := GroupResults([]Result{mkResult("a", "bc"), mkResult("ab", "c")})
	if len(groups) != 2 {
		t.Errorf("got %d groups, want 2", len(groups))
	}
}

// ---------- Stats ----------

func TestStats_Counts(t *testing.T) {
	g := Group{Model: "m", Task: "t", Rows: []Result{
		mkResult("m", "t", func(r *Result) {
			r.Passed, r.EditApplied = true, true
			r.ToolCalls = map[string]int{"read_file": 2, "edit_file": 1}
			r.PromptTokens, r.WallMS, r.LoadMS = 1000, 5000, 1000
		}),
		mkResult("m", "t", func(r *Result) { // says it is done, changed nothing
			r.ToolCalls = map[string]int{"read_file": 2}
			r.PromptTokens, r.WallMS = 2000, 3000
		}),
		mkResult("m", "t", func(r *Result) {
			r.Ended = "max_rounds"
			r.ToolCalls = map[string]int{"read_file": 12}
			r.ToolErrors = map[string]int{"read_file": 3}
			r.PromptTokens, r.WallMS = 3000, 7000
		}),
		mkResult("m", "t", func(r *Result) {
			r.Tampered, r.EditApplied = true, true
			r.WallMS = 1000
		}),
	}}

	s := g.Stats()

	if s.N != 4 || s.Passed != 1 || s.Tampered != 1 || s.EditApplied != 2 {
		t.Errorf("counts wrong: %+v", s)
	}
	if s.ClaimedNoEdit != 1 {
		t.Errorf("ClaimedNoEdit = %d, want 1 (max_rounds is not a claim of being done)", s.ClaimedNoEdit)
	}
	if s.Ended["ok"] != 3 || s.Ended["max_rounds"] != 1 {
		t.Errorf("Ended = %v", s.Ended)
	}
	if s.ToolErrors != 3 {
		t.Errorf("ToolErrors = %d, want 3", s.ToolErrors)
	}
	if s.MeanCalls != 4.25 { // (3+2+12+0)/4
		t.Errorf("MeanCalls = %v, want 4.25", s.MeanCalls)
	}
	if s.MeanPromptTokens != 1500 {
		t.Errorf("MeanPromptTokens = %v, want 1500", s.MeanPromptTokens)
	}
	// wall minus load: 4000, 3000, 7000, 1000
	if s.WallMedianMS != 3500 || s.WallMinMS != 1000 || s.WallMaxMS != 7000 {
		t.Errorf("wall = median %d min %d max %d, want 3500/1000/7000", s.WallMedianMS, s.WallMinMS, s.WallMaxMS)
	}
	if s.NoTools {
		t.Error("NoTools should be false")
	}
}

func TestStats_NoToolsGroup(t *testing.T) {
	noTools := func(unnecessary int) func(*Result) {
		return func(r *Result) { r.NoTools, r.UnnecessaryCalls = true, unnecessary }
	}
	g := Group{Model: "m", Task: "greeting", Rows: []Result{
		mkResult("m", "greeting", noTools(2)),
		mkResult("m", "greeting", noTools(0)),
		mkResult("m", "greeting", noTools(1)),
	}}

	s := g.Stats()

	if !s.NoTools {
		t.Error("NoTools should be true")
	}
	if s.ClaimedNoEdit != 0 {
		t.Errorf("ClaimedNoEdit = %d: not editing is correct on a no_tools task", s.ClaimedNoEdit)
	}
	if s.MeanUnnecessary != 1 {
		t.Errorf("MeanUnnecessary = %v, want 1", s.MeanUnnecessary)
	}
}

func TestStats_WallExcludesLoad(t *testing.T) {
	g := Group{Rows: []Result{mkResult("m", "t", func(r *Result) { r.WallMS, r.LoadMS = 9656, 4421 })}}
	s := g.Stats()
	if s.WallMedianMS != 5235 || s.WallMinMS != 5235 || s.WallMaxMS != 5235 {
		t.Errorf("wall = %d/%d/%d, want 5235 for all", s.WallMedianMS, s.WallMinMS, s.WallMaxMS)
	}
}

func TestStats_EmptyGroupDoesNotPanic(t *testing.T) {
	s := Group{}.Stats()
	if s.N != 0 || s.Passed != 0 {
		t.Errorf("empty group: %+v", s)
	}
}

func TestMedian(t *testing.T) {
	cases := []struct {
		in   []int64
		want int64
	}{
		{nil, 0},
		{[]int64{5}, 5},
		{[]int64{3, 1, 2}, 2},
		{[]int64{4, 1, 3, 2}, 2}, // (2+3)/2, integer division
		{[]int64{10, 20}, 15},
	}
	for _, tc := range cases {
		if got := median(tc.in); got != tc.want {
			t.Errorf("median(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}

	in := []int64{3, 1, 2}
	median(in)
	if !slices.Equal(in, []int64{3, 1, 2}) {
		t.Errorf("median reordered its input: %v", in)
	}
}

// ---------- Warnings ----------

func TestWarnings(t *testing.T) {
	cases := []struct {
		name string
		edit func(rows []Result)
		want []string // one substring per expected warning, in order
	}{
		{"consistent group", func(rs []Result) {}, nil},
		{"mixed commit", func(rs []Result) { rs[1].GitCommit = "c2" }, []string{"git_commit"}},
		{"mixed prompt hash", func(rs []Result) { rs[1].PromptHash = "p2" }, []string{"prompt_hash"}},
		{"mixed num_ctx", func(rs []Result) { rs[1].NumCtx = 4096 }, []string{"num_ctx"}},
		{"mixed temperature", func(rs []Result) { rs[1].Temperature = 0.7 }, []string{"temperature"}},
		{"two fields mixed", func(rs []Result) { rs[1].PromptHash = "p2"; rs[1].NumCtx = 4096 }, []string{"prompt_hash", "num_ctx"}},
		{"dirty everywhere", func(rs []Result) {
			rs[0].GitCommit, rs[1].GitCommit = "c1-dirty", "c1-dirty"
		}, []string{"not reproducible"}},
		{"unchecked tree", func(rs []Result) {
			rs[0].GitCommit, rs[1].GitCommit = "c1-unchecked", "c1-unchecked"
		}, []string{"not reproducible"}},
		{"unknown commit", func(rs []Result) {
			rs[0].GitCommit, rs[1].GitCommit = "unknown", "unknown"
		}, []string{"not reproducible"}},
		{"mixed and dirty", func(rs []Result) { rs[1].GitCommit = "c1-dirty" }, []string{"git_commit", "not reproducible"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := []Result{mkResult("m", "t"), mkResult("m", "t")}
			tc.edit(rows)

			got := Group{Model: "m", Task: "t", Rows: rows}.Warnings()

			if len(got) != len(tc.want) {
				t.Fatalf("got %d warnings %q, want %d", len(got), got, len(tc.want))
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("warning %d = %q, want it to mention %q", i, got[i], w)
				}
			}
		})
	}
}

// ---------- FormatMarkdown ----------

func TestFormatMarkdown_Cells(t *testing.T) {
	g := Group{Model: "m", Task: "t", Rows: []Result{
		mkResult("m", "t", func(r *Result) {
			r.Passed, r.EditApplied = true, true
			r.ToolCalls = map[string]int{"read_file": 1, "edit_file": 1}
			r.PromptTokens, r.WallMS, r.LoadMS = 1000, 4000, 1000
		}),
		mkResult("m", "t", func(r *Result) {
			r.ToolCalls = map[string]int{"read_file": 2}
			r.PromptTokens, r.WallMS = 2000, 2000
		}),
	}}

	rows := parseTable(t, FormatMarkdown([]Group{g}))

	if len(rows) != 1 {
		t.Fatalf("want 1 data row, got %d", len(rows))
	}
	want := map[string]string{
		"Model": "m", "Task": "t", "N": "2", "Pass": "1/2",
		"ok/max/fail/err": "2/0/0/0", "timeout/unreach": "0/0", "No-edit claims": "1", "Tampered": "0",
		"Calls/run": "2.0", "Tool errors": "0", "Unnecessary/run": "-",
		"Wall s (median, range)": "2.5 (2.0-3.0)", "Prompt tok/run": "1500",
	}
	for col, w := range want {
		if got := rows[0][col]; got != w {
			t.Errorf("column %q = %q, want %q", col, got, w)
		}
	}
}

func TestFormatMarkdown_NoToolsRowShowsNA(t *testing.T) {
	noTools := func(unnecessary int) func(*Result) {
		return func(r *Result) { r.NoTools, r.UnnecessaryCalls = true, unnecessary }
	}
	g := Group{Model: "m", Task: "greeting", Rows: []Result{
		mkResult("m", "greeting", noTools(2)),
		mkResult("m", "greeting", noTools(0)),
	}}

	rows := parseTable(t, FormatMarkdown([]Group{g}))

	if rows[0]["Pass"] != "n/a" || rows[0]["No-edit claims"] != "n/a" {
		t.Errorf("ungraded task should show n/a, got pass %q no-edit %q", rows[0]["Pass"], rows[0]["No-edit claims"])
	}
	if rows[0]["Unnecessary/run"] != "1.0" {
		t.Errorf("Unnecessary/run = %q, want 1.0", rows[0]["Unnecessary/run"])
	}
}

func TestFormatMarkdown_ShowsTransportFailures(t *testing.T) {
	g := Group{Model: "m", Task: "t", Rows: []Result{
		mkResult("m", "t", func(r *Result) { r.Ended = "timeout" }),
		mkResult("m", "t", func(r *Result) { r.Ended = "server_unreachable" }),
	}}

	rows := parseTable(t, FormatMarkdown([]Group{g}))

	if got := rows[0]["timeout/unreach"]; got != "1/1" {
		t.Errorf("timeout/unreach = %q, want 1/1", got)
	}
	if got := rows[0]["ok/max/fail/err"]; got != "0/0/0/0" {
		t.Errorf("ok/max/fail/err = %q, want 0/0/0/0", got)
	}
}

func TestFormatMarkdown_OneRowPerGroupInOrder(t *testing.T) {
	groups := GroupResults([]Result{
		mkResult("m2", "a"), mkResult("m1", "b"), mkResult("m1", "a"),
	})

	rows := parseTable(t, FormatMarkdown(groups))

	var got []string
	for _, r := range rows {
		got = append(got, r["Model"]+"/"+r["Task"])
	}
	if want := []string{"m1/a", "m1/b", "m2/a"}; !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

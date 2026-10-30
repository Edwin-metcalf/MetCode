# MetCode benchmark harness

## How to answer the small model question?
We found that to improve and iterate the ability of these small models as effective agentic assistants 
we needed a better way to test and evaluate. Therefore this harness was built.
The main goal here is be able to reliable experiment and test benchlines for MetCode.
## What this measures
This is a harness and gives the user an effective way to run many interactive tests with the agents.
It does this by running a model through a simulated task and grading the output.

At its simplest level the harness completes runs which a single run consits of:
- one task 
- one model
- one turn of the real agent loop
- grade by running the tests


## Quick start

### prerequisites
- [Go](https://go.dev/dl/) (see `go.mod` for version I used)
- [Ollama](https://ollama.com) running somewhere you can reach — either on the same machine, or on another machine on your network (I use tailscale and an old gaming PC)
- A model that supports tool calling. Recommended: `qwen2.5-coder:7b` or `llama3.1` both have been used with MetCode.
- The model already pulled on that server (`ollama pull qwen2.5-coder:7b`). The harness does not pull models for you
- `go` on the `PATH` of the machine running the harness. Grading shells out to `go test` inside the fixture
- `git` on the `PATH` if you want the commit recorded. Without it every row gets `"git_commit": "unknown"` and the report warns that the run is not reproducible

Run everything from the repo root: the defaults are relative paths (`bench/tasks`, `bench/results`).
  
### running the harness
**Run from the root of metcode** because the default is bench/tasks:
```bash
go run ./cmd/metbench validate

go run ./cmd/metbench run -models model1,model2 -n 5 -host http://<yourHost>:11434 -out bench/results/location/runs.jsonl

go run ./cmd/metbench report bench/results/location/runs.jsonl > bench/results/location/table.md
```
- Validate checks all the tasks in /bench/tasks
- Run you put your models in a comma seperated, add your host and then optionally route the output to a specific location
- report builds the mark down table, use the > to put it into a file instead of printing 

A few flags worth knowing:

| flag | default | what it does |
|---|---|---|
| `-models` | required | comma separated list, run in the order given |
| `-task` | all tasks | run just one task by folder name |
| `-n` | 1 | repeats per model and task |
| `-host` | `OLLAMA_HOST`, then `localhost:11434` | where the server is |
| `-out` | `bench/results/<timestamp>-<short commit>.jsonl` | results file. Never overwrites: it opens with `O_EXCL` |
| `-tasks` | `bench/tasks` | where to load tasks from |

`run` writes one JSON line per run to the file and progress to stderr, so `2>/dev/null` leaves you with just the data. It also re-validates every fixture before the first model call and refuses to start if one already passes. That costs a `go test` per task up front, which is worth knowing before a long matrix.

`report` takes one or more JSONL files and groups them together, so you can pass a whole study or compare two runs in one table. Warnings (mixed commit, prompt hash, num_ctx, temperature, dirty tree) go to stderr; the table goes to stdout.

## How a run works

1. **Fresh workspace.** `files/` is copied into a new `os.MkdirTemp` directory and the turn runs there. The fixture on disk is never touched, and the temp directory is deleted when the run ends. Two runs of the same task cannot see each other's edits.
2. **The real agent loop.** It calls `agent.Turn` with the real system prompt, the real tool schema, `num_ctx=8192` and `temperature=0.2`. Same code path as `metcode`, so a prompt or tool change moves the benchmark. One turn only: the model gets the prompt, calls tools, and answers. It is never told "the tests still fail, try again".
3. **Tools inside the workspace.** Edits are auto-approved and applied to the temp copy. `run_command` is declined every time and counted in `declined_commands`, so the harness measures file-tool behavior only. Nothing prints while the run is going; the only stdout is the JSON line.
4. **Grading.** First the tamper check: every `files/*_test.go` is compared byte for byte against the workspace copy. If one is changed or missing the row is `tampered` and never counts as a pass, and the tests are not even run. Otherwise `test_cmd` runs in the workspace under `timeout_sec`.
5. **What gets recorded.** Pass/fail, how the turn ended, which tools were called and how many errored, whether the workspace changed at all, token counts, and four timings (wall, load, prefill, generation). The whole transcript is in the row too, which is what makes a bad run diagnosable later.

Why a separate binary: `metbench` auto-approves edits, and that must never exist in the shipped `metcode`. Keeping it in `cmd/metbench` means the approval bypass cannot be reached by accident from the chat client.

A `no_tools` task skips step 4 entirely, since there is no test to run. `Pass` is `n/a` and every tool call the model makes is counted as unnecessary. That is how the greeting task measures "does it stay quiet when nothing needs doing".

## Reading the table

One row per (model, task), sorted by model then task.

| column | what it is |
|---|---|
| Model, Task | the group. Every other column summarizes the rows in that group |
| N | runs in the group (models × tasks × `-n`) |
| Pass | graded passes out of N. `n/a` for a `no_tools` task. Timeout and unreachable rows are still counted in N, so a dead host reads `0/5` pass and `5/5` unreachable. Read those two columns together |
| ok/max/fail/err | how runs ended. `ok` finished normally, `max_rounds` hit the 12 round cap, `too_many_failures` had 4 failing tool rounds in a row, `model_error` means Ollama answered with an error (a 500, a bad model name, a malformed body) |
| timeout/unreach | `timeout` means the server accepted the request and did not finish within 2 minutes. `server_unreachable` means nothing answered at all: refused, no route, DNS. Both are harness problems, not model behavior, so drop those rows instead of counting them against the model |
| No-edit claims | rows that ended `ok` and left the workspace byte for byte unchanged. The model said it was done and did nothing. This is the most useful column for "it talks but does not act". `n/a` for `no_tools` |
| Tampered | rows where a fixture test file was edited or deleted. Never a pass |
| Calls/run | mean tool calls per run |
| Tool errors | total tool errors in the group. A sum, not a mean, so it grows with N |
| Unnecessary/run | for a `no_tools` task, mean calls made when none were needed. `-` for graded tasks |
| Wall s (median, range) | wall time of the turn with model load subtracted, so prefill + generation + network + tool time. Median and min-max rather than mean: at N=5 one cold start or a network blip moves a mean a lot. The raw `wall_ms` and `load_ms` are both in the JSONL |
| Prompt tok/run | mean prompt tokens per run, the cost side of the turn |

Two habits that keep the table honest:

- **k/N is not a rate at small N.** 5/5 and 4/5 differ by one run. Treat N=5 as a smoke test, and N>=20 as a weak signal. Do not compare two tables from different commits, prompts, or `num_ctx` and call it a regression.
- **Believe the warnings.** If `report` prints "mixed prompt_hash" or "not reproducible" on stderr, the group is averaging rows that were not the same experiment. Rerun instead of reading the table.

## Adding a task

```
bench/tasks/<name>/
  task.json
  files/              # copied into a throwaway workspace for every run
    go.mod
    thing.go
    thing_test.go
```

`task.json` keys:

| key | required | meaning |
|---|---|---|
| `prompt` | yes | the single user message. Say what is broken and what to fix |
| `test_cmd` | unless `no_tools` | argv, not a shell string. No pipes, no globs, no `&&`. Run with the workspace as cwd |
| `timeout_sec` | no, default 100 | the harness kill switch for `test_cmd` |
| `no_tools` | no | the task is not graded. Used by the greeting task, where the point is that no tool is needed |
| `name`, `dir` | no | filled in by the loader from the folder name and path. Do not set them |

`files/` has its own `go.mod` on purpose. It makes the fixture a standalone module, so `go build ./...` and `go test ./...` at the repo root skip it, and `go test ./...` inside the temp workspace resolves nothing outside the fixture. Keep fixtures dependency free: grading should never need the network.

`test_cmd` needs its own `-timeout` shorter than `timeout_sec` (the fixtures use `60s` under `90`). Without it `go test` waits 10 minutes by default, so a hung test is killed by the harness instead of failing as a test with a goroutine dump. Either way it is a fail, but the short timeout gets there sooner.

The prompt says "do not edit the test file" because the tamper check is byte for byte: a rewritten test is a `tampered` row, never a pass. Telling the model up front gives it a fair chance to avoid the trap, and the check catches it when it does not.

**A new fixture must fail before the model touches it.** A task that already passes grades a model that did nothing as a success. `metbench validate` enforces this: it copies the fixture, grades it untouched, and errors if it passes or is tampered. So write the bug first, run `validate`, then write the prompt.

## Results and studies

Scratch runs are disposable and git-ignored:

```
bench/results/20261029-210253-32329592.jsonl    # scratch, ignored
bench/results/studies/baseline-1/runs.jsonl     # a study (see the gitignore note below)
bench/results/studies/baseline-1/table.md
```

Publish a run as `bench/results/studies/<name>/` with three files: the `runs.jsonl`, the generated `table.md`, and a short `README.md` saying what changed (branch, hypothesis, models, host, date). The table alone does not say what you were testing, and the JSONL is the data the table was derived from, so keep both.

```bash
go run ./cmd/metbench report bench/results/studies/<name>/runs.jsonl > bench/results/studies/<name>/table.md
```

Heads up: `.gitignore` currently ignores `bench/results/*` and only un-ignores top level `baseline-*.jsonl` and `ablation-*.jsonl`. A study under `bench/results/studies/` is ignored along with everything else, so add a rule for it before you try to commit one, or `git status` will look clean when it is not. 

## Known limitations

- **Small N, one machine, no seed.** Temperature is fixed at 0.2 but there is no seed, so runs vary. Every number here is preliminary.
- **One turn per task.** The model never gets a second chance after a failing test, which is exactly where a lot of real agent value lives. Pass rates here are lower than what an iterating agent would get.
- **Tests are the whole spec.** A model can pass by gutting the function to satisfy the test cases rather than fixing the bug. That is why tasks are written with a few edge cases, but nothing checks intent.
- **Only top-level `*_test.go` is tamper checked.** A nested test file under `files/` is copied but not compared, so it could be edited without being flagged.
- **`run_command` is always declined.** Nothing here measures command use, and `declined_commands` is in the JSONL but not in the table.
- **A dead host does not stop the matrix.** `RunMatrix` aborts only when a row is `model_error` with zero prompt tokens. An unreachable host now records `server_unreachable`, so the matrix happily writes N junk rows and exits 0. Check the `timeout/unreach` column before trusting a run.
- **Wall time is Ollama's numbers plus the clock.** Load, prefill and generation come from the server's own durations; `wall_ms` includes network and tool execution on top. Subtracting them leaves network and overhead, not a pure model number.
- **The first row of a model pays the load.** It is subtracted from the reported wall time, but it still makes the first run slower in practice.
- **Tool calls are dropped past the first one per assistant message**, same as `metcode`, so a model that batches calls is measured on one of them.
- **Restraint and inability look the same.** A model that never emits a parseable tool call scores 0 unnecessary calls on the greeting task and 0 passes everywhere else. Read the two together.
- **Old rows are not comparable.** Rows written before `no_tools`, `load_ms`, `prefill_ms` and `gen_ms` existed load with zero values, so an old greeting row will look graded and an old run will look like it loaded instantly.

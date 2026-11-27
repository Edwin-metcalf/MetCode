# Contributing to MetCode

Thanks for taking a look! MetCode is an early-stage project and I'm learning as I go, so any help is great! Bug reports, ideas, docs fixes, and code are all welcome.
If you're unsure whether something is wanted, open an issue and ask first.

## Getting set up

You need [Go](https://go.dev/dl/) and, to actually chat with it, a reachable
[Ollama](https://ollama.com) server with a tool-calling model (llama3.1 has been tested).
See the README for the full setup and Ollama networking notes.

```bash
git clone https://github.com/Edwin-metcalf/MetCode
cd MetCode
go build -o metcode ./cmd/metcode
./metcode
```

## Before you send a change

Run this and make sure it's clean:

```bash
make check
```

`make check` runs the same steps as CI (`.github/workflows/ci.yml`), in the same order:
`go build ./...`, `go vet ./...`, a `gofmt -l .` check, `go test ./...`, and
`go run ./cmd/metbench validate`. If a file isn't formatted, `make fmt` fixes it.
Run `make help` to see every target (`make build`, `make test`, etc. run one step on its own).

No `make`? Run the steps directly:

```bash
go build ./...
go vet ./...
gofmt -l .    # should print nothing
go test ./...
go run ./cmd/metbench validate
```

Tests do **not** need a running Ollama server or network access — `internal/agent`,
`internal/ollama`, and `internal/ui` fake the model API with `httptest`.

There is no linter config; `make check` is the whole contract. Keep pull requests focused — one change per PR is easiest to review.

## Project layout

```
cmd/metcode/           entry point: wires everything together
cmd/metbench/          benchmark command line: validate, run, report
bench/                 task fixtures (bench/tasks) and results (bench/results)
internal/agent/        the model <-> tool loop for a single turn
internal/ollama/       Ollama API types and HTTP client (the only code that knows the wire format)
internal/bench/        benchmark engine: tasks, workspace, grading, runs, reports
internal/tools/        tool definitions and handlers, path safety
internal/plan/         plan step types (a bare data type, no logic)
internal/conversation/ saving and loading chat history
internal/prompt/       embedded default system prompt and loader
internal/ui/           banner, spinner, slash commands, input loop
internal/verify/       runs a command and reports whether it passed
```

`internal/ollama` is the only package that knows the Ollama wire format. `internal/tools`
depends on it; nothing depends on `ui`.

## Adding or changing a tool

You have to touch **three places** or the model will never see your change:

1. `internal/tools/definitions.go` — append to the package-level `tools.All` slice, which is
   sent on every request.
2. `internal/tools/tools.go` — add the tool name to the `Handle` switch.
3. The handler itself (`files.go`, `command.go`, or `planhelpers.go`), taking an
   `*ollama.ToolCall`.

**Hard contract:** every handler must return an `ollama.Message` with `Role: "tool"`, and every
failure must have a `Content` that starts with the literal string `error`. The runaway-loop
guard in `internal/agent/agent.go` detects failures purely with
`strings.HasPrefix(result.Content, "error")`, so an error message that doesn't start with
`error` will silently stop that guard from working.

## MetCode Quirks

These are known and cause it to work any fix's would need larger discussion.

- **`system_prompt.md` is embedded.** It's only written to `~/.metcode/system_prompt.md` when
  that file does **not** already exist. Editing the repo copy changes nothing for anyone who has
  already run MetCode.
- **Everything is jailed by `resolveSafePath`.** Every file tool and `run_command` resolve paths
  against the launch directory. Absolute `run_command` paths are rejected outright.
- **File creation goes through `edit_file`** To save tokens and simplify for small models create_file
  was combined with edit_file. Edit file with an empty old_text creates the file (and parent directories). 
  An existing file is only written when old_text matches exacly once, so a normal edit cannot silently 
  truncate it. Check `isProtected` in any new file handler; protected files match on
  basename only.
- **Only the first tool call per assistant message runs** (`firstCallOnly`). Extra calls in the
  same message are dropped silently.
- **Plan state lives on `Agent.Plan`, not in history**, so the two are cleared separately; `/clear` resets both.
- **Saved conversations are JSON in `.txt` files.** Keep the `.txt` extension for back-compat.

## Style notes
This is a fun project all about learning and providing good 
software for smaller models and old hardware. Try and follow 
what you see but I also understand there are multiple good ways
to do things.

## Reporting bugs and suggesting ideas

Open an issue. For a bug, the most helpful thing is: what you ran, what you expected, what
actually happened, and your OS + Go version + Ollama model.

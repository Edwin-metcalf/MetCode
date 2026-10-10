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
go install -o metcode ./cmd/metcode
./metcode
```

## Before you send a change

Run all four of these and make sure they're clean:

```bash
gofmt -l .    
go build ./...
go vet ./...
go test ./...
```

Tests do **not** need a running Ollama server or network access — `internal/agent`,
`internal/ollama`, and `internal/ui` fake the model API with `httptest`.

There is no Makefile, linter config, or CI gate keeping you out; the four commands above are
the whole contract. Keep pull requests focused — one change per PR is easiest to review.

## Project layout

```
cmd/metcode/           entry point: wires everything together
internal/agent/        the model <-> tool loop for a single turn
internal/ollama/       Ollama API types and HTTP client (the only code that knows the wire format)
internal/tools/        tool definitions and handlers, path safety
internal/plan/         plan step types (a bare data type, no logic)
internal/conversation/ saving and loading chat history
internal/prompt/       embedded default system prompt and loader
internal/ui/           banner, spinner, slash commands, input loop
```

`internal/ollama` is the only package that knows the Ollama wire format. `internal/tools`
depends on it; nothing depends on `ui`.

## Adding or changing a tool

You have to touch **four places** or the model will never see your change:

1. `internal/tools/definitions.go` — append to the package-level `tools.All` slice, which is
   sent on every request.
2. `internal/tools/tools.go` — add the tool name to the `Handle` switch.
3. The handler itself (`files.go`, `command.go`, or `planhelpers.go`), taking an
   `*ollama.ToolCall`.
4. `internal/prompt/system_prompt.md` — the prose tool list the model reads.

**Hard contract:** every handler must return an `ollama.Message` with `Role: "tool"`, and every
failure must have a `Content` that starts with the literal string `error`. The runaway-loop
guard in `internal/agent/agent.go` detects failures purely with
`strings.HasPrefix(result.Content, "error")`, so an error message that doesn't start with
`error` will silently stop that guard from working.

## MetCode Quirks

These are known and and cause it to work any fix's would need larger discussion.

- **`system_prompt.md` is embedded.** It's only written to `~/.metcode/system_prompt.md` when
  that file does **not** already exist. Editing the repo copy changes nothing for anyone who has
  already run MetCode.
- **Everything is jailed by `resolveSafePath`.** Every file tool and `run_command` resolve paths
  against the launch directory. Absolute `run_command` paths are rejected outright.
- **`create_file` never truncates.** It uses `O_EXCL` and errors if the file exists, pointing the
  model at `edit_file`. Check `isProtected` in any new file handler; protected files match on
  basename only.
- **Only the first tool call per assistant message runs** (`firstCallOnly`). Extra calls in the
  same message are dropped silently.
- **Plan state lives on `Agent.Plan`, not in history**, so `/clear` does not reset it.
- **Saved conversations are JSON in `.txt` files.** Keep the `.txt` extension for back-compat.

## Style notes
This is a fun project all about learning and providing good 
software for smaller models and old hardware. Try and follow 
what you see but I also understand there are multiple good ways
to do things.

## Reporting bugs and suggesting ideas

Open an issue. For a bug, the most helpful thing is: what you ran, what you expected, what
actually happened, and your OS + Go version + Ollama model.

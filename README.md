<div align="center">
<pre>
__  __      _    ____          _     
|  \/  | ___| |_ / ___|___   __| | ___ 
| |\/| |/ _ \ __| |   / _ \ / _` |/ _ \
| |  | |  __/ |_| |__| (_) | (_| |  __/
|_|  |_|\___|\__|\____\___/ \__,_|\___|

Local AI. Your Terminal. Your machines.
</pre>
<h3>Open-source coding agents for local models and modest hardware.</h3>

<p> Run an agentic coding assistant on your own machine using Ollama specifically optimized for small models and budget hardware. </p>

[![CI](https://github.com/Edwin-metcalf/MetCode/actions/workflows/ci.yml/badge.svg)](https://github.com/Edwin-metcalf/MetCode/actions/workflows/ci.yml)
</div>

## Table of Contents

- [Why MetCode?](#why-metcode)
- [Overview](#overview)
- [MetCode includes](#metcode-includes)
- [The Small Model Project](#the-small-model-project)
- [Benchmark harness](#benchmark-harness)
- [Model notes](#model-notes)
    - [qwen2.5-coder:7b](#qwen25-coder7b)
    - [llama3.1:latest](#llama31latest)
    - [Which to use](#which-to-use)
- [Setup Guide](#setup-guide)
- [Commands](#commands)
- [Model Safeguards](#model-safeguards)
- [Project layout](#project-layout)
- [Development](#development)
- [Known limitations](#known-limitations)
- [Contributing](#contributing)

---

## Why MetCode?

Ran out of Claude Code tokens? Tokens too expensive? Don't want to send all your data to some big company? Want to better understand how coding assistants work? Old gaming PC with a 1080 in it?

Welcome to MetCode! An approach to a local coding agent optimized for smaller models.

If small model optimization is interesting, you have an old GPU sitting around, or just want to experiment with local agents MetCode is right for you!

## Overview

MetCode is a local command line assistant built in Go on top of Ollama.
It provides an interactive interface for working with local LLMs and having your own personal assistant.

## MetCode includes

- **Chatting with Local Ollama Server** no cost, no limits, and no data going to big companies!
- **Model Selection** Download and use whatever model you want and `/change-model` mid session
- **File Tools** List directory, read files, create files or edit files, all sandboxed to the directory metcode is launched from
- **Robust Tool Calls** Recovers many tool calls that small models write as plain text, so a model that fumbles the format still gets the work done
- **Command Execution** Can run shell commands in your directory
- **Confirmation Before Changes** Every edit and command execution shows a diff and only completes after user input
- **Planning** for multi step tasks the model can write and update a plan as it works
- **Customizable system Prompt** Edit '~/.metcode/system_prompt.md' to change MetCode's behavior across all projects
- **Smart Compaction** `/compact` summarizes long conversations to free up context, built to keep file paths, decisions, and errors even with a small context window
- **Conversation Saving and Loading** to pick up where you left off

## The Small Model Project

MetCode thrives on getting the most out of small models that are run offline. For situations like your laptop on a plane without wifi.

Steps this project has taken to optimize small model performance:

- Recovering tool calls small models write as plain text or in malformed ways
- Compact tool definitions (folded creat_file into edit_file, saving a model round trip and context tokens)
- Context-aware compaction: history is flattened into a plain-text transcript and summarized in a single call. Built so Tool calls, file paths, and important information are visible regardless of chat template
    - Long tool results are trimmed to their head and tail (errors and file starts survive), and call arguments if re-read is needed


The project is currently working toward:

- Better prompts for 7-8B coding models (or smaller even)
- Efficient context management
- Better tool outputs
- Reduce unnecessary tool calls and context clutter
- Measuring each change on the [benchmark harness](#benchmark-harness), one change at a time

With a long term goal of how much can design make up for model size?

## Benchmark harness

To measure what harness design is worth, `metbench` runs the real agent loop against small fixture projects (a bug, and a test that must pass) and records whether the tests pass, how the turn ended, which tools were called, and how long it took. The method is one change at a time, each on its own branch: write a hypothesis, run the benchmark, keep or revert.

```bash
# from the repo root
go run ./cmd/metbench validate                              # every fixture must fail before the model touches it
go run ./cmd/metbench run -models qwen2.5-coder:7b -n 5     # one JSON line per run, under bench/results/
go run ./cmd/metbench report bench/results/<run>.jsonl      # print a table
```

`metbench` is a separate binary on purpose: it auto-approves edits inside a throwaway workspace, and that must never exist in the shipped `metcode`. Results so far are preliminary (small N, one machine, no fixed seed). See [bench/README.md](bench/README.md) for setup, how to add a task, and how to read the table.

## Model notes

Model choice affects MetCode more than any prompt or tool tweak measured so far. The two models used most with MetCode in development are
qwen2.5-coder:7b and llama3.1:latest. Both run on the test machine (Ubuntu GPU: Nvidia 1080) with `num_ctx=8192`.

| Model              | Unnecessary tool calls | Calls a tool when asked | Prefill per turn                |
| ------------------ | ---------------------- | ----------------------- | ------------------------------- |
| `qwen2.5-coder:7b` | 0 / 6                  | unreliable              | ~50ms (only turn 1 is uncached) |
| `llama3.1:latest`  | 5 / 6                  | reliable                | ~1.3s (on a fresh prompt)       |

### qwen2.5-coder:7b

Strengths

- Tool restraint: greetings and small talk get direct answers. Asked "hello" and "what is 2 + 2" it answered directly every time, with no extra round trip and no approval prompt.
- Speed: reuses the prompt cache across turns, so only the first turn pays full prefill (~1.5s). Later turns prefill in ~45-90ms, making it several times faster end to end.

Weaknesses

- Tool calls are unreliable. It often answers with the code or an explanation instead of invoking a tool, and may need to be pushed ("actually use the tool") before it calls one.
- When it does write a call as plain text, it sometimes wraps it in a markdown code fence, which MetCode's recovery currently misses.

### llama3.1:latest

Strengths

- Reliable tool use. When a tool is needed it calls the right one on the first try. Listing a directory, editing a file, and running a command all worked without prompting.
- Natural replies. Its final plain-text answers read well, so an unnecessary call is easy to miss unless you watch the `[tool]` lines.

Weaknesses

- Over-calls. It reaches for `list_directory` or `read_file` on greetings, thanks, and small talk, even right after being told no calls were needed. Each one costs an extra model round trip and an approval prompt.
- No prompt caching: it pays ~1.3s of prefill on every call, so its turns are roughly 7x slower.

### Which to use

- `qwen2.5-coder:7b` is the recommended default for speed and restraint, with the caveat that tool calls sometimes need a nudge.
- `llama3.1:latest` is the better pick when tool use needs to just work and you can accept the extra calls and the slower turns.

## Setup Guide

### Prerequisites

- [Go](https://go.dev/dl/) (see `go.mod` for version I used)
- [Ollama](https://ollama.com) running somewhere you can reach — either on the same machine, or on another machine on your network (I use tailscale and an old gaming PC)
- A model that supports tool calling. Recommended: `qwen2.5-coder:7b` or `llama3.1` both have been used with MetCode: see [Model notes](#model-notes)

### 1. Install

Install from latest release:

```bash
go install github.com/Edwin-metcalf/MetCode/cmd/metcode@latest
```

Or build from source

```bash
git clone https://github.com/Edwin-metcalf/MetCode
cd MetCode
go install ./cmd/metcode
```

This builds the MetCode binary and installs it into your Go bin directory. Check that it is in you 'PATH' (echo $PATH).

Run `metcode --version` (or `-v`) to print the version and exit. A plain `go build` prints `dev`; to stamp a version, pass it at build time:

```bash
go build -ldflags "-X main.version=v0.1.0" -o metcode ./cmd/metcode
```

### 2. Connect MetCode to Ollama server

If you are running Ollama on the same machine you want to use MetCode on, skip this (the default is http://localhost:11434)

To run Ollama on a different machine (like my childhood gaming PC) set 'OLLAMA_HOST' in your shell config to point at it:

```bash
export OLLAMA_HOST=http://<ollama-machine-ip>:11434
source ~/.zshrc
```

Ollama only listens on localhost by default. Therefore to connect from another machine we have to force it to allow remote connections. This is what I did to open up all connections (I am sure there is a better way but this worked for me)

Run this on the machine with Ollama (important part is putting the service and environment into the config)

```bash
sudo mkdir -p /etc/systemd/system/ollama.service.d
sudo tee /etc/systemd/system/ollama.service.d/override.conf > /dev/null <<'EOF'
[Service]
Environment="OLLAMA_HOST=0.0.0.0:11434"
EOF
sudo systemctl daemon-reload
sudo systemctl restart ollama
```

> HOWEVER '0.0.0.0' opens Ollama to any network interface on that machine, not just the connection over Tailscale. Only do this if you trust everything that is connected to the network the machine is on

Verify Connection with

```bash
curl $OLLAMA_HOST/api/tags
```

This should return your installed models that you can then use with MetCode

### 3. run it!

Move to your project directory and run:

```bash
metcode
```

On first run, MetCode create `'~/.metcode/'` and populates 'system_prompt.md' with a sensible default. Edit that to whatever you want and the changes will apply next run.
Conversations are also stored in `'~/.metcode/conversations/'`.

To stop chatting type `exit`

---

## Commands

Type any of these at the prompt (not sent to the model):

| Command         | What it does                                                                            |
| --------------- | --------------------------------------------------------------------------------------- |
| `/help`         | Shows the list of available commands                                                    |
| `/change-model` | Lists models available on your Ollama server and lets you switch                        |
| `/save`         | Saves the current conversation. Prompts for a name the first time; reuses it after that |
| `/load`         | Lists saved conversations and lets you pick one to resume                               |
| `/clear`        | Clears the current conversation, keeping only the system prompt                         |
| `/compact`      | Summarizes the conversation to free up context (keeps file paths, decisions, errors)    |

---

## Model Safeguards

Models can make mistakes, here are guardrails to minimize damage.

- **Sandboxed File Tools** File tools resolve paths against the directory you launched from and reject anything that escapes it. The check is lexical: symlinks inside the project are not resolved, so a symlink that points outside could still be followed.
- **You Approve Every Change** Edits show a diff and commands are shown before they run. Nothing is applied until you confirm.
- **Protected Files** The model cannot edit `go.mod`, `go.sum`, `main_test.go` or `system_prompt.md`. Other test files are not protected.
- **Limits on Runaway Loops** A single turn stops after a fixed number of tool-call rounds, or after repeated failing tool calls
- **Tolerant of Malformed Tool calls** If a model returns a tool call as text or in an unexpected format, MetCode tries to detect them and run them. Unknown tools are never executed.

Use your discretion: MetCode will run any command you approve. Commands start in your launch directory and absolute
command paths are rejected, but the arguments are not sandboxed, so an approved command can still read or change files outside the project (for example `cat ../other-project/file`).

## Project layout

```
cmd/metcode/          entry point: wires everything together
cmd/metbench/         benchmark command line: validate, run, report
bench/                task fixtures (bench/tasks) and results (bench/results)
internal/agent/       the model <-> tool loop, tool-call normalization, and conversation compaction
internal/bench/       benchmark engine: tasks, workspace, grading, runs, reports
internal/ollama/      Ollama API types and HTTP client
internal/tools/       tool definitions and handlers, path safety
internal/plan/        plan step types
internal/conversation/ saving and loading chat history
internal/prompt/      embedded default system prompt and loader
internal/ui/          banner, spinner, slash commands, input loop
```

## Development

```bash
go build ./...
go vet ./...
go test ./...
```

Tests don't need a running Ollama server; the model API is faked with `httptest`. 
The `internal/bench` tests run `go test` on temporary copies of the fixtures, so they need the Go toolchain on your `PATH` and take a few seconds. 
Each fixture under `bench/tasks` has its own `go.mod`, so `go build ./...` and `go test ./...` skip the fixtures on purpose.

## Known limitations

- Metcode recovers from small models hallucinating tool calls or producing malformed tool calls. However we are not perfect and errors may still occur.
- Only the first tool call per assistant message is executed; extras are dropped
- Creating a file uses `edit_file` with and empty old_text. An existing file is only changed when old_text matches exactly once, so a normal edit never truncates it.
- An edit rewrites the whole file after you confirm, so changes made to the file while the confirmation is pending are overwritten.
- `run_command` and file tools are jailed to the launch directory (lexically; see Model Safeguards).
- Editing the repo's `system_prompt.md` won't affect users who already ran MetCode once
- `/compact` only shows the summarizer the start and end of long tool results, so details from the middle of a large file are lost unless the model mentioned them(In the response to the user). File paths are kept, so the model can re-read the file if needed

## Contributing

Would love help on this project! See [CONTRIBUTING.md](CONTRIBUTING.md) for how to build, test, and contribute.

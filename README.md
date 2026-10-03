<div align="center">
<pre>
__  __      _    ____          _     
|  \/  | ___| |_ / ___|___   __| | ___ 
| |\/| |/ _ \ __| |   / _ \ / _` |/ _ \
| |  | |  __/ |_| |__| (_) | (_| |  __/
|_|  |_|\___|\__|\____\___/ \__,_|\___|

 Local AI. Your Terminal. Your machines.
</pre>
</div>

Ran out of Claude Code tokens? Tokens too expensive? Don't want to send all your data to some big company? I wanted to better understand how agentic coding assistants work?
Welcome to MetCode! A command line chat client for local LLMS. MetCode is built in Go on top of Ollama. 

---
## Overview 
MetCode is a local command line assistant built in Go on top of Ollama.
It provides an interactive interface for working with local LLMS and having your own personal assistant.

## MetCode includes
* **Chatting with Local Ollama Server** no cost, no limits, and no data going to big companies!
* **Model Selection** Download and use whatever model you want and `/change-model` mid session
* **File Tools** list directory, read files, create files, edit files, all sandboxed to the directory metcode is launched from
* **Command Execution** Can run shell commands in your directory
* **Confirmation Before Changes** Every edit and command execution shows a diff and only completes after user input
* **Planning** for multi step tasks the model can write and update a plan as it works
* **Customizable system Prompt** edit '~/.metcode/system_prompt.md' to change Metcodes behavior across all projects
* **Conversation Saving and Loading** to pick up where you left off


---
## Setup Guide

### Prerequisites
* [Go](https://go.dev/dl/) (see `go.mod`for version I used)
* [Ollama](https://ollama.com) running somewhere you can reach — either on the same machine, or on another machine on your network (I use tailscale and an old gaming PC)
* A model that supports tool calling. MetCode has been tested with llama3.1

### 1. Install
```bash
git clone https://github.com/Edwin-metcalf/MetCode
cd MetCode
go build -o ./cmd/metcode
```
Or if you have go installed:
```bash 
go install github.com/Edwin-metcalf/MetCode/cmd/metcode@latest
```

This should build the MetCode binary and install it into GOPATH (good to check if the go bin directory is in you 'PATH' run echo $PATH to check)

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
 
| Command | What it does |
|---|---|
| `/help` | Shows the list of available commands |
| `/change-model` | Lists models available on your Ollama server and lets you switch |
| `/save` | Saves the current conversation. Prompts for a name the first time; reuses it after that |
| `/load` | Lists saved conversations and lets you pick one to resume |
| `/clear` | Clears the current conversation, keeping only the system prompt |

---
## Model Safeguards
Models can make mistakes, here are guardrails to minimize damage.

* **Sandbox File Access** File tools resolve paths against the directory you launched from and rejects anything that escapes it
* **You Approve Every Change** Edits display a diff and commands show before running/editing
* **Protected Files** A list of protected files
* **Limits on Runaway Loops** A single turn stops after a fixed number of tool-call rounds, or after repeated failing tool calls

Use your discretion it will run any command you approve it. And the path is not restricted to the project directory it is free to go into sub directories.


## Project layout
 
```
cmd/metcode/          entry point: wires everything together
internal/agent/       the model <-> tool loop for a single turn
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
 
## Roadmap
 
- Web search tool
- `/summarize` command
- Terminal UI overhaul
- Skills

Ideas and bug reports are awlays welcome! open an issue.
 
## Contributing
 
Contributions are welcome! I am learning and welcome any help I can get.
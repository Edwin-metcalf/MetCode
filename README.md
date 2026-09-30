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
* **Chatting with Local Ollama Server** Converse with context and all local, no cost, no limits, and no security issues!
* **Model Selection** Download and use whatever model you want
* **File Reading** ability to read files on your system
* **File Creation and Editing** Can create new files or edit pre existing files, shows a preview and a y/n confirmation before anything is written
* **Command Execution** Can run shell commands, also has a preview with y/n before anything is ran
* **Planning** breaks multi-step tasks into tracked plan
* **Customizable system Prompt** edit '~/.metcode/system_prompt.md' to change Metcodes behavior across all projects
* **Conversation Saving and Loading** ability to save conversations and reload them later, see these and other [commands](#commands) below

## More stuff coming!
Like web search and UI overhaul

---
## Setup Guide

### Prerequisites
* [Go](https://go.dev/dl/) 1.21 or newer — check with `go version`
* [Ollama](https://ollama.com) running somewhere you can reach — either on the same machine, or on another machine on your network (I use tailscale and an old gaming PC)

### 1. Install
```bash
git clone https://github.com/Edwin-metcalf/MetCode
cd MetCode
go install .
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

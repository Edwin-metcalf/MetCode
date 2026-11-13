package tools

import (
	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

var All = []ollama.Tool{
	// this is global(package level) dont edited it in runtime if wanting to add more tools
	//make a copy and use that instead
	{
		Type: "function",
		Function: ollama.ToolFunction{
			Name:        "list_directory",
			Description: "list all files in the current directory. Use when asked about directory content or when you need the name of files to then read them or edit them.",
			Parameters: ollama.Parameters{
				Type:       "object",
				Required:   []string{},
				Properties: map[string]any{},
			},
		},
	},
	{
		Type: "function",
		Function: ollama.ToolFunction{
			Name:        "read_file",
			Description: "reads the contents of a file and returns it as plain text. Use when the user asks about context of a file or wants you to refer to code/text from a file that is not in the conversation.",
			Parameters: ollama.Parameters{
				Type:     "object",
				Required: []string{"path"},
				Properties: map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "relative path to the file, relative to the current working directory. Example 'main.go' or 'internal/utils/helper.go'. Use list_directory first to get exact file names",
					},
				},
			},
		},
	},
	{
		Type: "function",
		Function: ollama.ToolFunction{
			// Two payload shapes share this tool because small models reach for
			// both: old_text+new_text replaces a snippet, content replaces the
			// whole file. content is never a snippet payload — applying a whole
			// file at a snippet site would splice the file into itself — so when
			// old_text is present content is still rejected with an error that
			// teaches new_text. The handler is the teaching channel; the schema
			// states both modes.
			Name:        "edit_file",
			Description: "Edit a file. Two ways to call it: (1) replace a snippet by providing old_text and new_text; (2) replace or create the whole file by providing content. old_text must match the file exactly and appear only once; call read_file first and copy it exactly. For Go files content must be the complete file including the package clause.",
			Parameters: ollama.Parameters{
				Type:     "object",
				Required: []string{"path"},
				Properties: map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "relative path to the file you want to edit. Relative to the current working directory. Example: 'example.go' or 'stuff/things/example.go'",
					},
					"old_text": map[string]any{
						"type":        "string",
						"description": "The exact text to replace, copied from read_file output. Keep indentation exactly as shown, including tabs. Must appear exactly once in the file. Use an empty string only when creating a file that does not exist yet.",
					},
					"new_text": map[string]any{
						"type":        "string",
						"description": "The replacement text for old_text. Only the lines that replace old_text, not the whole file. Use content instead to replace the entire file.",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "The complete new contents of the whole file. Use this instead of old_text/new_text to replace or create a file. Must include the package clause for Go files.",
					},
				},
			},
		},
	},
	{
		Type: "function",
		Function: ollama.ToolFunction{
			Name:        "run_command",
			Description: "Run a terminal command. Use only when the user asks you to run a command or when one is needed to complete the task not for responses in plain text.",
			Parameters: ollama.Parameters{
				Type:     "object",
				Required: []string{"command"},
				Properties: map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The executable/command name only, no arguments or subcommands attached — e.g. 'go', 'ls', 'grep'. Put subcommands and flags in 'arguments' instead (e.g. command: 'go', arguments: [\"run\", \"main.go\"])",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "the path relative to the current directory. This is optional if it is empty it will automatically run in the current directory. Must be relative and must stay inside the current directory. Example: 'stuff/things/'",
					},
					"arguments": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "list of arguments to pass to the command, each as a separate element. Example: [\"-la\", \"/tmp\"]",
					},
				},
			},
		},
	},
	{
		Type: "function",
		Function: ollama.ToolFunction{
			Name:        "create_plan",
			Description: "create a plan to keep you organized with task descriptions and their status which could be pending, in_progress, or done",
			Parameters: ollama.Parameters{
				Type:     "object",
				Required: []string{"descriptions"},
				Properties: map[string]any{
					"descriptions": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "list of descriptions that describe the individual step of the plan. ",
					},
				},
			},
		},
	},
	{
		Type: "function",
		Function: ollama.ToolFunction{
			Name:        "update_plan",
			Description: "update the status of a step in the plan",
			Parameters: ollama.Parameters{
				Type:     "object",
				Required: []string{"id", "status"},
				Properties: map[string]any{
					"id": map[string]any{
						"type":        "integer",
						"description": "the ID of the step in the plan that is supposed to be updated",
					},
					"status": map[string]any{
						"type":        "string",
						"enum":        []string{"pending", "in_progress", "done"},
						"description": "The new status to update the step of the plan",
					},
				},
			},
		},
	},
}

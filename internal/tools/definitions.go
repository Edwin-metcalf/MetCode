package tools

import (
	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

var All = []ollama.Tool{
	// this is global(package level) dont edit could be risky just copy if needed
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
			Description: "reads the context of a file and returns it as plain text. Use when the user asks about context of a file or wants you to refer to code/text from a file that is not in the conversation.",
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
			Name:        "edit_file",
			Description: "Edit an existing file by replacing old_text with new_text. To create a new file, pass an empty old_text and put the entire file contents in new_text. old_text must match the file exactly and appear only once. Call read_file first and copy it from the real content.",
			Parameters: ollama.Parameters{
				Type:     "object",
				Required: []string{"path", "old_text", "new_text"},
				Properties: map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "relative path to the file you want to edit. Relative to the current working directory. Example: 'example.go' or 'stuff/things/example.go'",
					},
					"old_text": map[string]any{
						"type":        "string",
						"description": "The exact text to replace, including whitespace. Must appear exactly once. For a new file, use an empty string '' and new_text becomes the whole file.",
					},
					"new_text": map[string]any{
						"type":        "string",
						"description": "The replacement text. This is the entire new contents.",
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

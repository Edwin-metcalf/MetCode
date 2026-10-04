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
			Name:        "create_file",
			Description: "creates a new empty file. Use edit_file after to add content",
			Parameters: ollama.Parameters{
				Type:     "object",
				Required: []string{"path"},
				Properties: map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "relative path and name of a file. Relative to the current working directory. Examples: 'example.go' or 'stuff/example.go'",
					},
				},
			},
		},
	},
	{
		Type: "function",
		Function: ollama.ToolFunction{
			Name:        "edit_file",
			Description: "edit a file. If the file is empty (e.g. just created with create_file), use an empty string for old_text. If the file is not empty old_text will have its contents read with read_file and you are editing those contents",
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
						"description": "the text that was in the file. old_text must match the file's content exactly, including whitespace, indentation, and proper line breaks or the edit will fail.",
					},
					"new_text": map[string]any{
						"type":        "string",
						"description": "the new text that is going into the file after the edit",
					},
				},
			},
		},
	},
	{
		Type: "function",
		Function: ollama.ToolFunction{
			Name:        "run_command",
			Description: "Run a command in the terminal. Ability to run in local directory on an empty path or with a declared path.",
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
						"description": "the path relative to the current directory. This is optional if it is empty it will automatically run in the current directory. Example: '/stuff/things/'",
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

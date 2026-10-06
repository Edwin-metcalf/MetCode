// the tool package handles all the stuff we need for tools for the model to use
package tools

import (
	"fmt"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/plan"
)

func Handle(call ollama.ToolCall, root string, currentPlan *[]plan.Item) ollama.Message {
	fmt.Printf("[tool] %s %v\n", call.Function.Name, call.Function.Arguments)
	toolName := call.Function.Name
	switch toolName {
	case "list_directory":
		return listDirectoryHelper(root)
	case "read_file":
		return readFileHelper(&call, root)
	case "create_file":
		return createFileHelper(&call, root)
	case "edit_file":
		return editFileHelper(&call, root)
	case "run_command":
		return runCommandHelper(&call, root)
	case "create_plan":
		return createPlanHelper(&call, currentPlan)
	case "update_plan":
		return updatePlanItemHelper(&call, currentPlan)
	default:
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("error: unknown tool: %v", toolName)}
	}
}

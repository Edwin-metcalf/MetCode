// the tool package handles all the stuff we need for tools for the model to use
package tools

import (
	"fmt"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/plan"
)

// Quiet silences the per-call log and the confirm prompts. The UI leaves it
// false; bench sets it so a run emits only its JSON line.
var Quiet bool

// printLoud is where tool printing goes. Quiet turns it off.
func printLoud(a ...any) {
	if Quiet {
		return
	}
	fmt.Println(a...)
}

func Handle(call ollama.ToolCall, root string, currentPlan *[]plan.Item) ollama.Message {
	if !Quiet {
		fmt.Printf("[tool] %s %v\n", call.Function.Name, call.Function.Arguments)
	}
	toolName := call.Function.Name
	switch toolName {
	case "list_directory":
		return listDirectoryHelper(root)
	case "read_file":
		return readFileHelper(&call, root)
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

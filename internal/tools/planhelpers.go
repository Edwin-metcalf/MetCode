package tools

import (
	"fmt"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/plan"
)

func createPlanHelper(call *ollama.ToolCall, items *[]plan.Item) ollama.Message {
	var descriptions []string
	if rawDesc, ok := call.Function.Arguments["descriptions"].([]any); ok {
		for _, a := range rawDesc {
			if str, ok := a.(string); ok {
				descriptions = append(descriptions, str)
			}
		}
	}
	if len(descriptions) == 0 {
		return ollama.Message{
			Role:    "tool",
			Content: "error: at least one description is required",
		}
	}

	var planItemList []plan.Item
	curId := 1.0
	returnString := "plan: "
	for _, description := range descriptions {
		newItem := plan.Item{
			Id:          curId,
			Description: description,
			Status:      plan.Status("pending"),
		}
		planItemList = append(planItemList, newItem)
		localAdd := fmt.Sprintf("Id: %v, Description: %v, Status pending \n", curId, description)
		returnString = returnString + localAdd

		curId += 1
	}

	*items = planItemList

	return ollama.Message{
		Role:    "tool",
		Content: fmt.Sprintf("New plan created succesfully: %v", returnString),
	}
}

func updatePlanItemHelper(call *ollama.ToolCall, itemList *[]plan.Item) ollama.Message {
	// seemss a little absurd to store them as float 64s but its whats returned from the json unmarshaling
	id, ok := call.Function.Arguments["id"].(float64)
	if !ok {
		return ollama.Message{
			Role:    "tool",
			Content: "error: invalid id",
		}
	}

	status, ok := call.Function.Arguments["status"].(string)
	if !ok {
		return ollama.Message{
			Role:    "tool",
			Content: "error: invalid status",
		}
	}
	switch plan.Status(status) {
	case plan.StatusDone, plan.StatusInProgress, plan.StatusPending:
		for i := range *itemList {
			if (*itemList)[i].Id == id {
				(*itemList)[i].Status = plan.Status(status)

				return ollama.Message{
					Role:    "tool",
					Content: "succesfully updated plan item",
				}
			}
		}

	default:
		return ollama.Message{
			Role:    "tool",
			Content: "error invalid status",
		}
	}

	return ollama.Message{
		Role:    "tool",
		Content: "error: no plan item found with that ID",
	}
}

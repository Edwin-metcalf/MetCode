package main

import (
	"strings"
	"testing"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/plan"
)

func TestReadDirectoryHelper_ValidFile(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "read_file",
			Arguments: map[string]any{"path": "main.go"},
		},
	}

	result := readFileHelper(call, ".")

	if result.Role != "tool" {
		t.Errorf("Role is not tool it is %v", result.Role)
	}
	if result.Content == "" {
		t.Error("Content is empty ")
	}
}

func TestReadDirectoryHelper_NoPath(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "read_file",
			Arguments: map[string]any{},
		},
	}
	result := readFileHelper(call, ".")
	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %q", result.Content)
	}
}

func TestProtectedFiles(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "main.go", "old_text": "package main\ngood code:)", "new_text": "bad code:("},
		},
	}
	result := editFileHelper(call, ".")

	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %v", result.Content)
	}
}

func TestEditFileValid(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "example.go", "old_text": "bad stuff :(", "new_text": "good stuff :)"},
		},
	}
	result := editFileHelper(call, ".")

	if result.Role != "tool" {
		t.Errorf("error: Role is not a tool it is: %v", result.Role)
	}

	if result.Content == "" {
		t.Error("content is empty")
	}
}

func TestReadFileHelper_PathEscapesRoot(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "read_file",
			Arguments: map[string]any{"path": "../../../passwords"},
		},
	}

	result := readFileHelper(call, ".")
	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %q", result.Content)
	}
}

func TestEditFileHelper_PathEscapesRoot(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "../outside.go", "old_text": "foo", "new_text": "bar"},
		},
	}
	result := editFileHelper(call, ".")
	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %q", result.Content)
	}
}

func TestUpdatePlanItemHelper_ValidUpdate(t *testing.T) {
	items := []plan.Item{
		{Id: 1.0, Description: "read main.go", Status: plan.StatusPending},
		{Id: 2.0, Description: "write tests", Status: plan.StatusPending},
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name: "update_plan",
			Arguments: map[string]any{
				"id":     1.0,
				"status": "in_progress",
			},
		},
	}

	result := updatePlanItemHelper(call, &items)

	if strings.Contains(result.Content, "error") {
		t.Errorf("expected success but got %q", result.Content)
	}

	if items[0].Status != plan.StatusInProgress {
		t.Errorf("expected item 0 to be in_progress, got %v", items[0].Status)
	}

	if items[1].Status != plan.StatusPending {
		t.Errorf("item 1 should be untouched, got %v", items[1].Status)
	}
}

func TestUpdatePlanItemHelper_InvalidStatus(t *testing.T) {
	items := []plan.Item{
		{Id: 1.0, Description: "read main.go", Status: plan.StatusPending},
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name: "update_plan",
			Arguments: map[string]any{
				"id":     1.0,
				"status": "completed", // not one of the three valid values
			},
		},
	}

	result := updatePlanItemHelper(call, &items)

	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected rejection of invalid status, got %q", result.Content)
	}
	if items[0].Status != plan.StatusPending {
		t.Errorf("item should be untouched after rejected update, got %v", items[0].Status)
	}
}

func TestCreatePlanHelper_MissingDescriptions(t *testing.T) {
	app := &App{}
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "create_plan",
			Arguments: map[string]any{},
		},
	}

	result := createPlanHelper(call, app)

	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %q", result.Content)
	}
	if len(app.CurrentPlan) != 0 {
		t.Errorf("plan should remain empty, got %d items", len(app.CurrentPlan))
	}
}

func TestCreatePlanHelper_DoesNotClobberExistingPlan(t *testing.T) {
	app := &App{
		CurrentPlan: []plan.Item{
			{Id: 1.0, Description: "existing step", Status: plan.StatusInProgress},
		},
	}
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "create_plan",
			Arguments: map[string]any{},
		},
	}

	createPlanHelper(call, app)

	if len(app.CurrentPlan) != 1 {
		t.Fatalf("expected existing plan to survive a rejected call, got %d items", len(app.CurrentPlan))
	}
	if app.CurrentPlan[0].Status != plan.StatusInProgress {
		t.Errorf("existing item should be untouched")
	}
}

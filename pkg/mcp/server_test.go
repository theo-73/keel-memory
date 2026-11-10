package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"keel/pkg/model"
	"keel/pkg/store"
)

func TestMCPServerLifecycle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-mcp-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := store.Init(tmpDir, "MCP Test", "Testing MCP server")
	if err != nil {
		t.Fatal(err)
	}

	// Prepare mock stdio
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"keel_task_create","arguments":{"id":"test-1","title":"Test Task"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"keel_brief","arguments":{}}}`,
	}, "\n") + "\n"

	in := strings.NewReader(input)
	var out bytes.Buffer

	server := NewServer(s, in, &out)
	if err := server.Run(); err != nil {
		t.Fatalf("server Run failed: %v", err)
	}

	outputLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(outputLines) != 4 {
		t.Fatalf("expected 4 JSON-RPC responses, got %d. Output:\n%s", len(outputLines), out.String())
	}

	// Verify response 1 (initialize)
	var initResp JSONRPCResponse
	if err := json.Unmarshal([]byte(outputLines[0]), &initResp); err != nil {
		t.Fatalf("parse init response failed: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("init error: %v", initResp.Error)
	}

	// Verify response 3 (task created)
	var createResp JSONRPCResponse
	if err := json.Unmarshal([]byte(outputLines[2]), &createResp); err != nil {
		t.Fatalf("parse task create response failed: %v", err)
	}
	if createResp.Error != nil {
		t.Fatalf("task create error: %v", createResp.Error)
	}

	// Verify task exists in store
	task, err := s.GetTask("test-1")
	if err != nil || task.Title != "Test Task" {
		t.Fatalf("expected task-1 in store, got %v, err: %v", task, err)
	}
}

func TestMCPServerGoalsAndResources(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-mcp-res-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := store.Init(tmpDir, "MCP Resources Test", "Testing MCP server resources and goal operations")
	if err != nil {
		t.Fatal(err)
	}

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"keel://project/tasks"}}`,
		`{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"keel://project/goals"}}`,
		`{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"keel://project/worklogs"}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"keel_goal_create","arguments":{"id":"goal-api","title":"API Milestone","status":"active"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"keel_goal_list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"keel_goal_update","arguments":{"id":"goal-api","status":"completed"}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"keel_task_create","arguments":{"id":"task-del","title":"To Delete","goal_id":"goal-api"}}}`,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"keel_task_update","arguments":{"id":"task-del","status":"in_progress","priority":"high","description":"in progress note","blocked_reason":""}}}`,
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"keel_task_ready","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"keel_worklog_record","arguments":{"summary":"Progress made","harness":"claude","next_steps":["verify"]}}}`,
		`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"keel_doctor","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"keel_task_delete","arguments":{"id":"task-del"}}}`,
		`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"unknown_tool","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":15,"method":"unknown_method","params":{}}`,
	}, "\n") + "\n"

	in := strings.NewReader(input)
	var out bytes.Buffer

	server := NewServer(s, in, &out)
	if err := server.Run(); err != nil {
		t.Fatalf("server Run failed: %v", err)
	}

	outputLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(outputLines) != 15 {
		t.Fatalf("expected 15 responses, got %d. Output:\n%s", len(outputLines), out.String())
	}

	// Verify goal-api was updated to completed
	goal, err := s.GetGoal("goal-api")
	if err != nil || goal.Status != "completed" {
		t.Fatalf("expected goal status completed, got %v, err: %v", goal, err)
	}

	// Verify task-del was deleted
	_, err = s.GetTask("task-del")
	if err == nil {
		t.Fatalf("expected task-del to be deleted")
	}
}

func TestMCPExtendedTools(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-mcp-ext-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := store.Init(tmpDir, "MCP Extended Test", "Testing new MCP tools")
	if err != nil {
		t.Fatal(err)
	}

	// Setup initial goal, task, worklog
	_ = s.CreateGoal(model.Goal{ID: "g1", Title: "Initial Goal", Outcome: "Active outcome", Status: "active"})
	_ = s.CreateGoal(model.Goal{ID: "g2", Title: "Goal to Keep", Outcome: "Temporary", Status: "active"})
	_ = s.CreateGoal(model.Goal{ID: "g3", Title: "Goal to Delete", Outcome: "Temporary", Status: "active"})
	_ = s.CreateTask(model.Task{ID: "t1", Title: "Initial Task", GoalID: "g1", Status: "pending"})
	_ = s.RecordWorklog(model.WorklogEntry{Harness: "test-harness", Summary: "Recorded progress", NextSteps: []string{"verify"}})

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"keel_task_get","arguments":{"id":"t1"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"keel_goal_get","arguments":{"id":"g1"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"keel_goal_select","arguments":{"id":"g1"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"keel_task_update","arguments":{"id":"t1","goalId":"g2","deliverables":["bin/app","docs/spec.md"],"priority":"critical"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"keel_worklog_list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"keel_task_list","arguments":{"goalId":"g2"}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"prompts/list","params":{}}`,
		`{"jsonrpc":"2.0","id":8,"method":"prompts/get","params":{"name":"keel_orient"}}`,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"keel_goal_delete","arguments":{"id":"g3"}}}`,
	}, "\n") + "\n"

	in := strings.NewReader(input)
	var out bytes.Buffer

	server := NewServer(s, in, &out)
	if err := server.Run(); err != nil {
		t.Fatalf("server Run failed: %v", err)
	}

	outputLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(outputLines) != 9 {
		t.Fatalf("expected 9 responses, got %d. Output:\n%s", len(outputLines), out.String())
	}

	// 1. Verify task update applied goalId, deliverables, priority
	task, err := s.GetTask("t1")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if task.GoalID != "g2" {
		t.Errorf("expected task GoalID to be g2, got %q", task.GoalID)
	}
	if len(task.Deliverables) != 2 || task.Deliverables[0] != "bin/app" {
		t.Errorf("expected deliverables [bin/app docs/spec.md], got %v", task.Deliverables)
	}
	if task.Priority != "critical" {
		t.Errorf("expected priority critical, got %q", task.Priority)
	}

	// 2. Verify active goal is g1
	state, err := s.State()
	if err != nil {
		t.Fatalf("State failed: %v", err)
	}
	if state.CurrentGoalID != "g1" {
		t.Errorf("expected currentGoalId g1, got %q", state.CurrentGoalID)
	}

	// 3. Verify g3 was deleted
	_, err = s.GetGoal("g3")
	if err == nil {
		t.Errorf("expected goal g3 to be deleted, but still found")
	}

	// 4. Verify cascade unlinks task when g2 is deleted
	_ = s.DeleteGoal("g2")
	taskAfterDelete, err := s.GetTask("t1")
	if err != nil || taskAfterDelete.GoalID != "" {
		t.Errorf("expected task GoalID unlinked after g2 deleted, got %q", taskAfterDelete.GoalID)
	}
}

func TestMCPServerAutoClearBlocker(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-mcp-blocker-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := store.Init(tmpDir, "MCP Blocker Test", "Testing blocker auto-clearing")
	if err != nil {
		t.Fatal(err)
	}

	blockedTask := model.Task{
		ID:            "task-blocked",
		Title:         "Stalled Task",
		Status:        "blocked",
		BlockedReason: "Waiting on external approval",
	}
	if err := s.CreateTask(blockedTask); err != nil {
		t.Fatal(err)
	}

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"keel_task_update","arguments":{"id":"task-blocked","status":"in_progress"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"keel_task_create","arguments":{"title":"Deploy Lambda Worker"}}}`,
	}, "\n") + "\n"

	in := strings.NewReader(input)
	var out bytes.Buffer

	server := NewServer(s, in, &out)
	if err := server.Run(); err != nil {
		t.Fatalf("server Run failed: %v", err)
	}

	updated, err := s.GetTask("task-blocked")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "in_progress" {
		t.Errorf("expected status 'in_progress', got %q", updated.Status)
	}
	if updated.BlockedReason != "" {
		t.Errorf("expected BlockedReason to be cleared, got %q", updated.BlockedReason)
	}

	state, err := s.State()
	if err != nil {
		t.Fatal(err)
	}
	if state.Health != "on_track" {
		t.Errorf("expected health 'on_track', got %q", state.Health)
	}

	created, err := s.GetTask("deploy-lambda-worker")
	if err != nil {
		t.Fatalf("expected auto-slug 'deploy-lambda-worker' to be created: %v", err)
	}
	if created.Title != "Deploy Lambda Worker" {
		t.Errorf("expected title 'Deploy Lambda Worker', got %q", created.Title)
	}
}

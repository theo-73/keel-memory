package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"keel/pkg/model"
)

func TestStoreLifecycle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-store-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Init
	s, err := Init(tmpDir, "Test Project", "A project for testing Keel")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	manifest, err := s.Manifest()
	if err != nil || manifest.Name != "Test Project" {
		t.Fatalf("unexpected manifest: %v, err: %v", manifest, err)
	}

	// Goals
	goal := model.Goal{
		ID:      "launch-v1",
		Title:   "Launch Version 1.0",
		Outcome: "Deliver reliable agent memory",
		Status:  "active",
	}
	if err := s.CreateGoal(goal); err != nil {
		t.Fatalf("CreateGoal failed: %v", err)
	}

	goals, err := s.ListGoals()
	if err != nil || len(goals) != 1 {
		t.Fatalf("expected 1 goal, got %d (err: %v)", len(goals), err)
	}

	// Tasks
	t1 := model.Task{
		ID:     "task-setup",
		GoalID: "launch-v1",
		Title:  "Setup project structure",
		Status: "completed",
	}
	t2 := model.Task{
		ID:        "task-cli",
		GoalID:    "launch-v1",
		Title:     "Build CLI binary",
		Status:    "in_progress",
		DependsOn: []string{"task-setup"},
	}

	if err := s.CreateTask(t1); err != nil {
		t.Fatalf("CreateTask t1 failed: %v", err)
	}
	if err := s.CreateTask(t2); err != nil {
		t.Fatalf("CreateTask t2 failed: %v", err)
	}

	tasks, err := s.ListTasks()
	if err != nil || len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	// Verify state reconciliation
	state, err := s.State()
	if err != nil {
		t.Fatalf("State read failed: %v", err)
	}
	if len(state.ActiveTaskIDs) != 1 || state.ActiveTaskIDs[0] != "task-cli" {
		t.Errorf("expected task-cli to be active, got %v", state.ActiveTaskIDs)
	}

	// Worklog
	worklog := model.WorklogEntry{
		Harness:   "claude-code",
		Summary:   "Completed setup and started CLI",
		NextSteps: []string{"Finish task commands"},
	}
	if err := s.RecordWorklog(worklog); err != nil {
		t.Fatalf("RecordWorklog failed: %v", err)
	}

	latest, err := s.LatestWorklog()
	if err != nil || latest == nil || latest.Summary != worklog.Summary {
		t.Fatalf("expected latest worklog, got %v, err: %v", latest, err)
	}

	// Verify rules files created
	if _, err := os.Stat(filepath.Join(tmpDir, "AGENTS.md")); err != nil {
		t.Errorf("AGENTS.md missing")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "CLAUDE.md")); err != nil {
		t.Errorf("CLAUDE.md missing")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".cursor", "rules", "keel.mdc")); err != nil {
		t.Errorf("keel.mdc missing")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".windsurfrules")); err != nil {
		t.Errorf(".windsurfrules missing")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".github", "copilot-instructions.md")); err != nil {
		t.Errorf("copilot-instructions.md missing")
	}
}

func TestStoreConcurrencyWithLock(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-concurrent-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := Init(tmpDir, "Concurrent Project", "Testing store concurrency")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	const workerCount = 10
	errCh := make(chan error, workerCount*2)

	// Concurrently create tasks
	for i := 0; i < workerCount; i++ {
		go func(idx int) {
			task := model.Task{
				ID:     "task-" + string(rune('a'+idx)),
				Title:  "Concurrent task",
				Status: "pending",
			}
			errCh <- s.CreateTask(task)
		}(i)
	}

	// Concurrently record worklogs
	for i := 0; i < workerCount; i++ {
		go func(idx int) {
			wl := model.WorklogEntry{
				Harness: "agent",
				Summary: "Log summary",
			}
			errCh <- s.RecordWorklog(wl)
		}(i)
	}

	for i := 0; i < workerCount*2; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("concurrent operation failed: %v", err)
		}
	}

	tasks, err := s.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if len(tasks) != workerCount {
		t.Fatalf("expected %d tasks, got %d", workerCount, len(tasks))
	}

	worklogs, err := s.ListWorklogs()
	if err != nil {
		t.Fatalf("ListWorklogs failed: %v", err)
	}
	if len(worklogs) != workerCount {
		t.Fatalf("expected %d worklogs, got %d", workerCount, len(worklogs))
	}
}

func TestDeleteTaskAndGoal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-delete-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := Init(tmpDir, "Delete Test", "Testing delete operations")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	goal := model.Goal{
		ID:     "goal-to-delete",
		Title:  "Temporary Goal",
		Status: "active",
	}
	if err := s.CreateGoal(goal); err != nil {
		t.Fatalf("CreateGoal failed: %v", err)
	}

	task := model.Task{
		ID:     "task-to-delete",
		GoalID: "goal-to-delete",
		Title:  "Temporary Task",
		Status: "pending",
	}
	if err := s.CreateTask(task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Delete task
	if err := s.DeleteTask("task-to-delete"); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}
	_, err = s.GetTask("task-to-delete")
	if err == nil {
		t.Errorf("expected task to be deleted")
	}

	// Set current goal
	if err := s.SetCurrentGoal("goal-to-delete"); err != nil {
		t.Fatalf("SetCurrentGoal failed: %v", err)
	}
	state, err := s.State()
	if err != nil || state.CurrentGoalID != "goal-to-delete" {
		t.Errorf("expected current goal ID to be set in state, got %v", state)
	}

	// Delete goal
	if err := s.DeleteGoal("goal-to-delete"); err != nil {
		t.Fatalf("DeleteGoal failed: %v", err)
	}
	state, err = s.State()
	if err != nil || state.CurrentGoalID != "" {
		t.Errorf("expected current goal ID to be reset after deleting active goal")
	}
}

func TestReconcileStateCompletedPhase(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-reconcile-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := Init(tmpDir, "Reconcile Project", "Testing phase reconciliation")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	task := model.Task{
		ID:     "task-complete",
		Title:  "All done task",
		Status: "completed",
	}
	if err := s.CreateTask(task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Add cancelled task; should not prevent completion
	cancelledTask := model.Task{
		ID:     "task-cancelled",
		Title:  "Dropped task",
		Status: "cancelled",
	}
	if err := s.CreateTask(cancelledTask); err != nil {
		t.Fatalf("CreateTask cancelled failed: %v", err)
	}

	state, err := s.State()
	if err != nil {
		t.Fatalf("State failed: %v", err)
	}
	if state.Health != "completed" {
		t.Errorf("expected health 'completed', got '%s'", state.Health)
	}
	if state.CurrentPhase != "Verification & Handoff" {
		t.Errorf("expected phase 'Verification & Handoff', got '%s'", state.CurrentPhase)
	}
}

func TestHealthNotAtRiskOnNormalPrerequisites(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-prereq-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := Init(tmpDir, "Prereq Project", "Testing normal prerequisites health")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	t1 := model.Task{
		ID:     "t1",
		Title:  "First step",
		Status: "in_progress",
	}
	t2 := model.Task{
		ID:        "t2",
		Title:     "Second step",
		Status:    "pending",
		DependsOn: []string{"t1"},
	}

	if err := s.CreateTask(t1); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTask(t2); err != nil {
		t.Fatal(err)
	}

	state, err := s.State()
	if err != nil {
		t.Fatal(err)
	}
	// Normal downstream dependency with t1 in_progress should remain on_track, NOT at_risk!
	if state.Health != "on_track" {
		t.Errorf("expected health 'on_track' for normal dependencies, got %q", state.Health)
	}
}

func TestLatestWorklogO1(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-worklog-o1-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := Init(tmpDir, "Worklog Test", "Testing O(1) worklog retrieval")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Initially nil
	latest, err := s.LatestWorklog()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if latest != nil {
		t.Fatalf("expected nil worklog initially, got %+v", latest)
	}

	// Record multiple worklogs
	_ = s.RecordWorklog(model.WorklogEntry{Harness: "claude", Summary: "First session", NextSteps: []string{"next-1"}})
	_ = s.RecordWorklog(model.WorklogEntry{Harness: "cursor", Summary: "Second session", NextSteps: []string{"next-2"}})
	_ = s.RecordWorklog(model.WorklogEntry{Harness: "antigravity", Summary: "Third session - latest", NextSteps: []string{"done"}})

	latest, err = s.LatestWorklog()
	if err != nil {
		t.Fatalf("LatestWorklog failed: %v", err)
	}
	if latest == nil {
		t.Fatal("expected latest worklog, got nil")
	}
	if latest.Summary != "Third session - latest" {
		t.Errorf("expected summary 'Third session - latest', got %q", latest.Summary)
	}
	if latest.Harness != "antigravity" {
		t.Errorf("expected harness 'antigravity', got %q", latest.Harness)
	}
}

func TestStoreSecurityAndAgentRulePreservation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-security-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := Init(tmpDir, "Security Test", "Testing security checks")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Path traversal attempts on GetTask and GetGoal
	invalidIDs := []string{"../../etc/passwd", "task/sub", "task..name", "../config"}
	for _, badID := range invalidIDs {
		if _, err := s.GetTask(badID); err == nil {
			t.Errorf("expected GetTask(%q) to fail with validation/traversal error, got nil", badID)
		}
		if _, err := s.GetGoal(badID); err == nil {
			t.Errorf("expected GetGoal(%q) to fail with validation/traversal error, got nil", badID)
		}
	}

	// 2. Existing agent rules should not be clobbered by InstallAgentRules
	existingClaude := "# Custom User Claude Instructions\n- Do not run tests on Friday\n"
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte(existingClaude), 0644); err != nil {
		t.Fatal(err)
	}

	if err := s.InstallAgentRules(); err != nil {
		t.Fatalf("InstallAgentRules failed: %v", err)
	}

	data, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "Do not run tests on Friday") {
		t.Errorf("pre-existing instructions were destroyed! content: %s", content)
	}
	if !strings.Contains(content, "keel brief") {
		t.Errorf("keel brief section was not appended! content: %s", content)
	}
}

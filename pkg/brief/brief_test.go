package brief

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"keel/pkg/model"
	"keel/pkg/store"
)

func TestBriefFormattingAndTruncation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-brief-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := store.Init(tmpDir, "Brief Test", "Testing brief truncation")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Empty brief test
	b, err := Generate(s)
	if err != nil {
		t.Fatalf("Generate on empty failed: %v", err)
	}
	formatted := b.FormatTerminal()
	if !strings.Contains(formatted, "No ready tasks") {
		t.Errorf("expected empty warning, got:\n%s", formatted)
	}

	// 2. Goal creation
	goal := model.Goal{
		ID:       "goal-1",
		Title:    "Primary Goal",
		Outcome:  "Test Outcome",
		Status:   "active",
		DoneWhen: []string{"Item 1", "Item 2"},
	}
	if err := s.CreateGoal(goal); err != nil {
		t.Fatal(err)
	}

	// 3. Create 7 ready tasks (should truncate at 5)
	for i := 1; i <= 7; i++ {
		task := model.Task{
			ID:       fmt.Sprintf("task-%d", i),
			Title:    fmt.Sprintf("Task %d Title", i),
			Status:   "pending",
			Priority: "high",
		}
		if err := s.CreateTask(task); err != nil {
			t.Fatal(err)
		}
	}

	b, err = Generate(s)
	if err != nil {
		t.Fatal(err)
	}

	formatted = b.FormatTerminal()
	if !strings.Contains(formatted, "more ready tasks (run 'keel task ready' for full list)") {
		t.Errorf("expected truncation message in brief, got:\n%s", formatted)
	}

	// Verify JSON format
	jsonStr, err := b.FormatJSON()
	if err != nil || !strings.Contains(jsonStr, `"projectName": "Brief Test"`) {
		t.Errorf("invalid json output: %s", jsonStr)
	}
}

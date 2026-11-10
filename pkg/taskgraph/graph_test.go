package taskgraph

import (
	"testing"

	"keel/pkg/model"
)

func TestEvaluateReadiness(t *testing.T) {
	tasks := []model.Task{
		{ID: "task-1", Title: "DB Schema", Status: "completed", Priority: "high"},
		{ID: "task-2", Title: "API Backend", Status: "pending", DependsOn: []string{"task-1"}, Priority: "critical"},
		{ID: "task-3", Title: "Frontend UI", Status: "pending", DependsOn: []string{"task-2"}, Priority: "medium"},
	}

	result, err := Evaluate(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Completed) != 1 || result.Completed[0].ID != "task-1" {
		t.Errorf("expected task-1 in completed, got %v", result.Completed)
	}

	if len(result.Ready) != 1 || result.Ready[0].Task.ID != "task-2" {
		t.Errorf("expected task-2 in ready, got %v", result.Ready)
	}

	if len(result.Blocked) != 1 || result.Blocked[0].Task.ID != "task-3" {
		t.Errorf("expected task-3 in blocked, got %v", result.Blocked)
	}
}

func TestEvaluateCycleDetection(t *testing.T) {
	tasks := []model.Task{
		{ID: "task-a", Title: "A", Status: "pending", DependsOn: []string{"task-b"}},
		{ID: "task-b", Title: "B", Status: "pending", DependsOn: []string{"task-a"}},
	}

	_, err := Evaluate(tasks)
	if err == nil {
		t.Fatal("expected dependency cycle error, got nil")
	}
}

func TestPriorityScoreNormalization(t *testing.T) {
	cases := []struct {
		input    string
		expected int
	}{
		{"critical", 4},
		{"CRITICAL", 4},
		{" Critical ", 4},
		{"high", 3},
		{"HIGH", 3},
		{" High ", 3},
		{"medium", 2},
		{"Medium", 2},
		{"low", 1},
		{"LOW", 1},
		{"unknown", 0},
		{"", 0},
	}

	for _, tc := range cases {
		score := priorityScore(tc.input)
		if score != tc.expected {
			t.Errorf("priorityScore(%q) = %d, expected %d", tc.input, score, tc.expected)
		}
	}
}

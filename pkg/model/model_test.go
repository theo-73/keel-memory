package model

import "testing"

func TestValidateIDValidation(t *testing.T) {
	validIDs := []string{"goal-1", "task_api", "v1.0.0", "task123"}
	for _, id := range validIDs {
		if err := ValidateID(id); err != nil {
			t.Errorf("expected %q to be valid, got: %v", id, err)
		}
	}

	invalidIDs := []string{"", "Task-1", "has space", "-leading", "invalid@char!", "a/b"}
	for _, id := range invalidIDs {
		if err := ValidateID(id); err == nil {
			t.Errorf("expected %q to be invalid, got nil", id)
		}
	}
}

func TestValidateGoalStatus(t *testing.T) {
	validGoal := Goal{
		ID:     "valid-goal",
		Title:  "Title",
		Status: "active",
	}
	if err := ValidateGoal(validGoal); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	invalidGoal := Goal{
		ID:     "valid-goal",
		Title:  "Title",
		Status: "invalid_status",
	}
	if err := ValidateGoal(invalidGoal); err == nil {
		t.Error("expected error for invalid status, got nil")
	}
}

func TestValidateTaskSelfDependency(t *testing.T) {
	task := Task{
		ID:        "task-1",
		Title:     "Self Dependent",
		Status:    "pending",
		DependsOn: []string{"task-1"},
	}
	if err := ValidateTask(task); err == nil {
		t.Error("expected self-dependency error, got nil")
	}
}

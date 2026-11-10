package model

import (
	"fmt"
	"strings"
	"time"
)

// Task represents a discrete, actionable work item.
type Task struct {
	SchemaVersion string    `json:"schemaVersion"`
	ID            string    `json:"id"`
	GoalID        string    `json:"goalId,omitempty"`
	Title         string    `json:"title"`
	Description   string    `json:"description,omitempty"`
	Priority      string    `json:"priority,omitempty"` // "low", "medium", "high", "critical"
	Status        string    `json:"status"`             // "pending", "ready", "in_progress", "completed", "blocked", "cancelled"
	DependsOn     []string  `json:"dependsOn,omitempty"`
	Assignee      string    `json:"assignee,omitempty"`
	BlockedReason string    `json:"blockedReason,omitempty"`
	Result        string    `json:"result,omitempty"`
	Deliverables  []string  `json:"deliverables,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func ValidateTask(t Task) error {
	if err := ValidateID(t.ID); err != nil {
		return fmt.Errorf("task id: %w", err)
	}
	if strings.TrimSpace(t.Title) == "" {
		return fmt.Errorf("task title is required")
	}
	if t.GoalID != "" {
		if err := ValidateID(t.GoalID); err != nil {
			return fmt.Errorf("task goalId: %w", err)
		}
	}
	switch t.Status {
	case "pending", "ready", "in_progress", "completed", "blocked", "cancelled":
		// valid
	case "":
		return fmt.Errorf("task status is required")
	default:
		return fmt.Errorf("invalid task status %q", t.Status)
	}
	if t.Priority != "" {
		switch strings.ToLower(strings.TrimSpace(t.Priority)) {
		case "low", "medium", "high", "critical":
			// valid
		default:
			return fmt.Errorf("invalid task priority %q (allowed: low, medium, high, critical)", t.Priority)
		}
	}
	for _, dep := range t.DependsOn {
		if err := ValidateID(dep); err != nil {
			return fmt.Errorf("task dependsOn %q: %w", dep, err)
		}
		if dep == t.ID {
			return fmt.Errorf("task cannot depend on itself")
		}
	}
	return nil
}

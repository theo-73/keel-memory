package model

import (
	"fmt"
	"strings"
	"time"
)

// Goal represents an overarching milestone or target outcome.
type Goal struct {
	SchemaVersion string    `json:"schemaVersion"`
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Outcome       string    `json:"outcome"`
	Status        string    `json:"status"` // "active", "completed", "blocked", "paused"
	DoneWhen      []string  `json:"doneWhen,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func ValidateGoal(g Goal) error {
	if err := ValidateID(g.ID); err != nil {
		return fmt.Errorf("goal id: %w", err)
	}
	if strings.TrimSpace(g.Title) == "" {
		return fmt.Errorf("goal title is required")
	}
	switch g.Status {
	case "active", "completed", "blocked", "paused":
		// valid
	case "":
		return fmt.Errorf("goal status is required")
	default:
		return fmt.Errorf("invalid goal status %q (allowed: active, completed, blocked, paused)", g.Status)
	}
	return nil
}

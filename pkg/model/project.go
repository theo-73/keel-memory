package model

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var idRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// ValidateID ensures all project identifiers (goals, tasks, repos) follow a clean, URL-safe slug pattern.
func ValidateID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("identifier cannot be empty")
	}
	if !idRegex.MatchString(id) {
		return fmt.Errorf("invalid identifier %q: must contain only lowercase letters, numbers, hyphens, dots, or underscores (up to 64 chars)", id)
	}
	return nil
}

// ProjectManifest defines the immutable or root metadata of a Keel project.
type ProjectManifest struct {
	SchemaVersion string    `json:"schemaVersion"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// ProjectState is the materialized control-plane state reflecting current progress and next actions.
type ProjectState struct {
	SchemaVersion  string    `json:"schemaVersion"`
	UpdatedAt      time.Time `json:"updatedAt"`
	CurrentGoalID  string    `json:"currentGoalId,omitempty"`
	CurrentPhase   string    `json:"currentPhase"`
	Health         string    `json:"health"` // "on_track", "at_risk", "blocked", "completed"
	Progress       string    `json:"progress"`
	ActiveTaskIDs  []string  `json:"activeTaskIds,omitempty"`
	BlockedTaskIDs []string  `json:"blockedTaskIds,omitempty"`
	NextActions    []string  `json:"nextActions"`
	Risks          []string  `json:"risks,omitempty"`
}

// DefaultState creates an initial healthy project state.
func DefaultState(name string) ProjectState {
	return ProjectState{
		SchemaVersion: "1.0.0",
		UpdatedAt:     time.Now().UTC(),
		CurrentPhase:  "Initialization",
		Health:        "on_track",
		Progress:      "Project initialized with Keel agent memory.",
		NextActions:   []string{"Define your primary project goal with 'keel goal create'"},
		Risks:         []string{},
	}
}

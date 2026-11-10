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
	if strings.HasSuffix(id, ".json") {
		return fmt.Errorf("identifier %q must not end with .json extension", id)
	}
	return nil
}

// Slugify converts an arbitrary title or string into a valid, URL-safe identifier.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			if b.Len() > 0 && b.String()[b.Len()-1] != '-' {
				b.WriteByte('-')
			}
		}
	}
	res := strings.Trim(b.String(), "-")
	if len(res) > 64 {
		res = res[:64]
		res = strings.TrimRight(res, "-")
	}
	if res == "" {
		res = "item"
	}
	return res
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

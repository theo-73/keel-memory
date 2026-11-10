package model

import (
	"time"
)

// WorklogEntry records an agent session's activity, findings, and unfinished work.
type WorklogEntry struct {
	Harness   string    `json:"harness"`
	Summary   string    `json:"summary"`
	NextSteps []string  `json:"nextSteps,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

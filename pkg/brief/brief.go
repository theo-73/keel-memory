package brief

import (
	"encoding/json"
	"fmt"
	"strings"

	"keel/pkg/model"
	"keel/pkg/store"
	"keel/pkg/taskgraph"
)

// Brief is a token-compressed, high-density project overview designed for agent session start.
type Brief struct {
	ProjectName   string                  `json:"projectName"`
	Root          string                  `json:"root"`
	CurrentPhase  string                  `json:"currentPhase"`
	Health        string                  `json:"health"`
	Progress      string                  `json:"progress"`
	ActiveGoal    *model.Goal             `json:"activeGoal,omitempty"`
	NextActions   []string                `json:"nextActions"`
	ReadyTasks    []taskgraph.TaskEvaluation `json:"readyTasks"`
	InProgress    []model.Task            `json:"inProgress"`
	BlockedTasks  []taskgraph.TaskEvaluation `json:"blockedTasks"`
	LatestWorklog *model.WorklogEntry     `json:"latestWorklog,omitempty"`
}

const (
	MaxBriefReadyTasks   = 5
	MaxBriefBlockedTasks = 3
)

// Generate creates a concise brief from the project store.
func Generate(s *store.Store) (*Brief, error) {
	manifest, err := s.Manifest()
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	state, err := s.State()
	if err != nil {
		state = model.DefaultState(manifest.Name)
	}

	goals, err := s.ListGoals()
	if err != nil {
		return nil, fmt.Errorf("list goals: %w", err)
	}

	tasks, err := s.ListTasks()
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	eval, err := taskgraph.Evaluate(tasks)
	if err != nil {
		return nil, fmt.Errorf("evaluate tasks: %w", err)
	}

	var activeGoal *model.Goal
	for i := range goals {
		if goals[i].ID == state.CurrentGoalID || (state.CurrentGoalID == "" && goals[i].Status == "active") {
			activeGoal = &goals[i]
			break
		}
	}

	latestLog, _ := s.LatestWorklog()

	b := &Brief{
		ProjectName:   manifest.Name,
		Root:          s.Root,
		CurrentPhase:  state.CurrentPhase,
		Health:        state.Health,
		Progress:      state.Progress,
		ActiveGoal:    activeGoal,
		NextActions:   state.NextActions,
		ReadyTasks:    eval.Ready,
		InProgress:    eval.InProgress,
		BlockedTasks:  eval.Blocked,
		LatestWorklog: latestLog,
	}

	return b, nil
}

// FormatTerminal returns an aesthetically pleasing, dense terminal / Markdown representation.
func (b *Brief) FormatTerminal() string {
	var sb strings.Builder

	healthIcon := "●"
	switch b.Health {
	case "on_track":
		healthIcon = "● [On Track]"
	case "at_risk":
		healthIcon = "▲ [At Risk]"
	case "blocked":
		healthIcon = "✖ [Blocked]"
	case "completed":
		healthIcon = "✔ [Completed]"
	default:
		healthIcon = "○ [" + b.Health + "]"
	}

	sb.WriteString(fmt.Sprintf("# Keel Project Brief: %s\n", b.ProjectName))
	sb.WriteString(fmt.Sprintf("**Status:** %s | **Phase:** %s\n", healthIcon, b.CurrentPhase))
	if b.Progress != "" {
		sb.WriteString(fmt.Sprintf("**Progress:** %s\n", b.Progress))
	}
	sb.WriteString("\n")

	// Goal
	if b.ActiveGoal != nil {
		sb.WriteString(fmt.Sprintf("## Current Goal: %s\n", b.ActiveGoal.Title))
		if b.ActiveGoal.Outcome != "" {
			sb.WriteString(fmt.Sprintf("> %s\n\n", b.ActiveGoal.Outcome))
		}
		if len(b.ActiveGoal.DoneWhen) > 0 {
			sb.WriteString("**Done When:**\n")
			for _, cond := range b.ActiveGoal.DoneWhen {
				sb.WriteString(fmt.Sprintf("- [ ] %s\n", cond))
			}
			sb.WriteString("\n")
		}
	}

	// Ready Tasks (Token budgeted)
	sb.WriteString("## Actionable Work (Ready Tasks)\n")
	readyCount := len(b.ReadyTasks)
	if readyCount == 0 {
		sb.WriteString("_No ready tasks. Check backlog or create new tasks with `keel task create`._\n\n")
	} else {
		displayReady := b.ReadyTasks
		if readyCount > MaxBriefReadyTasks {
			displayReady = b.ReadyTasks[:MaxBriefReadyTasks]
		}
		for _, r := range displayReady {
			prio := ""
			if r.Task.Priority != "" {
				prio = fmt.Sprintf(" [%s]", strings.ToUpper(r.Task.Priority))
			}
			sb.WriteString(fmt.Sprintf("- **`%s`**%s: %s\n", r.Task.ID, prio, r.Task.Title))
			if r.Task.Description != "" {
				sb.WriteString(fmt.Sprintf("  _%s_\n", r.Task.Description))
			}
		}
		if readyCount > MaxBriefReadyTasks {
			sb.WriteString(fmt.Sprintf("  _...and %d more ready tasks (run 'keel task ready' for full list)_\n", readyCount-MaxBriefReadyTasks))
		}
		sb.WriteString("\n")
	}

	// In Progress
	if len(b.InProgress) > 0 {
		sb.WriteString("## In-Progress Tasks\n")
		for _, t := range b.InProgress {
			assignee := ""
			if t.Assignee != "" {
				assignee = fmt.Sprintf(" (assigned to %s)", t.Assignee)
			}
			sb.WriteString(fmt.Sprintf("- ⏳ **`%s`**: %s%s\n", t.ID, t.Title, assignee))
		}
		sb.WriteString("\n")
	}

	// Blocked Tasks (Token budgeted)
	blockedCount := len(b.BlockedTasks)
	if blockedCount > 0 {
		sb.WriteString("## Blocked Tasks\n")
		displayBlocked := b.BlockedTasks
		if blockedCount > MaxBriefBlockedTasks {
			displayBlocked = b.BlockedTasks[:MaxBriefBlockedTasks]
		}
		for _, blk := range displayBlocked {
			reasons := strings.Join(blk.Blockers, ", ")
			sb.WriteString(fmt.Sprintf("- 🚫 **`%s`**: %s _(Blocked by: %s)_\n", blk.Task.ID, blk.Task.Title, reasons))
		}
		if blockedCount > MaxBriefBlockedTasks {
			sb.WriteString(fmt.Sprintf("  _...and %d more blocked tasks_\n", blockedCount-MaxBriefBlockedTasks))
		}
		sb.WriteString("\n")
	}

	// Next Actions
	if len(b.NextActions) > 0 {
		sb.WriteString("## Recommended Next Steps\n")
		for i, act := range b.NextActions {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, act))
		}
		sb.WriteString("\n")
	}

	// Recent Worklog
	if b.LatestWorklog != nil {
		sb.WriteString(fmt.Sprintf("## Previous Session Notes (%s by %s)\n", b.LatestWorklog.CreatedAt.Format("Jan 02 15:04"), b.LatestWorklog.Harness))
		sb.WriteString(fmt.Sprintf("> %s\n", b.LatestWorklog.Summary))
		if len(b.LatestWorklog.NextSteps) > 0 {
			sb.WriteString("**Left for this session:**\n")
			for _, step := range b.LatestWorklog.NextSteps {
				sb.WriteString(fmt.Sprintf("- %s\n", step))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (b *Brief) FormatJSON() (string, error) {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

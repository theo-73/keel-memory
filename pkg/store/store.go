package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"keel/pkg/lock"
	"keel/pkg/model"
	"keel/pkg/platform"
	"keel/pkg/taskgraph"
)

const ManifestFilename = "keel.json"

type Store struct {
	Root   string
	locker *lock.Locker
	mu     sync.Mutex
}

func (s *Store) withLock(key string, fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.locker == nil {
		s.locker = lock.New(s.Root)
	}

	release, err := s.locker.Acquire(key)
	if err != nil {
		return fmt.Errorf("acquire lock %s: %w", key, err)
	}
	defer release()

	return fn()
}

// FindRoot searches from current dir upward for keel.json.
func FindRoot(start string) (string, error) {
	curr, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(curr, ManifestFilename)); err == nil {
			return curr, nil
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return "", fmt.Errorf("no Keel project found (searched upward from %s)", start)
}

// Open opens an existing Keel project at root.
func Open(root string) (*Store, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return nil, err
	}
	return &Store{
		Root:   resolved,
		locker: lock.New(resolved),
	}, nil
}

// Init initializes a new Keel project structure.
func Init(root, name, description string) (*Store, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(absRoot)
	}

	manifestPath := filepath.Join(absRoot, ManifestFilename)
	if _, err := os.Stat(manifestPath); err == nil {
		return nil, fmt.Errorf("a Keel project already exists at %s", absRoot)
	}

	manifest := model.ProjectManifest{
		SchemaVersion: "1.0.0",
		Name:          name,
		Description:   description,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	// Create directories
	dirs := []string{"goals", "tasks", "state", "worklogs", ".keel/locks"}
	for _, d := range dirs {
		if err := platform.EnsureDir(filepath.Join(absRoot, d)); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", d, err)
		}
	}

	if err := platform.AtomicWriteJSON(manifestPath, manifest); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	// Create default state
	state := model.DefaultState(name)
	statePath := filepath.Join(absRoot, "state", "state.json")
	if err := platform.AtomicWriteJSON(statePath, state); err != nil {
		return nil, fmt.Errorf("write state: %w", err)
	}

	s := &Store{
		Root:   absRoot,
		locker: lock.New(absRoot),
	}

	// Install agent instructions & configs
	if err := s.InstallAgentRules(); err != nil {
		return nil, fmt.Errorf("install agent rules: %w", err)
	}

	return s, nil
}

func (s *Store) Manifest() (model.ProjectManifest, error) {
	return platform.ReadJSON[model.ProjectManifest](filepath.Join(s.Root, ManifestFilename))
}

func (s *Store) State() (model.ProjectState, error) {
	statePath := filepath.Join(s.Root, "state", "state.json")
	return platform.ReadJSON[model.ProjectState](statePath)
}

func (s *Store) SaveState(st model.ProjectState) error {
	st.UpdatedAt = time.Now().UTC()
	return platform.AtomicWriteJSON(filepath.Join(s.Root, "state", "state.json"), st)
}

// Goals
func (s *Store) CreateGoal(g model.Goal) error {
	if err := model.ValidateGoal(g); err != nil {
		return err
	}
	return s.withLock("project:store", func() error {
		path := filepath.Join(s.Root, "goals", g.ID+".json")
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("goal %q already exists", g.ID)
		}
		g.CreatedAt = time.Now().UTC()
		g.UpdatedAt = g.CreatedAt
		g.SchemaVersion = "1.0.0"
		if err := platform.AtomicWriteJSON(path, g); err != nil {
			return err
		}

		// Advance state and select goal if none active
		state, err := s.State()
		if err == nil {
			if state.CurrentGoalID == "" {
				state.CurrentGoalID = g.ID
			}
			state.UpdatedAt = time.Now().UTC()
			_ = s.SaveState(state)
		}
		return nil
	})
}

func (s *Store) GetGoal(id string) (model.Goal, error) {
	path := filepath.Join(s.Root, "goals", id+".json")
	return platform.ReadJSON[model.Goal](path)
}

func (s *Store) ListGoals() ([]model.Goal, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, "goals"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []model.Goal{}, nil
		}
		return nil, err
	}
	goals := make([]model.Goal, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		g, err := platform.ReadJSON[model.Goal](filepath.Join(s.Root, "goals", e.Name()))
		if err == nil {
			goals = append(goals, g)
		}
	}
	sort.Slice(goals, func(i, j int) bool { return goals[i].ID < goals[j].ID })
	return goals, nil
}

func (s *Store) UpdateGoal(g model.Goal) error {
	if err := model.ValidateGoal(g); err != nil {
		return err
	}
	return s.withLock("project:store", func() error {
		path := filepath.Join(s.Root, "goals", g.ID+".json")
		existing, err := s.GetGoal(g.ID)
		if err != nil {
			return fmt.Errorf("goal %q not found: %w", g.ID, err)
		}
		g.CreatedAt = existing.CreatedAt
		g.UpdatedAt = time.Now().UTC()
		g.SchemaVersion = "1.0.0"
		if err := platform.AtomicWriteJSON(path, g); err != nil {
			return err
		}

		state, err := s.State()
		if err == nil {
			state.UpdatedAt = time.Now().UTC()
			_ = s.SaveState(state)
		}
		return nil
	})
}

func (s *Store) DeleteGoal(id string) error {
	if err := model.ValidateID(id); err != nil {
		return err
	}
	return s.withLock("project:store", func() error {
		path := filepath.Join(s.Root, "goals", id+".json")
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("goal %q not found: %w", id, err)
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("delete goal %s: %w", id, err)
		}

		state, err := s.State()
		if err == nil {
			if state.CurrentGoalID == id {
				state.CurrentGoalID = ""
			}
			state.UpdatedAt = time.Now().UTC()
			_ = s.SaveState(state)
		}
		return nil
	})
}

func (s *Store) SetCurrentGoal(id string) error {
	if _, err := s.GetGoal(id); err != nil {
		return fmt.Errorf("goal %q not found: %w", id, err)
	}
	return s.withLock("project:store", func() error {
		state, err := s.State()
		if err != nil {
			return err
		}
		state.CurrentGoalID = id
		state.UpdatedAt = time.Now().UTC()
		return s.SaveState(state)
	})
}

// Tasks
func (s *Store) CreateTask(t model.Task) error {
	if err := model.ValidateTask(t); err != nil {
		return err
	}
	return s.withLock("project:store", func() error {
		path := filepath.Join(s.Root, "tasks", t.ID+".json")
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("task %q already exists", t.ID)
		}
		t.CreatedAt = time.Now().UTC()
		t.UpdatedAt = t.CreatedAt
		t.SchemaVersion = "1.0.0"
		if err := platform.AtomicWriteJSON(path, t); err != nil {
			return err
		}
		return s.reconcileStateLocked()
	})
}

func (s *Store) GetTask(id string) (model.Task, error) {
	path := filepath.Join(s.Root, "tasks", id+".json")
	return platform.ReadJSON[model.Task](path)
}

func (s *Store) ListTasks() ([]model.Task, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, "tasks"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []model.Task{}, nil
		}
		return nil, err
	}
	tasks := make([]model.Task, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		t, err := platform.ReadJSON[model.Task](filepath.Join(s.Root, "tasks", e.Name()))
		if err == nil {
			tasks = append(tasks, t)
		}
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks, nil
}

func (s *Store) UpdateTask(t model.Task) error {
	if err := model.ValidateTask(t); err != nil {
		return err
	}
	return s.withLock("project:store", func() error {
		path := filepath.Join(s.Root, "tasks", t.ID+".json")
		existing, err := s.GetTask(t.ID)
		if err != nil {
			return fmt.Errorf("task %q not found: %w", t.ID, err)
		}
		t.CreatedAt = existing.CreatedAt
		t.UpdatedAt = time.Now().UTC()
		t.SchemaVersion = "1.0.0"
		if err := platform.AtomicWriteJSON(path, t); err != nil {
			return err
		}
		return s.reconcileStateLocked()
	})
}

func (s *Store) DeleteTask(id string) error {
	if err := model.ValidateID(id); err != nil {
		return err
	}
	return s.withLock("project:store", func() error {
		path := filepath.Join(s.Root, "tasks", id+".json")
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("task %q not found: %w", id, err)
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("delete task %s: %w", id, err)
		}
		return s.reconcileStateLocked()
	})
}

// ReconcileState re-evaluates the task graph and synchronizes active/blocked tasks and project health.
func (s *Store) ReconcileState() error {
	return s.withLock("project:store", func() error {
		return s.reconcileStateLocked()
	})
}

func (s *Store) reconcileStateLocked() error {
	state, err := s.State()
	if err != nil {
		manifest, mErr := s.Manifest()
		if mErr != nil {
			return err
		}
		state = model.DefaultState(manifest.Name)
	}

	tasks, err := s.ListTasks()
	if err != nil {
		return err
	}

	eval, err := taskgraph.Evaluate(tasks)
	if err != nil {
		state.Health = "at_risk"
		state.Risks = []string{fmt.Sprintf("Task graph error: %v", err)}
		return s.SaveState(state)
	}

	activeIDs := make([]string, 0, len(eval.InProgress))
	for _, t := range eval.InProgress {
		activeIDs = append(activeIDs, t.ID)
	}
	blockedIDs := make([]string, 0, len(eval.Blocked))
	for _, b := range eval.Blocked {
		blockedIDs = append(blockedIDs, b.Task.ID)
	}

	state.ActiveTaskIDs = activeIDs
	state.BlockedTaskIDs = blockedIDs

	activeTotal := 0
	for _, t := range tasks {
		if t.Status != "cancelled" {
			activeTotal++
		}
	}
	done := len(eval.Completed)

	// Check if any blocked task has an explicit BlockedReason or a missing dependency reference
	hasExplicitBlocker := false
	for _, b := range eval.Blocked {
		if b.Task.BlockedReason != "" {
			hasExplicitBlocker = true
			break
		}
		for _, dep := range b.Blockers {
			if strings.Contains(dep, "(missing)") {
				hasExplicitBlocker = true
				break
			}
		}
		if hasExplicitBlocker {
			break
		}
	}

	// Health & Phase Transitions
	if activeTotal > 0 && done == activeTotal {
		state.Health = "completed"
		state.CurrentPhase = "Verification & Handoff"
	} else if len(blockedIDs) > 0 && len(activeIDs) == 0 && len(eval.Ready) == 0 {
		state.Health = "blocked"
	} else if hasExplicitBlocker {
		state.Health = "at_risk"
	} else {
		state.Health = "on_track"
		if done > 0 {
			state.CurrentPhase = "Implementation"
		}
	}

	if activeTotal > 0 {
		state.Progress = fmt.Sprintf("%d of %d tasks completed (%d%%)", done, activeTotal, done*100/activeTotal)
	} else if len(tasks) > 0 {
		state.Progress = "All tasks cancelled."
	} else {
		state.Progress = "No tasks created yet."
	}

	// Next Actions derivation
	if len(eval.Ready) > 0 {
		actions := make([]string, 0, 3)
		for _, r := range eval.Ready {
			actions = append(actions, fmt.Sprintf("Execute task %s: %s", r.Task.ID, r.Task.Title))
			if len(actions) >= 3 {
				break
			}
		}
		state.NextActions = actions
	} else if activeTotal > 0 && done == activeTotal {
		state.NextActions = []string{"All planned tasks completed. Verify deliverables and record final worklog."}
	} else if len(blockedIDs) > 0 && len(activeIDs) == 0 {
		state.NextActions = []string{"Resolve task dependency blockers or add unblocked tasks."}
	} else {
		state.NextActions = []string{"Define new tasks with 'keel task create'"}
	}

	return s.SaveState(state)
}

func sanitizeHarnessSlug(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "agent"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	res := strings.Trim(b.String(), "-")
	if res == "" {
		return "agent"
	}
	return res
}

// Worklogs
func (s *Store) RecordWorklog(w model.WorklogEntry) error {
	return s.withLock("project:store", func() error {
		w.CreatedAt = time.Now().UTC()
		harnessSlug := sanitizeHarnessSlug(w.Harness)
		filename := fmt.Sprintf("%s_%09d_%s.json", w.CreatedAt.Format("20060102_150405"), w.CreatedAt.Nanosecond(), harnessSlug)
		path := filepath.Join(s.Root, "worklogs", filename)
		for i := 1; ; i++ {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				break
			}
			filename = fmt.Sprintf("%s_%09d_%d_%s.json", w.CreatedAt.Format("20060102_150405"), w.CreatedAt.Nanosecond(), i, harnessSlug)
			path = filepath.Join(s.Root, "worklogs", filename)
		}
		if err := platform.AtomicWriteJSON(path, w); err != nil {
			return err
		}

		// Real-time reactivity trigger: touch state so SSE broadcaster picks it up
		state, err := s.State()
		if err == nil {
			state.UpdatedAt = time.Now().UTC()
			_ = s.SaveState(state)
		}
		return nil
	})
}

func (s *Store) ListWorklogs() ([]model.WorklogEntry, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, "worklogs"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []model.WorklogEntry{}, nil
		}
		return nil, err
	}
	logs := make([]model.WorklogEntry, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		w, err := platform.ReadJSON[model.WorklogEntry](filepath.Join(s.Root, "worklogs", e.Name()))
		if err == nil {
			logs = append(logs, w)
		}
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].CreatedAt.After(logs[j].CreatedAt) })
	return logs, nil
}

func (s *Store) LatestWorklog() (*model.WorklogEntry, error) {
	logs, err := s.ListWorklogs()
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return nil, nil
	}
	return &logs[0], nil
}

// InstallAgentRules configures instructions for Claude Code, Cursor, Codex, and Antigravity.
func (s *Store) InstallAgentRules() error {
	agentsMD := `# Keel Project Agent Guide

This repository uses **Keel** for persistent agent memory, goal tracking, and cross-session handoffs.

## Session Start
At the beginning of every session, run:
` + "```bash" + `
keel brief
` + "```" + `
or invoke the native MCP tool ` + "`keel_brief`" + `.

## Working on Tasks
1. Inspect available ready tasks: ` + "`keel task ready`" + `
2. Pick a task: ` + "`keel task update <id> --status in_progress`" + `
3. Deliver results & verify against requirements.
4. Mark complete: ` + "`keel task update <id> --status completed --result \"What was done\"`" + `

## Session Checkpoint (Before Leaving)
Always leave notes for the next agent:
` + "```bash" + `
keel worklog "Summary of work done" --next "Immediate next task"
` + "```" + `
`
	if err := os.WriteFile(filepath.Join(s.Root, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		return err
	}

	claudeMD := `# Claude Code Instructions
Always start your work by checking project context:
- Run 'keel brief' to get current goals, ready tasks, and recent session notes.
- Mark tasks completed using 'keel task update <id> --status completed'.
- Before ending your turn, record a worklog using 'keel worklog "<summary>"'.
`
	if err := os.WriteFile(filepath.Join(s.Root, "CLAUDE.md"), []byte(claudeMD), 0644); err != nil {
		return err
	}

	cursorRule := `---
description: Keel Agent Memory System
globs: *
---
# Keel Project Integration
- Always run 'keel brief' at the start of a conversation to review project goals and tasks.
- Keep tasks up to date via 'keel task update'.
- Write worklogs before session finish via 'keel worklog'.
`
	cursorDir := filepath.Join(s.Root, ".cursor", "rules")
	if err := platform.EnsureDir(cursorDir); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cursorDir, "keel.mdc"), []byte(cursorRule), 0644); err != nil {
		return err
	}

	windsurfRules := `# Windsurf Keel Integration
- Start sessions by running 'keel brief' or reading MCP 'keel_brief'.
- Query ready tasks with 'keel task ready' and maintain accurate task states.
- Leave session handoffs before exit via 'keel worklog "<summary>" --next "<next-steps>"'.
`
	if err := os.WriteFile(filepath.Join(s.Root, ".windsurfrules"), []byte(windsurfRules), 0644); err != nil {
		return err
	}

	copilotDir := filepath.Join(s.Root, ".github")
	if err := platform.EnsureDir(copilotDir); err != nil {
		return err
	}
	copilotMD := `# Copilot Coding Instructions
This repository utilizes Keel for persistent agent memory, goal alignment, and session handoffs.
- Project Orientation: Run 'keel brief' to retrieve current goals, active tasks, and recent handoff notes.
- Task Lifecycle: Update task status using 'keel task update <id> --status completed'.
- Checkpoints: Record worklogs before session completion via 'keel worklog "<summary>" --next "<next-steps>"'.
`
	return os.WriteFile(filepath.Join(copilotDir, "copilot-instructions.md"), []byte(copilotMD), 0644)
}

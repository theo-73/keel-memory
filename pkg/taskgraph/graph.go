package taskgraph

import (
	"fmt"
	"sort"
	"strings"

	"keel/pkg/model"
)

// TaskEvaluation represents a evaluated task in the project graph.
type TaskEvaluation struct {
	Task     model.Task `json:"task"`
	IsReady  bool       `json:"isReady"`
	Blockers []string   `json:"blockers,omitempty"` // IDs of prerequisite tasks not yet completed
}

// GraphResult categorizes tasks into actionable buckets.
type GraphResult struct {
	Ready      []TaskEvaluation `json:"ready"`
	InProgress []model.Task     `json:"inProgress"`
	Blocked    []TaskEvaluation `json:"blocked"`
	Completed  []model.Task     `json:"completed"`
}

// Evaluate processes a slice of tasks and categorizes them by readiness.
func Evaluate(tasks []model.Task) (GraphResult, error) {
	taskMap := make(map[string]model.Task, len(tasks))
	for _, t := range tasks {
		taskMap[t.ID] = t
	}

	// Check for cycles
	if err := detectCycles(tasks, taskMap); err != nil {
		return GraphResult{}, err
	}

	res := GraphResult{
		Ready:      make([]TaskEvaluation, 0),
		InProgress: make([]model.Task, 0),
		Blocked:    make([]TaskEvaluation, 0),
		Completed:  make([]model.Task, 0),
	}

	for _, t := range tasks {
		switch t.Status {
		case "completed":
			res.Completed = append(res.Completed, t)
		case "in_progress":
			res.InProgress = append(res.InProgress, t)
			if t.BlockedReason != "" {
				res.Blocked = append(res.Blocked, TaskEvaluation{
					Task:     t,
					IsReady:  false,
					Blockers: []string{"In-progress stalled: " + t.BlockedReason},
				})
			}
		case "cancelled":
			// Skip cancelled from ready/blocked queues
		default:
			// "pending", "ready", "blocked"
			blockers := make([]string, 0)
			for _, depID := range t.DependsOn {
				dep, exists := taskMap[depID]
				if !exists {
					blockers = append(blockers, fmt.Sprintf("%s (missing)", depID))
				} else if dep.Status != "completed" {
					blockers = append(blockers, fmt.Sprintf("%s (%s)", dep.ID, dep.Status))
				}
			}

			if t.BlockedReason != "" {
				blockers = append(blockers, t.BlockedReason)
			}

			eval := TaskEvaluation{
				Task:     t,
				IsReady:  len(blockers) == 0,
				Blockers: blockers,
			}

			if eval.IsReady {
				res.Ready = append(res.Ready, eval)
			} else {
				res.Blocked = append(res.Blocked, eval)
			}
		}
	}

	// Sort ready tasks by priority then ID (deterministic)
	sort.SliceStable(res.Ready, func(i, j int) bool {
		pI := priorityScore(res.Ready[i].Task.Priority)
		pJ := priorityScore(res.Ready[j].Task.Priority)
		if pI != pJ {
			return pI > pJ
		}
		return res.Ready[i].Task.ID < res.Ready[j].Task.ID
	})

	return res, nil
}

func priorityScore(p string) int {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func detectCycles(tasks []model.Task, taskMap map[string]model.Task) error {
	visited := make(map[string]int) // 0: unvisited, 1: visiting, 2: visited

	var dfs func(id string) error
	dfs = func(id string) error {
		state := visited[id]
		if state == 1 {
			return fmt.Errorf("dependency cycle detected involving task %q", id)
		}
		if state == 2 {
			return nil
		}

		visited[id] = 1
		if t, ok := taskMap[id]; ok {
			for _, depID := range t.DependsOn {
				if err := dfs(depID); err != nil {
					return err
				}
			}
		}
		visited[id] = 2
		return nil
	}

	for _, t := range tasks {
		if visited[t.ID] == 0 {
			if err := dfs(t.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

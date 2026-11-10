package cli

import (
	"flag"
	"fmt"
	"strings"

	"keel/pkg/model"
	"keel/pkg/taskgraph"
)

func (a *App) cmdTask(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(a.Out, `Usage: keel task <subcommand> [flags]

Subcommands:
  create   Create a new task
  list     List all tasks
  get      Inspect a specific task
  update   Update task status, assignee, or results
  delete   Delete a task
  ready    List tasks that are ready to be worked on (dependencies met)
`)
		return 0
	}

	sub := args[0]
	subArgs := args[1:]

	s, err := a.getStore()
	if err != nil {
		fmt.Fprintf(a.Err, "Error: %v\n", err)
		return 1
	}

	switch sub {
	case "create":
		fs := flag.NewFlagSet("task create", flag.ContinueOnError)
		id := fs.String("id", "", "Unique task slug (required)")
		title := fs.String("title", "", "Task title (required)")
		desc := fs.String("desc", "", "Task description")
		priority := fs.String("priority", "medium", "Priority: low, medium, high, critical")
		goalID := fs.String("goal", "", "Associated goal ID")
		depends := fs.String("depends", "", "Comma-separated list of prerequisite task IDs")
		deliverables := fs.String("deliverables", "", "Comma-separated list of expected deliverables")

		if err := fs.Parse(subArgs); err != nil {
			return 1
		}
		if *id == "" || *title == "" {
			fmt.Fprintf(a.Err, "Error: --id and --title flags are required\n")
			return 1
		}

		prio := strings.ToLower(strings.TrimSpace(*priority))
		switch prio {
		case "low", "medium", "high", "critical":
			// valid
		default:
			fmt.Fprintf(a.Err, "Error: invalid priority %q (allowed: low, medium, high, critical)\n", *priority)
			return 1
		}

		var deps []string
		if *depends != "" {
			for _, d := range strings.Split(*depends, ",") {
				d = strings.TrimSpace(d)
				if d != "" {
					deps = append(deps, d)
				}
			}
		}

		var delivs []string
		if *deliverables != "" {
			for _, d := range strings.Split(*deliverables, ",") {
				d = strings.TrimSpace(d)
				if d != "" {
					delivs = append(delivs, d)
				}
			}
		}

		task := model.Task{
			ID:           *id,
			Title:        *title,
			Description:  *desc,
			Priority:     prio,
			GoalID:       *goalID,
			Status:       "pending",
			DependsOn:    deps,
			Deliverables: delivs,
		}

		if err := s.CreateTask(task); err != nil {
			fmt.Fprintf(a.Err, "Error creating task: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Out, "✓ Created task %q [%s]\n", task.ID, task.Title)
		return 0

	case "list":
		fs := flag.NewFlagSet("task list", flag.ContinueOnError)
		statusFilter := fs.String("status", "", "Filter by status: ready, blocked, pending, in_progress, completed")
		asJSON := fs.Bool("json", false, "Output as JSON")

		if err := fs.Parse(subArgs); err != nil {
			return 1
		}

		tasks, err := s.ListTasks()
		if err != nil {
			fmt.Fprintf(a.Err, "Error: %v\n", err)
			return 1
		}

		var filtered []model.Task
		if *statusFilter == "ready" {
			eval, err := taskgraph.Evaluate(tasks)
			if err != nil {
				fmt.Fprintf(a.Err, "Error evaluating task graph: %v\n", err)
				return 1
			}
			for _, r := range eval.Ready {
				filtered = append(filtered, r.Task)
			}
		} else if *statusFilter == "blocked" {
			eval, err := taskgraph.Evaluate(tasks)
			if err != nil {
				fmt.Fprintf(a.Err, "Error evaluating task graph: %v\n", err)
				return 1
			}
			for _, b := range eval.Blocked {
				filtered = append(filtered, b.Task)
			}
		} else {
			for _, t := range tasks {
				if *statusFilter == "" || t.Status == *statusFilter {
					filtered = append(filtered, t)
				}
			}
		}

		if *asJSON {
			a.emitJSON(filtered)
			return 0
		}

		if len(filtered) == 0 {
			fmt.Fprintln(a.Out, "No tasks found.")
			return 0
		}

		fmt.Fprintf(a.Out, "%-18s %-12s %-10s %s\n", "ID", "STATUS", "PRIORITY", "TITLE")
		fmt.Fprintf(a.Out, "%s\n", strings.Repeat("-", 60))
		for _, t := range filtered {
			fmt.Fprintf(a.Out, "%-18s %-12s %-10s %s\n", t.ID, t.Status, t.Priority, t.Title)
		}
		return 0

	case "ready":
		fs := flag.NewFlagSet("task ready", flag.ContinueOnError)
		fs.SetOutput(a.Err)
		asJSON := fs.Bool("json", false, "Output in JSON format")
		if err := fs.Parse(subArgs); err != nil {
			return 1
		}

		tasks, err := s.ListTasks()
		if err != nil {
			fmt.Fprintf(a.Err, "Error: %v\n", err)
			return 1
		}
		eval, err := taskgraph.Evaluate(tasks)
		if err != nil {
			fmt.Fprintf(a.Err, "Error evaluating task graph: %v\n", err)
			return 1
		}

		if *asJSON {
			readyTasks := make([]model.Task, 0, len(eval.Ready))
			for _, r := range eval.Ready {
				readyTasks = append(readyTasks, r.Task)
			}
			a.emitJSON(readyTasks)
			return 0
		}

		if len(eval.Ready) == 0 {
			fmt.Fprintln(a.Out, "No tasks currently ready (check dependencies or create new tasks).")
			return 0
		}

		fmt.Fprintln(a.Out, "Actionable Ready Tasks:")
		for _, r := range eval.Ready {
			prio := ""
			if r.Task.Priority != "" {
				prio = fmt.Sprintf(" [%s]", strings.ToUpper(r.Task.Priority))
			}
			fmt.Fprintf(a.Out, "  → %-16s%s %s\n", r.Task.ID, prio, r.Task.Title)
		}
		return 0

	case "get":
		if len(subArgs) == 0 {
			fmt.Fprintf(a.Err, "Usage: keel task get <id>\n")
			return 1
		}
		t, err := s.GetTask(subArgs[0])
		if err != nil {
			fmt.Fprintf(a.Err, "Error: %v\n", err)
			return 1
		}
		a.emitJSON(t)
		return 0

	case "delete":
		if len(subArgs) == 0 {
			fmt.Fprintf(a.Err, "Usage: keel task delete <id>\n")
			return 1
		}
		taskID := subArgs[0]
		if err := s.DeleteTask(taskID); err != nil {
			fmt.Fprintf(a.Err, "Error deleting task: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Out, "✓ Deleted task %q\n", taskID)
		return 0

	case "update":
		if len(subArgs) == 0 {
			fmt.Fprintf(a.Err, "Usage: keel task update <id> [--status <status>] [--result <result>] ...\n")
			return 1
		}

		var taskID string
		var flagArgs []string
		if !strings.HasPrefix(subArgs[0], "-") {
			taskID = subArgs[0]
			flagArgs = subArgs[1:]
		} else {
			flagArgs = subArgs
		}

		fs := flag.NewFlagSet("task update", flag.ContinueOnError)
		status := fs.String("status", "", "New status: pending, in_progress, completed, blocked, cancelled")
		result := fs.String("result", "", "Completion deliverables or summary")
		blocked := fs.String("blocked", "", "Reason if task is blocked")
		unblock := fs.Bool("unblock", false, "Clear blocked reason")
		clearDeps := fs.Bool("clear-deps", false, "Clear all task prerequisites")
		assignee := fs.String("assignee", "", "Assignee name")
		title := fs.String("title", "", "Updated title")
		desc := fs.String("desc", "", "Updated description")
		priority := fs.String("priority", "", "Updated priority: low, medium, high, critical")
		goal := fs.String("goal", "", "Updated goal ID")
		depends := fs.String("depends", "", "Updated comma-separated prerequisites")
		deliverables := fs.String("deliverables", "", "Updated comma-separated deliverables")

		if err := fs.Parse(flagArgs); err != nil {
			return 1
		}
		if taskID == "" {
			if fs.NArg() > 0 {
				taskID = fs.Arg(0)
			} else {
				fmt.Fprintf(a.Err, "Usage: keel task update <id> [--status <status>] [--result <result>] ...\n")
				return 1
			}
		}

		task, err := s.GetTask(taskID)
		if err != nil {
			fmt.Fprintf(a.Err, "Error: %v\n", err)
			return 1
		}

		if *status != "" {
			task.Status = *status
		}
		if *result != "" {
			task.Result = *result
		}
		if *blocked != "" {
			task.BlockedReason = *blocked
		}
		// Auto-clear blocked reason on progression or explicit unblock
		if *unblock || (*status != "" && *status != "blocked" && *blocked == "") {
			task.BlockedReason = ""
		}
		if *clearDeps {
			task.DependsOn = nil
		}
		if *assignee != "" {
			task.Assignee = *assignee
		}
		if *title != "" {
			task.Title = *title
		}
		if *desc != "" {
			task.Description = *desc
		}
		if *priority != "" {
			prio := strings.ToLower(strings.TrimSpace(*priority))
			switch prio {
			case "low", "medium", "high", "critical":
				task.Priority = prio
			default:
				fmt.Fprintf(a.Err, "Error: invalid priority %q (allowed: low, medium, high, critical)\n", *priority)
				return 1
			}
		}
		if *goal != "" {
			task.GoalID = *goal
		}
		if *depends != "" {
			var deps []string
			for _, d := range strings.Split(*depends, ",") {
				d = strings.TrimSpace(d)
				if d != "" {
					deps = append(deps, d)
				}
			}
			task.DependsOn = deps
		}
		if *deliverables != "" {
			var delivs []string
			for _, d := range strings.Split(*deliverables, ",") {
				d = strings.TrimSpace(d)
				if d != "" {
					delivs = append(delivs, d)
				}
			}
			task.Deliverables = delivs
		}

		if err := s.UpdateTask(task); err != nil {
			fmt.Fprintf(a.Err, "Error updating task: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Out, "✓ Updated task %q (status: %s)\n", task.ID, task.Status)
		return 0

	default:
		fmt.Fprintf(a.Err, "Unknown task subcommand %q\n", sub)
		return 1
	}
}

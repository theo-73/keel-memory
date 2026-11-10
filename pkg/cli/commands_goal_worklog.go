package cli

import (
	"flag"
	"fmt"
	"strings"

	"keel/pkg/model"
)

func (a *App) cmdGoal(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(a.Out, `Usage: keel goal <subcommand> [flags]

Subcommands:
  create   Define a new high-level goal
  list     List all goals
  get      Inspect a specific goal
  update   Update goal outcome, title, or status
  delete   Delete a goal
  select   Set a goal as the current active project goal
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
		fs := flag.NewFlagSet("goal create", flag.ContinueOnError)
		id := fs.String("id", "", "Unique goal slug (required)")
		title := fs.String("title", "", "Goal title (required)")
		outcome := fs.String("outcome", "", "Target outcome / vision")
		doneWhen := fs.String("done-when", "", "Comma-separated list of completion criteria")

		if err := fs.Parse(subArgs); err != nil {
			return 1
		}
		if *id == "" || *title == "" {
			fmt.Fprintf(a.Err, "Error: --id and --title are required\n")
			return 1
		}

		var conds []string
		if *doneWhen != "" {
			for _, c := range strings.Split(*doneWhen, ",") {
				c = strings.TrimSpace(c)
				if c != "" {
					conds = append(conds, c)
				}
			}
		}

		goal := model.Goal{
			ID:       *id,
			Title:    *title,
			Outcome:  *outcome,
			Status:   "active",
			DoneWhen: conds,
		}

		if err := s.CreateGoal(goal); err != nil {
			fmt.Fprintf(a.Err, "Error creating goal: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Out, "✓ Created goal %q: %s\n", goal.ID, goal.Title)
		return 0

	case "list":
		fs := flag.NewFlagSet("goal list", flag.ContinueOnError)
		fs.SetOutput(a.Err)
		asJSON := fs.Bool("json", false, "Output in JSON format")
		if err := fs.Parse(subArgs); err != nil {
			return 1
		}
		goals, err := s.ListGoals()
		if err != nil {
			fmt.Fprintf(a.Err, "Error: %v\n", err)
			return 1
		}
		if *asJSON {
			a.emitJSON(goals)
			return 0
		}
		if len(goals) == 0 {
			fmt.Fprintln(a.Out, "No goals defined.")
			return 0
		}
		for _, g := range goals {
			fmt.Fprintf(a.Out, "• %-16s [%s] %s\n", g.ID, g.Status, g.Title)
			if g.Outcome != "" {
				fmt.Fprintf(a.Out, "  > %s\n", g.Outcome)
			}
		}
		return 0

	case "get":
		if len(subArgs) == 0 {
			fmt.Fprintf(a.Err, "Usage: keel goal get <id>\n")
			return 1
		}
		g, err := s.GetGoal(subArgs[0])
		if err != nil {
			fmt.Fprintf(a.Err, "Error: %v\n", err)
			return 1
		}
		a.emitJSON(g)
		return 0

	case "update":
		if len(subArgs) == 0 {
			fmt.Fprintf(a.Err, "Usage: keel goal update <id> [--title <title>] [--outcome <outcome>] [--status <status>] ...\n")
			return 1
		}

		var goalID string
		var flagArgs []string
		if !strings.HasPrefix(subArgs[0], "-") {
			goalID = subArgs[0]
			flagArgs = subArgs[1:]
		} else {
			flagArgs = subArgs
		}

		fs := flag.NewFlagSet("goal update", flag.ContinueOnError)
		title := fs.String("title", "", "Updated goal title")
		outcome := fs.String("outcome", "", "Updated outcome")
		status := fs.String("status", "", "Updated status: active, completed, blocked, paused")
		doneWhen := fs.String("done-when", "", "Updated comma-separated completion criteria")

		if err := fs.Parse(flagArgs); err != nil {
			return 1
		}

		if goalID == "" {
			if fs.NArg() > 0 {
				goalID = fs.Arg(0)
			} else {
				fmt.Fprintf(a.Err, "Usage: keel goal update <id> [--title <title>] [--outcome <outcome>] ...\n")
				return 1
			}
		}

		goal, err := s.GetGoal(goalID)
		if err != nil {
			fmt.Fprintf(a.Err, "Error: %v\n", err)
			return 1
		}

		if *title != "" {
			goal.Title = *title
		}
		if *outcome != "" {
			goal.Outcome = *outcome
		}
		if *status != "" {
			goal.Status = *status
		}
		if *doneWhen != "" {
			var conds []string
			for _, c := range strings.Split(*doneWhen, ",") {
				c = strings.TrimSpace(c)
				if c != "" {
					conds = append(conds, c)
				}
			}
			goal.DoneWhen = conds
		}

		if err := s.UpdateGoal(goal); err != nil {
			fmt.Fprintf(a.Err, "Error updating goal: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Out, "✓ Updated goal %q\n", goal.ID)
		return 0

	case "delete":
		if len(subArgs) == 0 {
			fmt.Fprintf(a.Err, "Usage: keel goal delete <id>\n")
			return 1
		}
		goalID := subArgs[0]
		if err := s.DeleteGoal(goalID); err != nil {
			fmt.Fprintf(a.Err, "Error deleting goal: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Out, "✓ Deleted goal %q\n", goalID)
		return 0

	case "select":
		if len(subArgs) == 0 {
			fmt.Fprintf(a.Err, "Usage: keel goal select <id>\n")
			return 1
		}
		goalID := subArgs[0]
		if err := s.SetCurrentGoal(goalID); err != nil {
			fmt.Fprintf(a.Err, "Error selecting goal: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Out, "✓ Current active goal set to %q\n", goalID)
		return 0

	default:
		fmt.Fprintf(a.Err, "Unknown goal subcommand %q\n", sub)
		return 1
	}
}

func (a *App) cmdWorklog(args []string) int {
	// Reorder args so flags come first, allowing:
	// keel worklog "Summary text" --next "step 1" --harness claude
	var flagArgs []string
	var posArgs []string
	boolFlags := map[string]bool{"list": true, "json": true}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			name := strings.TrimLeft(arg, "-")
			if strings.Contains(name, "=") || boolFlags[name] {
				continue
			}
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else {
			posArgs = append(posArgs, arg)
		}
	}

	reordered := append(flagArgs, posArgs...)

	fs := flag.NewFlagSet("worklog", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	harness := fs.String("harness", "cli-agent", "Agent name / harness (claude, cursor, etc.)")
	next := fs.String("next", "", "Comma-separated list of next actions left for the next agent")
	list := fs.Bool("list", false, "List recent worklogs")
	limit := fs.Int("limit", 10, "Maximum number of recent worklogs to list (0 for all)")
	asJSON := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(reordered); err != nil {
		return 1
	}

	s, err := a.getStore()
	if err != nil {
		fmt.Fprintf(a.Err, "Error: %v\n", err)
		return 1
	}

	if *list {
		logs, err := s.ListWorklogs()
		if err != nil {
			fmt.Fprintf(a.Err, "Error: %v\n", err)
			return 1
		}
		if *limit > 0 && len(logs) > *limit {
			logs = logs[:*limit]
		}
		if *asJSON {
			a.emitJSON(logs)
			return 0
		}
		if len(logs) == 0 {
			fmt.Fprintln(a.Out, "No worklogs recorded.")
			return 0
		}
		for _, l := range logs {
			fmt.Fprintf(a.Out, "[%s] by %s\n  %s\n\n", l.CreatedAt.Format("2006-01-02 15:04"), l.Harness, l.Summary)
		}
		return 0
	}

	if fs.NArg() == 0 {
		fmt.Fprintf(a.Err, "Usage: keel worklog \"<summary of work done>\" [--next \"step 1, step 2\"]\n")
		return 1
	}

	summary := strings.Join(fs.Args(), " ")
	var nextSteps []string
	if *next != "" {
		for _, step := range strings.Split(*next, ",") {
			step = strings.TrimSpace(step)
			if step != "" {
				nextSteps = append(nextSteps, step)
			}
		}
	}

	entry := model.WorklogEntry{
		Harness:   *harness,
		Summary:   summary,
		NextSteps: nextSteps,
	}

	if err := s.RecordWorklog(entry); err != nil {
		fmt.Fprintf(a.Err, "Error saving worklog: %v\n", err)
		return 1
	}
	fmt.Fprintf(a.Out, "✓ Session worklog checkpoint recorded successfully.\n")
	return 0
}

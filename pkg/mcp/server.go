package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"keel/pkg/brief"
	"keel/pkg/model"
	"keel/pkg/store"
	"keel/pkg/taskgraph"
)

type Server struct {
	store *store.Store
	in    io.Reader
	out   io.Writer
}

func NewServer(s *store.Store, in io.Reader, out io.Writer) *Server {
	return &Server{store: s, in: in, out: out}
}

func (s *Server) Run() error {
	scanner := bufio.NewScanner(s.in)
	buf := make([]byte, 4096)
	scanner.Buffer(buf, 4*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.sendError(nil, -32700, "Parse error", err.Error())
			continue
		}

		s.handleRequest(req)
	}

	return scanner.Err()
}

func (s *Server) handleRequest(req JSONRPCRequest) {
	if req.ID == nil {
		if req.Method == "notifications/initialized" {
			// Ready
		}
		return
	}

	switch req.Method {
	case "initialize":
		res := InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: ServerCapabilities{
				Tools:     &ToolsCapability{ListChanged: false},
				Resources: &ResourcesCapability{Subscribe: false, ListChanged: false},
				Prompts:   &PromptsCapability{ListChanged: false},
			},
			ServerInfo: ServerInfo{
				Name:    "keel-memory",
				Version: "1.0.0",
			},
		}
		s.sendResult(req.ID, res)

	case "ping":
		s.sendResult(req.ID, map[string]any{})

	case "prompts/list":
		s.sendResult(req.ID, map[string]any{
			"prompts": []Prompt{
				{
					Name:        "keel_orient",
					Description: "Start of session: retrieve dense project brief, inspect ready tasks, and begin highest-priority work.",
				},
				{
					Name:        "keel_checkpoint",
					Description: "End of session: record completed deliverables, verify tests, and leave handoff worklog.",
				},
			},
		})

	case "prompts/get":
		var params struct {
			Name      string            `json:"name"`
			Arguments map[string]string `json:"arguments,omitempty"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
		switch params.Name {
		case "keel_orient":
			s.sendResult(req.ID, map[string]any{
				"description": "Orientation prompt for starting an autonomous agent session.",
				"messages": []PromptMessage{
					{
						Role: "user",
						Content: PromptContent{
							Type: "text",
							Text: "Please start by invoking the 'keel_brief' tool to orient yourself with current project goals, health, and recent session notes. Then review ready tasks via 'keel_task_ready', select the highest-priority task, mark it in-progress using 'keel_task_update', and proceed with execution.",
						},
					},
				},
			})
		case "keel_checkpoint":
			s.sendResult(req.ID, map[string]any{
				"description": "Checkpoint and handoff prompt for wrapping up an agent session.",
				"messages": []PromptMessage{
					{
						Role: "user",
						Content: PromptContent{
							Type: "text",
							Text: "Before finishing your turn, ensure all code compiles and test suites pass. Update any completed tasks with 'keel_task_update' providing the verified deliverables in 'result'. Finally, call 'keel_worklog_record' with a concise summary of your progress and clear 'next_steps' for the next session.",
						},
					},
				},
			})
		default:
			s.sendError(req.ID, -32602, fmt.Sprintf("Unknown prompt: %s", params.Name), nil)
		}

	case "tools/list":
		s.sendResult(req.ID, map[string]any{
			"tools": s.getToolsList(),
		})

	case "tools/call":
		s.handleToolCall(req.ID, req.Params)

	case "resources/list":
		s.sendResult(req.ID, map[string]any{
			"resources": []Resource{
				{URI: "keel://project/brief", Name: "Project Brief", MimeType: "text/markdown"},
				{URI: "keel://project/state", Name: "Project State", MimeType: "application/json"},
				{URI: "keel://project/tasks", Name: "All Project Tasks", MimeType: "application/json"},
				{URI: "keel://project/goals", Name: "All Project Goals", MimeType: "application/json"},
				{URI: "keel://project/worklogs", Name: "Session Worklogs", MimeType: "application/json"},
			},
		})

	case "resources/read":
		var params ReadResourceParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
		s.handleResourceRead(req.ID, params.URI)

	default:
		s.sendError(req.ID, -32601, "Method not found", req.Method)
	}
}

func (s *Server) getToolsList() []Tool {
	return []Tool{
		{
			Name:        "keel_brief",
			Description: "Get the current project brief: orientation, goals, ready tasks, blocked tasks, and previous session notes.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{}},
		},
		{
			Name:        "keel_task_list",
			Description: "List all tasks in the project, optionally filtered by status.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"status": {
						Type:        "string",
						Description: "Optional filter (ready, blocked, pending, in_progress, completed)",
						Enum:        []string{"ready", "blocked", "pending", "in_progress", "completed"},
					},
					"goalId": {
						Type:        "string",
						Description: "Optional filter by associated goal ID",
					},
				},
			},
		},
		{
			Name:        "keel_task_create",
			Description: "Create a new task with dependencies and priority.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"id":          {Type: "string", Description: "Unique task slug (e.g. build-auth-api)"},
					"title":       {Type: "string", Description: "Clear title for the task"},
					"description": {Type: "string", Description: "Detailed description of requirements"},
					"priority":    {Type: "string", Description: "Priority level", Enum: []string{"low", "medium", "high", "critical"}},
					"goalId":      {Type: "string", Description: "Associated goal ID"},
					"dependsOn":   {Type: "array", Description: "List of prerequisite task IDs", Items: &ItemDef{Type: "string"}},
				},
				Required: []string{"id", "title"},
			},
		},
		{
			Name:        "keel_task_update",
			Description: "Update a task's progress, status, result, deliverables, or blocker reason.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"id":            {Type: "string", Description: "Task ID to update"},
					"status":        {Type: "string", Description: "New status", Enum: []string{"pending", "ready", "in_progress", "completed", "blocked", "cancelled"}},
					"result":        {Type: "string", Description: "Summary of completion deliverables or results"},
					"blockedReason": {Type: "string", Description: "Explanation if task is blocked"},
					"assignee":      {Type: "string", Description: "Agent or person working on this"},
					"title":         {Type: "string", Description: "Updated title"},
					"description":   {Type: "string", Description: "Updated description"},
					"priority":      {Type: "string", Description: "Updated priority", Enum: []string{"low", "medium", "high", "critical"}},
					"goalId":        {Type: "string", Description: "Associated goal ID"},
					"dependsOn":     {Type: "array", Description: "Updated dependencies", Items: &ItemDef{Type: "string"}},
					"deliverables":  {Type: "array", Description: "Updated deliverables", Items: &ItemDef{Type: "string"}},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "keel_task_get",
			Description: "Get complete details and status for a single task by ID.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"id": {Type: "string", Description: "Task ID to inspect"},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "keel_task_delete",
			Description: "Delete a task by ID.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"id": {Type: "string", Description: "Task ID to delete"},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "keel_goal_create",
			Description: "Create a new project goal with target outcome.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"id":       {Type: "string", Description: "Unique goal slug"},
					"title":    {Type: "string", Description: "Goal title"},
					"outcome":  {Type: "string", Description: "Target outcome or vision"},
					"doneWhen": {Type: "array", Description: "Completion criteria", Items: &ItemDef{Type: "string"}},
				},
				Required: []string{"id", "title"},
			},
		},
		{
			Name:        "keel_goal_get",
			Description: "Get complete details for a single goal by ID.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"id": {Type: "string", Description: "Goal ID to inspect"},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "keel_goal_select",
			Description: "Set the currently active project goal.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"id": {Type: "string", Description: "Goal ID to activate"},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "keel_goal_delete",
			Description: "Delete a project goal by ID.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"id": {Type: "string", Description: "Goal ID to delete"},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "keel_goal_list",
			Description: "List all project goals.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{}},
		},
		{
			Name:        "keel_goal_update",
			Description: "Update a project goal's title, outcome, or status.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"id":       {Type: "string", Description: "Goal ID to update"},
					"title":    {Type: "string", Description: "Updated title"},
					"outcome":  {Type: "string", Description: "Updated outcome"},
					"status":   {Type: "string", Description: "Updated status", Enum: []string{"active", "completed", "blocked", "paused"}},
					"doneWhen": {Type: "array", Description: "Updated completion criteria", Items: &ItemDef{Type: "string"}},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "keel_worklog_list",
			Description: "List all historical session worklog checkpoints.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{}},
		},
		{
			Name:        "keel_task_ready",
			Description: "List all currently actionable tasks whose dependencies are fully completed.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{}},
		},
		{
			Name:        "keel_doctor",
			Description: "Run health diagnostics, check DAG integrity, and detect broken dependencies.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{}},
		},
		{
			Name:        "keel_worklog_record",
			Description: "Record an agent session checkpoint, work completed, and instructions left for the next session.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"summary":   {Type: "string", Description: "What was accomplished during this session"},
					"harness":   {Type: "string", Description: "Name of the agent harness (e.g. claude-code, cursor, antigravity)"},
					"nextSteps": {Type: "array", Description: "Immediate next actions for the next agent", Items: &ItemDef{Type: "string"}},
				},
				Required: []string{"summary"},
			},
		},
	}
}

func normalizeJSONKeys(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return data
	}
	normalized := make(map[string]any, len(raw)*2)
	for k, v := range raw {
		normalized[k] = v
		camel := snakeToCamel(k)
		if camel != k {
			if _, exists := normalized[camel]; !exists {
				normalized[camel] = v
			}
		}
	}
	out, err := json.Marshal(normalized)
	if err != nil {
		return data
	}
	return out
}

func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	if len(parts) <= 1 {
		return s
	}
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

func (s *Server) handleToolCall(id any, rawParams json.RawMessage) {
	var params ToolCallParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		s.sendError(id, -32602, "Invalid params", err.Error())
		return
	}
	params.Arguments = normalizeJSONKeys(params.Arguments)

	switch params.Name {
	case "keel_brief":
		b, err := brief.Generate(s.store)
		if err != nil {
			s.sendToolError(id, fmt.Sprintf("Failed to generate brief: %v", err))
			return
		}
		s.sendToolSuccess(id, b.FormatTerminal())

	case "keel_task_list":
		var args struct {
			Status string `json:"status"`
			GoalID string `json:"goalId"`
		}
		_ = json.Unmarshal(params.Arguments, &args)

		tasks, err := s.store.ListTasks()
		if err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		if args.Status == "ready" {
			eval, err := taskgraph.Evaluate(tasks)
			if err != nil {
				s.sendToolError(id, err.Error())
				return
			}
			res := eval.Ready
			if args.GoalID != "" {
				var filteredReady []taskgraph.TaskEvaluation
				for _, r := range res {
					if r.Task.GoalID == args.GoalID {
						filteredReady = append(filteredReady, r)
					}
				}
				res = filteredReady
			}
			data, _ := json.MarshalIndent(res, "", "  ")
			s.sendToolSuccess(id, string(data))
			return
		}
		if args.Status == "blocked" {
			eval, err := taskgraph.Evaluate(tasks)
			if err != nil {
				s.sendToolError(id, err.Error())
				return
			}
			res := eval.Blocked
			if args.GoalID != "" {
				var filteredBlocked []taskgraph.TaskEvaluation
				for _, b := range res {
					if b.Task.GoalID == args.GoalID {
						filteredBlocked = append(filteredBlocked, b)
					}
				}
				res = filteredBlocked
			}
			data, _ := json.MarshalIndent(res, "", "  ")
			s.sendToolSuccess(id, string(data))
			return
		}

		var filtered []model.Task
		for _, t := range tasks {
			if (args.Status == "" || t.Status == args.Status) && (args.GoalID == "" || t.GoalID == args.GoalID) {
				filtered = append(filtered, t)
			}
		}
		data, _ := json.MarshalIndent(filtered, "", "  ")
		s.sendToolSuccess(id, string(data))

	case "keel_task_create":
		var args model.Task
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		if args.Status == "" {
			args.Status = "pending"
		}
		if err := s.store.CreateTask(args); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		s.sendToolSuccess(id, fmt.Sprintf("Task %q created successfully", args.ID))

	case "keel_task_update":
		var args struct {
			ID            string   `json:"id"`
			Status        string   `json:"status"`
			Result        string   `json:"result"`
			BlockedReason string   `json:"blockedReason"`
			Assignee      string   `json:"assignee"`
			Title         string   `json:"title"`
			Description   string   `json:"description"`
			Priority      string   `json:"priority"`
			GoalID        string   `json:"goalId"`
			DependsOn     []string `json:"dependsOn"`
			Deliverables  []string `json:"deliverables"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		task, err := s.store.GetTask(args.ID)
		if err != nil {
			s.sendToolError(id, fmt.Sprintf("Task %q not found: %v", args.ID, err))
			return
		}
		if args.Status != "" {
			task.Status = args.Status
		}
		if args.Result != "" {
			task.Result = args.Result
		}
		if args.BlockedReason != "" {
			task.BlockedReason = args.BlockedReason
		}
		if args.Assignee != "" {
			task.Assignee = args.Assignee
		}
		if args.Title != "" {
			task.Title = args.Title
		}
		if args.Description != "" {
			task.Description = args.Description
		}
		if args.Priority != "" {
			task.Priority = args.Priority
		}
		if args.GoalID != "" {
			task.GoalID = args.GoalID
		}
		if args.DependsOn != nil {
			task.DependsOn = args.DependsOn
		}
		if args.Deliverables != nil {
			task.Deliverables = args.Deliverables
		}
		if err := s.store.UpdateTask(task); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		s.sendToolSuccess(id, fmt.Sprintf("Task %q updated successfully", task.ID))

	case "keel_task_get":
		var args struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		task, err := s.store.GetTask(args.ID)
		if err != nil {
			s.sendToolError(id, fmt.Sprintf("Task %q not found: %v", args.ID, err))
			return
		}
		data, _ := json.MarshalIndent(task, "", "  ")
		s.sendToolSuccess(id, string(data))

	case "keel_task_delete":
		var args struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		if err := s.store.DeleteTask(args.ID); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		s.sendToolSuccess(id, fmt.Sprintf("Task %q deleted successfully", args.ID))

	case "keel_goal_create":
		var args model.Goal
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		if args.Status == "" {
			args.Status = "active"
		}
		if err := s.store.CreateGoal(args); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		s.sendToolSuccess(id, fmt.Sprintf("Goal %q created successfully", args.ID))

	case "keel_goal_list":
		goals, err := s.store.ListGoals()
		if err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		data, _ := json.MarshalIndent(goals, "", "  ")
		s.sendToolSuccess(id, string(data))

	case "keel_goal_update":
		var args struct {
			ID       string   `json:"id"`
			Title    string   `json:"title"`
			Outcome  string   `json:"outcome"`
			Status   string   `json:"status"`
			DoneWhen []string `json:"doneWhen"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		goal, err := s.store.GetGoal(args.ID)
		if err != nil {
			s.sendToolError(id, fmt.Sprintf("Goal %q not found: %v", args.ID, err))
			return
		}
		if args.Title != "" {
			goal.Title = args.Title
		}
		if args.Outcome != "" {
			goal.Outcome = args.Outcome
		}
		if args.Status != "" {
			goal.Status = args.Status
		}
		if args.DoneWhen != nil {
			goal.DoneWhen = args.DoneWhen
		}
		if err := s.store.UpdateGoal(goal); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		s.sendToolSuccess(id, fmt.Sprintf("Goal %q updated successfully", goal.ID))

	case "keel_goal_get":
		var args struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		goal, err := s.store.GetGoal(args.ID)
		if err != nil {
			s.sendToolError(id, fmt.Sprintf("Goal %q not found: %v", args.ID, err))
			return
		}
		data, _ := json.MarshalIndent(goal, "", "  ")
		s.sendToolSuccess(id, string(data))

	case "keel_goal_select":
		var args struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		if err := s.store.SetCurrentGoal(args.ID); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		s.sendToolSuccess(id, fmt.Sprintf("Active goal set to %q", args.ID))

	case "keel_goal_delete":
		var args struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		if err := s.store.DeleteGoal(args.ID); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		s.sendToolSuccess(id, fmt.Sprintf("Goal %q deleted successfully", args.ID))

	case "keel_worklog_list":
		logs, err := s.store.ListWorklogs()
		if err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		data, _ := json.MarshalIndent(logs, "", "  ")
		s.sendToolSuccess(id, string(data))

	case "keel_worklog_record":
		var args struct {
			Summary   string   `json:"summary"`
			Harness   string   `json:"harness"`
			NextSteps []string `json:"nextSteps"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		if args.Harness == "" {
			args.Harness = "mcp-agent"
		}
		entry := model.WorklogEntry{
			Summary:   args.Summary,
			Harness:   args.Harness,
			NextSteps: args.NextSteps,
		}
		if err := s.store.RecordWorklog(entry); err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		s.sendToolSuccess(id, "Session worklog recorded successfully")

	case "keel_task_ready":
		tasks, err := s.store.ListTasks()
		if err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		eval, err := taskgraph.Evaluate(tasks)
		if err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		data, _ := json.MarshalIndent(eval.Ready, "", "  ")
		s.sendToolSuccess(id, string(data))

	case "keel_doctor":
		tasks, err := s.store.ListTasks()
		if err != nil {
			s.sendToolError(id, err.Error())
			return
		}
		eval, err := taskgraph.Evaluate(tasks)
		if err != nil {
			s.sendToolError(id, fmt.Sprintf("DAG evaluation error: %v", err))
			return
		}
		state, err := s.store.State()
		if err != nil {
			s.sendToolError(id, fmt.Sprintf("State read error: %v", err))
			return
		}
		var missing []string
		for _, b := range eval.Blocked {
			for _, dep := range b.Blockers {
				if strings.Contains(dep, "(missing)") {
					missing = append(missing, fmt.Sprintf("Task %s -> %s", b.Task.ID, dep))
				}
			}
		}
		report := fmt.Sprintf("Health: %s | Phase: %s | Total Tasks: %d | Ready: %d | Missing Dependencies: %d",
			state.Health, state.CurrentPhase, len(tasks), len(eval.Ready), len(missing))
		if len(missing) > 0 {
			report += fmt.Sprintf(" | Issues: %s", strings.Join(missing, ", "))
		}
		s.sendToolSuccess(id, report)

	default:
		s.sendError(id, -32601, "Unknown tool", params.Name)
	}
}

func (s *Server) handleResourceRead(id any, uri string) {
	switch uri {
	case "keel://project/brief":
		b, err := brief.Generate(s.store)
		if err != nil {
			s.sendError(id, -32603, "Internal error", err.Error())
			return
		}
		s.sendResult(id, map[string]any{
			"contents": []ResourceContent{
				{URI: uri, MimeType: "text/markdown", Text: b.FormatTerminal()},
			},
		})
	case "keel://project/state":
		st, err := s.store.State()
		if err != nil {
			s.sendError(id, -32603, "Internal error", err.Error())
			return
		}
		data, _ := json.MarshalIndent(st, "", "  ")
		s.sendResult(id, map[string]any{
			"contents": []ResourceContent{
				{URI: uri, MimeType: "application/json", Text: string(data)},
			},
		})
	case "keel://project/tasks":
		tasks, err := s.store.ListTasks()
		if err != nil {
			s.sendError(id, -32603, "Internal error", err.Error())
			return
		}
		data, _ := json.MarshalIndent(tasks, "", "  ")
		s.sendResult(id, map[string]any{
			"contents": []ResourceContent{
				{URI: uri, MimeType: "application/json", Text: string(data)},
			},
		})
	case "keel://project/goals":
		goals, err := s.store.ListGoals()
		if err != nil {
			s.sendError(id, -32603, "Internal error", err.Error())
			return
		}
		data, _ := json.MarshalIndent(goals, "", "  ")
		s.sendResult(id, map[string]any{
			"contents": []ResourceContent{
				{URI: uri, MimeType: "application/json", Text: string(data)},
			},
		})
	case "keel://project/worklogs":
		logs, err := s.store.ListWorklogs()
		if err != nil {
			s.sendError(id, -32603, "Internal error", err.Error())
			return
		}
		data, _ := json.MarshalIndent(logs, "", "  ")
		s.sendResult(id, map[string]any{
			"contents": []ResourceContent{
				{URI: uri, MimeType: "application/json", Text: string(data)},
			},
		})
	default:
		s.sendError(id, -32602, "Resource not found", uri)
	}
}

func (s *Server) sendToolSuccess(id any, text string) {
	s.sendResult(id, ToolCallResult{
		Content: []TextContent{{Type: "text", Text: text}},
	})
}

func (s *Server) sendToolError(id any, message string) {
	s.sendResult(id, ToolCallResult{
		Content: []TextContent{{Type: "text", Text: "Error: " + message}},
		IsError: true,
	})
}

func (s *Server) sendResult(id any, result any) {
	res := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	bytes, _ := json.Marshal(res)
	_, _ = fmt.Fprintln(s.out, string(bytes))
}

func (s *Server) sendError(id any, code int, message string, data any) {
	res := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	bytes, _ := json.Marshal(res)
	_, _ = fmt.Fprintln(s.out, string(bytes))
}

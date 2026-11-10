package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"keel/pkg/store"
)

var Version = "1.0.0"

type App struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

func NewApp(in io.Reader, out, errOut io.Writer) *App {
	return &App{In: in, Out: out, Err: errOut}
}

func (a *App) Run(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || (args[0] == "help" && len(args) == 1) {
		a.printRootHelp()
		return 0
	}

	if args[0] == "--version" || args[0] == "-v" || args[0] == "version" {
		fmt.Fprintf(a.Out, "keel version %s\n", Version)
		return 0
	}

	switch args[0] {
	case "init":
		return a.cmdInit(args[1:])
	case "brief":
		return a.cmdBrief(args[1:])
	case "task":
		return a.cmdTask(args[1:])
	case "goal":
		return a.cmdGoal(args[1:])
	case "worklog":
		return a.cmdWorklog(args[1:])
	case "doctor":
		return a.cmdDoctor(args[1:])
	case "mcp":
		return a.cmdMCP(args[1:])
	case "serve":
		return a.cmdServe(args[1:])
	default:
		fmt.Fprintf(a.Err, "Error: unknown command %q\nRun 'keel --help' for available commands.\n", args[0])
		return 1
	}
}

func (a *App) getStore() (*store.Store, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return store.Open(cwd)
}

func (a *App) emitJSON(data any) {
	enc := json.NewEncoder(a.Out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)
}

func (a *App) printRootHelp() {
	help := `Keel · The Persistent Mission Control for Coding Agents

USAGE:
  keel <command> [arguments...]

CORE COMMANDS:
  keel init          Initialize a Keel project with agent instructions
  keel brief         Generate a dense, token-compressed project brief for session start
  keel task          Manage project tasks and dependencies (create, list, update, ready)
  keel goal          Manage high-level project goals and outcomes
  keel worklog       Record session progress and handoff notes for the next agent
  keel doctor        Audit project health, task graph integrity, and detect drift

INTEGRATIONS & INTERFACES:
  keel mcp           Launch the native Model Context Protocol (MCP) stdio server
  keel serve         Start the real-time reactive mission control web dashboard

FLAGS:
  --help, -h         Show help for any command
  --version, -v      Print Keel version

Run 'keel <command> --help' for more information on a specific command.
`
	fmt.Fprint(a.Out, help)
}

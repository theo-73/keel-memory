package cli

import (
	"flag"
	"fmt"

	"keel/pkg/brief"
	"keel/pkg/store"
)

func (a *App) cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	name := fs.String("name", "", "Project name")
	desc := fs.String("desc", "", "Short description of project")
	path := fs.String("path", ".", "Target directory path")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	targetDir := *path
	s, err := store.Init(targetDir, *name, *desc)
	if err != nil {
		fmt.Fprintf(a.Err, "Error initializing Keel project: %v\n", err)
		return 1
	}

	manifest, _ := s.Manifest()
	fmt.Fprintf(a.Out, "✓ Initialized Keel project %q in %s\n", manifest.Name, s.Root)
	fmt.Fprintf(a.Out, "  - Configured agent instructions (AGENTS.md, CLAUDE.md, .cursor/rules/keel.mdc)\n")
	fmt.Fprintf(a.Out, "  - Next: define your first goal with 'keel goal create' or inspect context with 'keel brief'\n")
	return 0
}

func (a *App) cmdBrief(args []string) int {
	fs := flag.NewFlagSet("brief", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "Output brief as JSON instead of Markdown")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	s, err := a.getStore()
	if err != nil {
		fmt.Fprintf(a.Err, "Error: %v\n", err)
		return 1
	}

	b, err := brief.Generate(s)
	if err != nil {
		fmt.Fprintf(a.Err, "Error generating brief: %v\n", err)
		return 1
	}

	if *asJSON {
		jsonStr, err := b.FormatJSON()
		if err != nil {
			fmt.Fprintf(a.Err, "JSON encode error: %v\n", err)
			return 1
		}
		fmt.Fprintln(a.Out, jsonStr)
	} else {
		fmt.Fprint(a.Out, b.FormatTerminal())
	}
	return 0
}

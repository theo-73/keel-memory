package cli

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"keel/pkg/dashboard"
	"keel/pkg/mcp"
	"keel/pkg/taskgraph"
)

func (a *App) cmdMCP(args []string) int {
	s, err := a.getStore()
	if err != nil {
		fmt.Fprintf(a.Err, "Error: %v\n", err)
		return 1
	}

	server := mcp.NewServer(s, a.In, a.Out)
	if err := server.Run(); err != nil {
		fmt.Fprintf(a.Err, "MCP server error: %v\n", err)
		return 1
	}
	return 0
}

func (a *App) cmdServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.Int("port", 7788, "HTTP port to bind dashboard")
	openBrowser := fs.Bool("open", true, "Open dashboard in default browser")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	s, err := a.getStore()
	if err != nil {
		fmt.Fprintf(a.Err, "Error: %v\n", err)
		return 1
	}

	srv := dashboard.NewServer(s)
	addr := fmt.Sprintf("127.0.0.1:%d", *port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		// Try fallback port
		addr = "127.0.0.1:0"
		listener, err = net.Listen("tcp", addr)
		if err != nil {
			fmt.Fprintf(a.Err, "Failed to start listener: %v\n", err)
			return 1
		}
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d", actualPort)

	fmt.Fprintf(a.Out, "⚓ Keel Mission Control active at: %s\n", url)
	fmt.Fprintf(a.Out, "  - Live Server-Sent Events (SSE) streaming enabled\n")
	fmt.Fprintf(a.Out, "  - Press Ctrl+C to terminate\n")

	if *openBrowser {
		go func() {
			switch runtime.GOOS {
			case "windows":
				_ = exec.Command("cmd", "/c", "start", url).Start()
			case "darwin":
				_ = exec.Command("open", url).Start()
			case "linux":
				_ = exec.Command("xdg-open", url).Start()
			}
		}()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		fmt.Fprintf(a.Err, "Dashboard server error: %v\n", err)
		return 1
	case <-ctx.Done():
		fmt.Fprintln(a.Out, "\nShutting down mission control gracefully...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(a.Err, "Server shutdown error: %v\n", err)
			return 1
		}
		return 0
	}
}

func (a *App) cmdDoctor(args []string) int {
	s, err := a.getStore()
	if err != nil {
		fmt.Fprintf(a.Err, "Error: %v\n", err)
		return 1
	}

	fmt.Fprintf(a.Out, "Running Keel Project Doctor on %s...\n\n", s.Root)

	// 1. Check Manifest
	manifest, err := s.Manifest()
	if err != nil {
		fmt.Fprintf(a.Err, "❌ Corrupt manifest: %v\n", err)
		return 1
	}
	fmt.Fprintf(a.Out, "✓ Project Manifest: %s (v%s)\n", manifest.Name, manifest.SchemaVersion)

	// 2. Check State
	state, err := s.State()
	if err != nil {
		fmt.Fprintf(a.Err, "❌ Corrupt project state: %v\n", err)
		return 1
	}
	fmt.Fprintf(a.Out, "✓ Project State: Health=%s, Phase=%s\n", state.Health, state.CurrentPhase)

	// 3. Task Graph Evaluation
	tasks, err := s.ListTasks()
	if err != nil {
		fmt.Fprintf(a.Err, "❌ Cannot list tasks: %v\n", err)
		return 1
	}

	eval, err := taskgraph.Evaluate(tasks)
	if err != nil {
		fmt.Fprintf(a.Err, "❌ Task graph dependency error: %v\n", err)
		return 1
	}

	fmt.Fprintf(a.Out, "✓ Task Graph: %d total tasks\n", len(tasks))
	fmt.Fprintf(a.Out, "    - %d ready to execute\n", len(eval.Ready))
	fmt.Fprintf(a.Out, "    - %d currently in progress\n", len(eval.InProgress))
	fmt.Fprintf(a.Out, "    - %d blocked by prerequisites\n", len(eval.Blocked))
	fmt.Fprintf(a.Out, "    - %d completed\n", len(eval.Completed))

	// Scan for missing / non-existent task dependencies
	var missingBlockers []string
	for _, b := range eval.Blocked {
		for _, dep := range b.Blockers {
			if strings.Contains(dep, "(missing)") {
				missingBlockers = append(missingBlockers, fmt.Sprintf("Task %q depends on non-existent %s", b.Task.ID, dep))
			}
		}
	}
	if len(missingBlockers) > 0 {
		fmt.Fprintf(a.Err, "\n⚠️  Broken Task Dependencies Detected:\n")
		for _, m := range missingBlockers {
			fmt.Fprintf(a.Err, "    - %s\n", m)
		}
		return 1
	}

	// 4. Goals check
	goals, _ := s.ListGoals()
	fmt.Fprintf(a.Out, "✓ Goals: %d registered\n", len(goals))

	// 5. Worklogs check
	logs, _ := s.ListWorklogs()
	fmt.Fprintf(a.Out, "✓ Worklogs: %d session checkpoints recorded\n", len(logs))

	if state.Health == "at_risk" || state.Health == "blocked" {
		fmt.Fprintf(a.Out, "\n⚠️  Project Health is currently %q. Review blockers or risks above.\n", state.Health)
		return 1
	}

	fmt.Fprintf(a.Out, "\n🎉 Project is healthy and ready for autonomous agent sessions.\n")
	return 0
}

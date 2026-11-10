package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestCLIFullWorkflow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-cli-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	_ = os.Chdir(tmpDir)

	runCmd := func(args ...string) (string, int) {
		var out, errOut bytes.Buffer
		app := NewApp(strings.NewReader(""), &out, &errOut)
		code := app.Run(args)
		return out.String() + errOut.String(), code
	}

	// 1. keel --version
	out, code := runCmd("--version")
	if code != 0 || !strings.Contains(out, "keel version") {
		t.Fatalf("version failed: code=%d, out=%s", code, out)
	}

	// 2. keel init
	out, code = runCmd("init", "--name", "Checkout Revamp", "--desc", "Test checkout redesign")
	if code != 0 || !strings.Contains(out, "Initialized Keel project") {
		t.Fatalf("init failed: code=%d, out=%s", code, out)
	}

	// 3. keel goal create
	out, code = runCmd("goal", "create", "--id", "speed-checkout", "--title", "Cut checkout latency", "--outcome", "Reduce checkout time to under 2 seconds")
	if code != 0 || !strings.Contains(out, "Created goal") {
		t.Fatalf("goal create failed: code=%d, out=%s", code, out)
	}

	// 4. keel task create
	out, code = runCmd("task", "create", "--id", "task-auth", "--title", "Optimize Auth Token Verification", "--priority", "high")
	if code != 0 || !strings.Contains(out, "Created task") {
		t.Fatalf("task create 1 failed: code=%d, out=%s", code, out)
	}

	out, code = runCmd("task", "create", "--id", "task-payment", "--title", "Parallelize Stripe Gateway Call", "--depends", "task-auth", "--priority", "critical")
	if code != 0 || !strings.Contains(out, "Created task") {
		t.Fatalf("task create 2 failed: code=%d, out=%s", code, out)
	}

	// 5. keel task ready
	out, code = runCmd("task", "ready")
	if code != 0 || !strings.Contains(out, "task-auth") || strings.Contains(out, "task-payment") {
		t.Fatalf("task ready should only include task-auth, got:\n%s", out)
	}

	// 6. keel brief
	out, code = runCmd("brief")
	if code != 0 || !strings.Contains(out, "Keel Project Brief: Checkout Revamp") {
		t.Fatalf("brief failed: code=%d, out=%s", code, out)
	}

	// 7. keel task update
	out, code = runCmd("task", "update", "task-auth", "--status", "completed", "--result", "JWT verification cached in Redis")
	if code != 0 || !strings.Contains(out, "Updated task") {
		t.Fatalf("task update failed: code=%d, out=%s", code, out)
	}

	// Now task-payment should be ready!
	out, code = runCmd("task", "ready")
	if code != 0 || !strings.Contains(out, "task-payment") {
		t.Fatalf("task-payment should now be ready after dependency completed, got:\n%s", out)
	}

	// 8. keel worklog
	out, code = runCmd("worklog", "Cached auth tokens and verified benchmark", "--harness", "claude-code", "--next", "Parallelize Stripe call")
	if code != 0 || !strings.Contains(out, "recorded successfully") {
		t.Fatalf("worklog failed: code=%d, out=%s", code, out)
	}

	// 9. keel doctor
	out, code = runCmd("doctor")
	if code != 0 || !strings.Contains(out, "Project is healthy") {
		t.Fatalf("doctor failed: code=%d, out=%s", code, out)
	}

	// 10. Test task list --status ready & blocked
	out, code = runCmd("task", "list", "--status", "ready")
	if code != 0 || !strings.Contains(out, "task-payment") {
		t.Fatalf("expected task-payment in ready list, got:\n%s", out)
	}

	// 11. Test goal select, update, and delete
	out, code = runCmd("goal", "select", "speed-checkout")
	if code != 0 || !strings.Contains(out, "Current active goal set to") {
		t.Fatalf("goal select failed: code=%d, out=%s", code, out)
	}

	out, code = runCmd("goal", "update", "speed-checkout", "--status", "completed")
	if code != 0 || !strings.Contains(out, "Updated goal") {
		t.Fatalf("goal update failed: code=%d, out=%s", code, out)
	}

	out, code = runCmd("goal", "delete", "speed-checkout")
	if code != 0 || !strings.Contains(out, "Deleted goal") {
		t.Fatalf("goal delete failed: code=%d, out=%s", code, out)
	}

	// 12. Test task delete
	out, code = runCmd("task", "delete", "task-payment")
	if code != 0 || !strings.Contains(out, "Deleted task") {
		t.Fatalf("task delete failed: code=%d, out=%s", code, out)
	}

	// 13. Test goal list and worklog list
	out, code = runCmd("goal", "list")
	if code != 0 {
		t.Fatalf("goal list failed: code=%d, out=%s", code, out)
	}

	out, code = runCmd("goal", "list", "--json")
	if code != 0 || !strings.Contains(out, "[") {
		t.Fatalf("goal list --json failed: code=%d, out=%s", code, out)
	}

	out, code = runCmd("worklog", "--list")
	if code != 0 || !strings.Contains(out, "Cached auth tokens") {
		t.Fatalf("worklog --list failed: code=%d, out=%s", code, out)
	}

	out, code = runCmd("worklog", "--list", "--json")
	if code != 0 || !strings.Contains(out, "[") {
		t.Fatalf("worklog --list --json failed: code=%d, out=%s", code, out)
	}

	// 14. Test JSON output modes
	out, code = runCmd("task", "list", "--json")
	if code != 0 || !strings.Contains(out, "[") {
		t.Fatalf("task list --json failed: code=%d, out=%s", code, out)
	}

	out, code = runCmd("task", "ready", "--json")
	if code != 0 || !strings.Contains(out, "[") {
		t.Fatalf("task ready --json failed: code=%d, out=%s", code, out)
	}

	// 15. Test task unblock, clear-deps, and doctor missing dependencies detection
	out, code = runCmd("task", "create", "--id", "t-broken", "--title", "Broken Task", "--depends", "nonexistent-task", "--assignee", "claude", "--priority", "high")
	if code != 0 {
		t.Fatalf("task create broken failed: %s", out)
	}
	out, code = runCmd("doctor")
	if code == 0 || !strings.Contains(out, "Broken Task Dependencies Detected") {
		t.Fatalf("doctor should fail on missing dependency, got: code=%d, out=%s", code, out)
	}

	// Test task filtering by assignee and priority
	out, code = runCmd("task", "list", "--assignee", "claude", "--priority", "high")
	if code != 0 || !strings.Contains(out, "t-broken") {
		t.Fatalf("task list with assignee and priority failed: %s", out)
	}

	// Test task tree visualization
	out, code = runCmd("task", "tree")
	if code != 0 || !strings.Contains(out, "Task Dependency Tree") {
		t.Fatalf("task tree failed: %s", out)
	}

	// Test worklog list with pagination limit
	out, code = runCmd("worklog", "--list", "--limit", "1")
	if code != 0 || !strings.Contains(out, "Cached auth tokens") {
		t.Fatalf("worklog --list --limit failed: %s", out)
	}

	// Test doctor --fix auto-repair
	out, code = runCmd("doctor", "--fix")
	if code != 0 || !strings.Contains(out, "Auto-repairing broken task dependencies") {
		t.Fatalf("doctor --fix failed to repair: code=%d, out=%s", code, out)
	}
	runCmd("task", "delete", "t-broken")

	// 16. Test task list --goal filter
	out, code = runCmd("task", "list", "--goal", "speed-checkout")
	if code != 0 {
		t.Fatalf("task list --goal failed: %s", out)
	}

	// 17. Test doctor corrupted JSON detection
	corruptPath := "tasks/corrupt.json"
	_ = os.WriteFile(corruptPath, []byte("NOT_VALID_JSON{"), 0644)
	out, code = runCmd("doctor")
	if code == 0 || !strings.Contains(out, "Corrupted JSON files detected") {
		t.Fatalf("doctor should fail on corrupt JSON file, got: code=%d, out=%s", code, out)
	}
	_ = os.Remove(corruptPath)

	// 18. Test help and unknown command handling
	out, code = runCmd("help")
	if code != 0 || !strings.Contains(out, "CORE COMMANDS") {
		t.Fatalf("help failed: code=%d, out=%s", code, out)
	}

	out, code = runCmd("unknown-cmd")
	if code == 0 {
		t.Fatalf("unknown command should fail with non-zero exit code")
	}
}

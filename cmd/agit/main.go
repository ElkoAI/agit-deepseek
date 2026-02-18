package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strings"
    "time"

    "agit/internal/db"
    "agit/internal/exec"
    "agit/internal/git"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		cmdInit()
	case "status":
		cmdStatus()
	case "task":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: agit task <subcommand>\n")
			fmt.Fprintf(os.Stderr, "Subcommands: new, list, start, done, next\n")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "new":
			cmdTaskNew()
		case "list":
			cmdTaskList()
		case "start":
			cmdTaskStart()
		case "done":
			cmdTaskDone()
		case "next":
			cmdTaskNext()
		default:
			fmt.Fprintf(os.Stderr, "Unknown task subcommand: %s\n", os.Args[2])
			os.Exit(1)
		}
	case "checkpoint":
		cmdCheckpoint()
	case "restore":
		cmdRestore()
	case "hook":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: agit hook <subcommand>\n")
			fmt.Fprintf(os.Stderr, "Subcommands: install, uninstall\n")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "install":
			cmdHookInstall()
		case "uninstall":
			cmdHookUninstall()
		default:
			fmt.Fprintf(os.Stderr, "Unknown hook subcommand: %s\n", os.Args[2])
			os.Exit(1)
		}
	case "workspace":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: agit workspace <subcommand>\n")
			fmt.Fprintf(os.Stderr, "Subcommands: list, switch\n")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			cmdWorkspaceList()
		case "switch":
			cmdWorkspaceSwitch()
		default:
			fmt.Fprintf(os.Stderr, "Unknown workspace subcommand: %s\n", os.Args[2])
			os.Exit(1)
		}
	case "version":
		fmt.Println("agit version 0.1.0")
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `AGIT: Agent Git - Git-native state persistence for AI agents

Usage:
  agit <command> [arguments]

Commands:
  init                   Initialize agit database in current repository
  status                 Show current workspace status
  task new <description> Create a new task
  task list             List tasks
  task start <id>       Start a task
  task done <id>        Complete a task
  task next             Show next pending task
  checkpoint <message>  Create checkpoint with commit
  restore               Restore to last checkpoint
  hook install          Install git hooks
  hook uninstall        Remove agit git hooks
  workspace list        List workspaces
  workspace switch <id> Switch to a workspace
  version               Show version

Examples:
  agit init
  agit status
  agit task new "Implement feature X" --test "go test ./..."
  agit task list
  agit checkpoint "Before refactoring"
  agit restore --force
`)
}

func openDB() (*db.DB, error) {
	dbPath, err := db.GetDBPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get database path: %w", err)
	}
	return db.NewDB(dbPath)
}

func getCurrentWorkspace(ctx context.Context, database *db.DB) (*db.Workspace, error) {
	return database.GetOrCreateCurrentWorkspace(ctx)
}

func cmdInit() {
	// Check if we're in a git repository
	if _, err := git.FindGitRepo("."); err != nil {
		log.Fatalf("Not a git repository: %v", err)
	}

	// Get database path
	dbPath, err := db.GetDBPath()
	if err != nil {
		log.Fatalf("Failed to get database path: %v", err)
	}

	// Check if database already exists
	if _, err := os.Stat(dbPath); err == nil {
		log.Fatalf("Database already exists at %s", dbPath)
	}

	// Create database
	database, err := db.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	fmt.Printf("Initialized agit database at %s\n", dbPath)

	// Create initial workspace
	ctx := context.Background()
	ws, err := getCurrentWorkspace(ctx, database)
	if err != nil {
		log.Fatalf("Failed to create initial workspace: %v", err)
	}

	fmt.Printf("Created workspace: %s (branch: %s)\n", ws.WorkspaceID, ws.Branch)
}

func cmdStatus() {
	database, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	ws, err := getCurrentWorkspace(ctx, database)
	if err != nil {
		log.Fatal(err)
	}

	status, err := database.GetWorkspaceStatus(ctx, ws.WorkspaceID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Workspace: %s\n", ws.DisplayName.String)
	fmt.Printf("  ID: %s\n", ws.WorkspaceID)
	fmt.Printf("  Branch: %s\n", ws.Branch)
	if ws.WorktreePath.Valid {
		fmt.Printf("  Worktree: %s\n", ws.WorktreePath.String)
	}
	fmt.Printf("  Last commit: %s\n", ws.LastCommit[:8])
	fmt.Printf("  Updated: %s\n", ws.UpdatedAt.Format(time.RFC3339))

	if status.ActiveTask != nil {
		fmt.Printf("\nActive task: #%d - %s\n", status.ActiveTask.ID, status.ActiveTask.Description)
	}

	fmt.Printf("\nTask counts:\n")
	for st, count := range status.TaskCounts {
		fmt.Printf("  %-10s: %d\n", st, count)
	}

	if status.LastCheckpoint != nil {
		fmt.Printf("\nLast checkpoint: #%d at %s\n", status.LastCheckpoint.ID, status.LastCheckpoint.CommitHash[:8])
		fmt.Printf("  Message: %s\n", status.LastCheckpoint.Message)
		fmt.Printf("  Created: %s\n", status.LastCheckpoint.CreatedAt.Format(time.RFC3339))
	}

	// Check working tree status
	isDirty, err := git.IsWorkingTreeDirty()
	if err != nil {
		fmt.Printf("\nWarning: Failed to check working tree status: %v\n", err)
	} else if isDirty {
		fmt.Printf("\nWorking tree has uncommitted changes\n")
	}
}

func cmdTaskNew() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: agit task new <description> [--test <command>]\n")
		os.Exit(1)
	}

	description := os.Args[3]
	fs := flag.NewFlagSet("task new", flag.ExitOnError)
	testCmd := fs.String("test", "", "Test command to run when task is done")
	fs.Parse(os.Args[4:])

	database, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	ws, err := getCurrentWorkspace(ctx, database)
	if err != nil {
		log.Fatal(err)
	}

	var testCommand *string
	if *testCmd != "" {
		testCommand = testCmd
	}

	task, err := database.CreateTask(ctx, ws.WorkspaceID, description, testCommand, nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Created task #%d: %s\n", task.ID, task.Description)
	if testCommand != nil {
		fmt.Printf("Test command: %s\n", *testCommand)
	}
}

func cmdTaskList() {
	fs := flag.NewFlagSet("task list", flag.ExitOnError)
	statusFilter := fs.String("status", "", "Filter by status (pending, active, done, failed, blocked, skipped)")
	fs.Parse(os.Args[3:])

	database, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	ws, err := getCurrentWorkspace(ctx, database)
	if err != nil {
		log.Fatal(err)
	}

	var filter *db.TaskStatus
	if *statusFilter != "" {
		st := db.TaskStatus(*statusFilter)
		// Validate
		switch st {
		case db.TaskStatusPending, db.TaskStatusActive, db.TaskStatusDone,
			db.TaskStatusFailed, db.TaskStatusBlocked, db.TaskStatusSkipped:
			filter = &st
		default:
			log.Fatalf("Invalid status: %s", *statusFilter)
		}
	}

	tasks, err := database.ListTasks(ctx, ws.WorkspaceID, filter, 0, 0)
	if err != nil {
		log.Fatal(err)
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks found")
		return
	}

	fmt.Printf("Tasks for workspace %s:\n", ws.DisplayName.String)
	for _, task := range tasks {
		statusIcon := "○"
		switch task.Status {
		case db.TaskStatusActive:
			statusIcon = "▶"
		case db.TaskStatusDone:
			statusIcon = "✓"
		case db.TaskStatusFailed:
			statusIcon = "✗"
		case db.TaskStatusBlocked:
			statusIcon = "⛔"
		case db.TaskStatusSkipped:
			statusIcon = "⏭"
		}
		fmt.Printf("%s #%-4d %-10s %s\n", statusIcon, task.ID, task.Status, task.Description)
		if task.TestCommand.Valid {
			fmt.Printf("    Test: %s\n", task.TestCommand.String)
		}
		if !task.CompletedAt.Time.IsZero() {
			fmt.Printf("    Completed: %s\n", task.CompletedAt.Time.Format("2006-01-02 15:04"))
		}
	}
}

func cmdTaskStart() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: agit task start <task_id>\n")
		os.Exit(1)
	}

	var taskID int64
	if _, err := fmt.Sscanf(os.Args[3], "%d", &taskID); err != nil {
		log.Fatalf("Invalid task ID: %s", os.Args[3])
	}

	database, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	ws, err := getCurrentWorkspace(ctx, database)
	if err != nil {
		log.Fatal(err)
	}

	// Verify task belongs to workspace
	task, err := database.GetTaskByID(ctx, taskID)
	if err != nil {
		log.Fatal(err)
	}

	if task.WorkspaceID != ws.WorkspaceID {
		log.Fatalf("Task #%d does not belong to current workspace", taskID)
	}

	// Update status
	if err := database.UpdateTaskStatus(ctx, taskID, db.TaskStatusActive); err != nil {
		log.Fatal(err)
	}

	// Set as current task
	if err := database.SetCurrentTask(ctx, ws.WorkspaceID, taskID); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Started task #%d: %s\n", taskID, task.Description)
}

func cmdTaskDone() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: agit task done <task_id> [--skip-test]\n")
		os.Exit(1)
	}

	var taskID int64
	if _, err := fmt.Sscanf(os.Args[3], "%d", &taskID); err != nil {
		log.Fatalf("Invalid task ID: %s", os.Args[3])
	}

	fs := flag.NewFlagSet("task done", flag.ExitOnError)
	skipTest := fs.Bool("skip-test", false, "Skip running tests")
	fs.Parse(os.Args[4:])

	database, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	ws, err := getCurrentWorkspace(ctx, database)
	if err != nil {
		log.Fatal(err)
	}

	// Get task
	task, err := database.GetTaskByID(ctx, taskID)
	if err != nil {
		log.Fatal(err)
	}

	if task.WorkspaceID != ws.WorkspaceID {
		log.Fatalf("Task #%d does not belong to current workspace", taskID)
	}

	// Run test if specified and not skipped
	if task.TestCommand.Valid && !*skipTest {
		// Execute the test command
		ctxExec := context.Background()
		timeout := 5 * time.Minute // Default timeout of 5 minutes
		testResult, err := exec.RunTest(ctxExec, task.TestCommand.String, ".", timeout)
		if err != nil {
			log.Fatalf("Failed to execute test command: %v", err)
		}

		// Convert to database format
		dbTestResult := db.TestResult{
			ExitCode:   testResult.ExitCode,
			Output:     exec.TruncateOutput(testResult.Output, 1000),
			DurationMS: testResult.Duration.Milliseconds(),
			Passed:     testResult.Passed,
			Timestamp:  testResult.RunAt,
		}

		resultJSON, err := json.Marshal(dbTestResult)
		if err != nil {
			log.Fatal(err)
		}

		if err := database.UpdateTaskTestResult(ctx, taskID, resultJSON); err != nil {
			log.Fatal(err)
		}

		// Record trace
		outputTruncated := exec.TruncateOutput(testResult.Output, 1000)
		durationMS := testResult.Duration.Milliseconds()
		_, err = database.CreateTrace(ctx, taskID, task.TestCommand.String, &testResult.ExitCode,
			&outputTruncated, nil, &durationMS)
		if err != nil {
			log.Fatal(err)
		}

		status := "passed"
		if !testResult.Passed {
			status = "failed"
		}
		if testResult.TimedOut {
			status = "timed out"
		}
		fmt.Printf("Test for task #%d %s (exit code: %d, duration: %v)\n",
			taskID, status, testResult.ExitCode, testResult.Duration)
	}

	// Mark task as done
	if err := database.UpdateTaskStatus(ctx, taskID, db.TaskStatusDone); err != nil {
		log.Fatal(err)
	}

	// Clear current task if this was the active one
	if ws.CurrentTaskID.Valid && ws.CurrentTaskID.Int64 == taskID {
		if err := database.SetCurrentTask(ctx, ws.WorkspaceID, 0); err != nil {
			log.Fatal(err)
		}
	}

	fmt.Printf("Completed task #%d: %s\n", taskID, task.Description)
	if *skipTest && task.TestCommand.Valid {
		fmt.Println("Test skipped (--skip-test flag used)")
	}
}

func cmdTaskNext() {
	database, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	ws, err := getCurrentWorkspace(ctx, database)
	if err != nil {
		log.Fatal(err)
	}

	task, err := database.GetNextPendingTask(ctx, ws.WorkspaceID)
	if err != nil {
		log.Fatal(err)
	}

	if task == nil {
		fmt.Println("No pending tasks")
		return
	}

	fmt.Printf("Next task: #%d - %s\n", task.ID, task.Description)
	fmt.Printf("Priority: %d\n", task.Priority)
	if task.TestCommand.Valid {
		fmt.Printf("Test command: %s\n", task.TestCommand.String)
	}
	fmt.Printf("Created: %s\n", task.CreatedAt.Format("2006-01-02 15:04"))
}

func cmdCheckpoint() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: agit checkpoint <message> [--allow-dirty]\n")
		os.Exit(1)
	}

	message := os.Args[2]
	fs := flag.NewFlagSet("checkpoint", flag.ExitOnError)
	allowDirty := fs.Bool("allow-dirty", false, "Allow checkpoint with uncommitted changes")
	fs.Parse(os.Args[3:])

	database, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	ws, err := getCurrentWorkspace(ctx, database)
	if err != nil {
		log.Fatal(err)
	}

	// Check if working tree is dirty
	isDirty, err := git.IsWorkingTreeDirty()
	if err != nil {
		log.Fatal(err)
	}

	if isDirty && !*allowDirty {
		log.Fatal("Working tree has uncommitted changes. Use --allow-dirty to force checkpoint")
	}

	// Get current agent state
	agentState := ws.AgentState
	if len(agentState) == 0 {
		// Create minimal agent state
		minimalState := db.AgentState{
			Version:      "1.0",
			SessionNotes: "Checkpoint created via CLI",
			LastAction:   "checkpoint",
			AgentType:    "agit-cli",
		}
		var err error
		agentState, err = json.Marshal(minimalState)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Create git commit
	commitHash, err := git.CommitAll(message)
	if err != nil {
		log.Fatal(err)
	}

	// Create checkpoint
	checkpoint, err := database.CreateCheckpoint(ctx, ws.WorkspaceID, commitHash, agentState, message, !isDirty)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Created checkpoint #%d at commit %s: %s\n", checkpoint.ID, commitHash[:8], message)
	if isDirty {
		fmt.Println("Warning: Checkpoint created with uncommitted changes (--allow-dirty used)")
	}
}

func cmdRestore() {
	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	force := fs.Bool("force", false, "Force restore even with uncommitted changes")
	hard := fs.Bool("hard", false, "Perform hard reset (discard working tree changes)")
	fs.Parse(os.Args[2:])

	database, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	ws, err := getCurrentWorkspace(ctx, database)
	if err != nil {
		log.Fatal(err)
	}

	// Check if working tree is dirty
	isDirty, err := git.IsWorkingTreeDirty()
	if err != nil {
		log.Fatal(err)
	}

	if isDirty && !*force {
		log.Fatal("Working tree has uncommitted changes. Use --force to proceed")
	}

	// Get latest checkpoint
	checkpoint, err := database.GetLatestCheckpoint(ctx, ws.WorkspaceID)
	if err != nil {
		log.Fatal(err)
	}

	if checkpoint == nil {
		log.Fatal("No checkpoints found for this workspace")
	}

	// Perform git reset
	resetMode := "--mixed"
	if *hard {
		resetMode = "--hard"
	}

	if err := git.Reset(checkpoint.CommitHash, resetMode); err != nil {
		log.Fatal(err)
	}

	// Restore agent state
	if len(checkpoint.AgentState) > 0 {
		if err := database.UpdateWorkspaceAgentState(ctx, ws.WorkspaceID, checkpoint.AgentState); err != nil {
			log.Fatal(err)
		}
	}

	// Update workspace
	if _, err := getCurrentWorkspace(ctx, database); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Restored to checkpoint #%d at commit %s: %s\n", checkpoint.ID, checkpoint.CommitHash[:8], checkpoint.Message)
	if *hard {
		fmt.Println("Warning: Hard reset performed - working tree changes were discarded")
	}
}

func cmdHookInstall() {
	// Get git directory
	gitDir, err := git.GetGitDir()
	if err != nil {
		log.Fatal(err)
	}

	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		log.Fatal(err)
	}

	// Install post-checkout hook
	hookPath := filepath.Join(hooksDir, "post-checkout")
	hookContent := `#!/bin/bash
# AGIT post-checkout hook
previous_head=$1
new_head=$2
checkout_type=$3

# Call agit-mcp to update workspace
if command -v agit-mcp >/dev/null 2>&1; then
    branch=$(git rev-parse --abbrev-ref HEAD)
    commit=$(git rev-parse HEAD)
    echo "{\"branch\":\"$branch\",\"commit\":\"$commit\"}" | agit-mcp tool workspace_switch 2>/dev/null || true
fi
`

	if err := os.WriteFile(hookPath, []byte(hookContent), 0755); err != nil {
		log.Fatal(err)
	}

	// Install post-merge hook
	mergeHookPath := filepath.Join(hooksDir, "post-merge")
	mergeHookContent := `#!/bin/bash
# AGIT post-merge hook
if command -v agit-mcp >/dev/null 2>&1; then
    agit-mcp tool workspace_sync 2>/dev/null || true
fi
`

	if err := os.WriteFile(mergeHookPath, []byte(mergeHookContent), 0755); err != nil {
		log.Fatal(err)
	}

	// Install post-commit hook
	commitHookPath := filepath.Join(hooksDir, "post-commit")
	commitHookContent := `#!/bin/bash
# AGIT post-commit hook
if command -v agit-mcp >/dev/null 2>&1; then
    commit=$(git rev-parse HEAD)
    agit-mcp tool workspace_commit --hash "$commit" 2>/dev/null || true
fi
`

	if err := os.WriteFile(commitHookPath, []byte(commitHookContent), 0755); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Installed agit git hooks:")
	fmt.Printf("  %s\n", hookPath)
	fmt.Printf("  %s\n", mergeHookPath)
	fmt.Printf("  %s\n", commitHookPath)
	fmt.Println("\nNote: agit-mcp must be in your PATH for hooks to work")
}

func cmdHookUninstall() {
	gitDir, err := git.GetGitDir()
	if err != nil {
		log.Fatal(err)
	}

	hooks := []string{"post-checkout", "post-merge", "post-commit"}
	removed := 0

	for _, hookName := range hooks {
		hookPath := filepath.Join(gitDir, "hooks", hookName)
		content, err := os.ReadFile(hookPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			log.Fatal(err)
		}

		// Only remove if it's an agit hook
		if strings.Contains(string(content), "# AGIT") {
			if err := os.Remove(hookPath); err != nil {
				log.Fatal(err)
			}
			fmt.Printf("Removed %s\n", hookPath)
			removed++
		}
	}

	if removed == 0 {
		fmt.Println("No agit hooks found")
	} else {
		fmt.Printf("Removed %d agit hook(s)\n", removed)
	}
}

func cmdWorkspaceList() {
	database, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()

	// Get current workspace first
	currentWs, err := getCurrentWorkspace(ctx, database)
	if err != nil {
		log.Fatal(err)
	}

	// List all workspaces (simplified - we don't have a ListWorkspaces method yet)
	// For now, just show current workspace
	fmt.Printf("Current workspace:\n")
	fmt.Printf("  ID: %s\n", currentWs.WorkspaceID)
	fmt.Printf("  Branch: %s\n", currentWs.Branch)
	if currentWs.WorktreePath.Valid {
		fmt.Printf("  Worktree: %s\n", currentWs.WorktreePath.String)
	}
	fmt.Printf("  Display name: %s\n", currentWs.DisplayName.String)
	fmt.Printf("  Last commit: %s\n", currentWs.LastCommit[:8])
	fmt.Printf("  Created: %s\n", currentWs.CreatedAt.Format("2006-01-02 15:04"))

	// TODO: Implement proper workspace listing when we add that method
	fmt.Println("\nNote: Full workspace listing not yet implemented")
}

func cmdWorkspaceSwitch() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: agit workspace switch <workspace_id>\n")
		os.Exit(1)
	}

	workspaceID := os.Args[3]

	database, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()

	// Get workspace
	ws, err := database.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		log.Fatal(err)
	}

	// Checkout the branch
	// TODO: Implement git checkout to the workspace branch
	// For now, just show info
	fmt.Printf("Workspace: %s\n", ws.DisplayName.String)
	fmt.Printf("  Branch: %s\n", ws.Branch)
	fmt.Printf("  To switch to this workspace, run: git checkout %s\n", ws.Branch)
	fmt.Println("\nNote: Automatic branch switching not yet implemented")
}

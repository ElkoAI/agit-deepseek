package db

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) (*DB, string, func()) {
	t.Helper()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "agit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Change to temp directory for git operations
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current dir: %v", err)
	}

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to change to temp dir: %v", err)
	}

	// Initialize a proper git repository
	cmd := exec.Command("git", "init")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to initialize git repository: %v\nOutput: %s", err, output)
	}

	// Configure git to avoid warnings
	exec.Command("git", "config", "user.email", "test@agit.com").Run()
	exec.Command("git", "config", "user.name", "Test User").Run()

	// Create an initial commit
	readmePath := filepath.Join(tempDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Repository\n"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	exec.Command("git", "add", "README.md").Run()
	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create initial commit: %v\nOutput: %s", err, output)
	}

	// Define git directory path
	gitDir := filepath.Join(tempDir, ".git")

	// Create database path
	dbPath := filepath.Join(gitDir, "agit-test.db")

	// Open database
	db, err := NewDB(dbPath)
	if err != nil {
		os.Chdir(oldDir)
		os.RemoveAll(tempDir)
		t.Fatalf("failed to create test database: %v", err)
	}

	// Cleanup function
	cleanup := func() {
		db.Close()
		os.Chdir(oldDir)
		os.RemoveAll(tempDir)
	}

	return db, tempDir, cleanup
}

// TestNewDB tests database creation and migration
func TestNewDB(t *testing.T) {
	db, _, cleanup := setupTestDB(t)
	defer cleanup()

	// Check that schema_meta table exists and has version 1
	var version int
	var description string
	err := db.conn.QueryRow("SELECT version, description FROM schema_meta ORDER BY version DESC LIMIT 1").Scan(&version, &description)
	if err != nil {
		t.Errorf("failed to query schema_meta: %v", err)
	}

	if version != 1 {
		t.Errorf("expected schema version 1, got %d", version)
	}

	if description != "Initial schema with workspace_id" {
		t.Errorf("unexpected schema description: %s", description)
	}

	// Check that workspaces table exists
	var tableCount int
	err = db.conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='workspaces'").Scan(&tableCount)
	if err != nil {
		t.Errorf("failed to check workspaces table: %v", err)
	}

	if tableCount != 1 {
		t.Errorf("expected workspaces table to exist, count: %d", tableCount)
	}
}

// TestGetOrCreateCurrentWorkspace tests workspace creation and retrieval
func TestGetOrCreateCurrentWorkspace(t *testing.T) {
	db, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a workspace
	workspace, err := db.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	if workspace.WorkspaceID == "" {
		t.Error("workspace ID should not be empty")
	}

	if workspace.Branch == "" {
		t.Error("branch should not be empty")
	}

	if workspace.LastCommit == "" {
		t.Error("last commit should not be empty")
	}

	// Get the same workspace again (should return existing one)
	workspace2, err := db.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		t.Fatalf("failed to get workspace: %v", err)
	}

	if workspace.WorkspaceID != workspace2.WorkspaceID {
		t.Errorf("workspace IDs don't match: %s != %s", workspace.WorkspaceID, workspace2.WorkspaceID)
	}

	// Verify workspace can be retrieved by ID
	retrieved, err := db.GetWorkspaceByID(ctx, workspace.WorkspaceID)
	if err != nil {
		t.Fatalf("failed to get workspace by ID: %v", err)
	}

	if retrieved.WorkspaceID != workspace.WorkspaceID {
		t.Errorf("retrieved workspace ID doesn't match: %s != %s", retrieved.WorkspaceID, workspace.WorkspaceID)
	}
}

// TestCreateAndGetTask tests task creation and retrieval
func TestCreateAndGetTask(t *testing.T) {
	db, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a workspace first
	workspace, err := db.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	// Test task without test command
	task, err := db.CreateTask(ctx, workspace.WorkspaceID, "Test task without test", nil, nil)
	if err != nil {
		t.Fatalf("failed to create task without test: %v", err)
	}

	if task.ID <= 0 {
		t.Errorf("invalid task ID: %d", task.ID)
	}

	// Retrieve the task
	retrievedTask, err := db.GetTaskByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("failed to get task by ID: %v", err)
	}

	if retrievedTask.ID != task.ID {
		t.Errorf("task ID mismatch: expected %d, got %d", task.ID, retrievedTask.ID)
	}

	if retrievedTask.Description != "Test task without test" {
		t.Errorf("task description mismatch: expected 'Test task without test', got '%s'", retrievedTask.Description)
	}

	if retrievedTask.Status != TaskStatusPending {
		t.Errorf("task status should be pending, got: %s", retrievedTask.Status)
	}

	// Test task with test command
	testCmd := "go test ./..."
	task2, err := db.CreateTask(ctx, workspace.WorkspaceID, "Test task with test", &testCmd, nil)
	if err != nil {
		t.Fatalf("failed to create task with test: %v", err)
	}

	retrievedTask2, err := db.GetTaskByID(ctx, task2.ID)
	if err != nil {
		t.Fatalf("failed to get task with test by ID: %v", err)
	}

	if !retrievedTask2.TestCommand.Valid {
		t.Error("test command should be valid")
	}

	if retrievedTask2.TestCommand.String != testCmd {
		t.Errorf("test command mismatch: expected '%s', got '%s'", testCmd, retrievedTask2.TestCommand.String)
	}
}

// TestUpdateTaskStatus tests task status updates
func TestUpdateTaskStatus(t *testing.T) {
	db, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create workspace and task
	workspace, err := db.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	task, err := db.CreateTask(ctx, workspace.WorkspaceID, "Test task for status updates", nil, nil)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Update task to active
	err = db.UpdateTaskStatus(ctx, task.ID, TaskStatusActive)
	if err != nil {
		t.Fatalf("failed to update task status to active: %v", err)
	}

	retrievedTask, err := db.GetTaskByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("failed to get task after status update: %v", err)
	}

	if retrievedTask.Status != TaskStatusActive {
		t.Errorf("task status should be active, got: %s", retrievedTask.Status)
	}

	// Update task to done
	err = db.UpdateTaskStatus(ctx, task.ID, TaskStatusDone)
	if err != nil {
		t.Fatalf("failed to update task status to done: %v", err)
	}

	retrievedTask, err = db.GetTaskByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("failed to get task after second status update: %v", err)
	}

	if retrievedTask.Status != TaskStatusDone {
		t.Errorf("task status should be done, got: %s", retrievedTask.Status)
	}
}

// TestListTasks tests task listing
func TestListTasks(t *testing.T) {
	db, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create workspace
	workspace, err := db.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	// Create multiple tasks with different statuses
	taskIDs := []int64{}
	descriptions := []string{
		"Pending task 1",
		"Pending task 2",
		"Active task",
		"Done task",
	}

	for i, desc := range descriptions {
		task, err := db.CreateTask(ctx, workspace.WorkspaceID, desc, nil, nil)
		if err != nil {
			t.Fatalf("failed to create task %d: %v", i, err)
		}
		taskIDs = append(taskIDs, task.ID)
	}

	// Update some tasks to different statuses
	err = db.UpdateTaskStatus(ctx, taskIDs[2], TaskStatusActive)
	if err != nil {
		t.Fatalf("failed to update task to active: %v", err)
	}

	err = db.UpdateTaskStatus(ctx, taskIDs[3], TaskStatusDone)
	if err != nil {
		t.Fatalf("failed to update task to done: %v", err)
	}

	// List all tasks
	tasks, err := db.ListTasks(ctx, workspace.WorkspaceID, nil, 0, 0)
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}

	if len(tasks) != 4 {
		t.Errorf("expected 4 tasks, got %d", len(tasks))
	}

	// List pending tasks only
	pendingFilter := TaskStatusPending
	pendingTasks, err := db.ListTasks(ctx, workspace.WorkspaceID, &pendingFilter, 0, 0)
	if err != nil {
		t.Fatalf("failed to list pending tasks: %v", err)
	}

	if len(pendingTasks) != 2 {
		t.Errorf("expected 2 pending tasks, got %d", len(pendingTasks))
	}

	// List with limit
	limitedTasks, err := db.ListTasks(ctx, workspace.WorkspaceID, nil, 2, 0)
	if err != nil {
		t.Fatalf("failed to list limited tasks: %v", err)
	}

	if len(limitedTasks) != 2 {
		t.Errorf("expected 2 tasks with limit, got %d", len(limitedTasks))
	}
}

// TestCreateAndGetCheckpoint tests checkpoint operations
func TestCreateAndGetCheckpoint(t *testing.T) {
	db, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create workspace
	workspace, err := db.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	// Create checkpoint
	commitHash := "abc123def456"
	message := "Test checkpoint"
	agentState := []byte(`{"version": "1.0", "session_notes": "Test"}`)

	checkpoint, err := db.CreateCheckpoint(ctx, workspace.WorkspaceID, commitHash, agentState, message, true)
	if err != nil {
		t.Fatalf("failed to create checkpoint: %v", err)
	}

	if checkpoint.ID <= 0 {
		t.Errorf("invalid checkpoint ID: %d", checkpoint.ID)
	}

	if checkpoint.CommitHash != commitHash {
		t.Errorf("commit hash mismatch: expected '%s', got '%s'", commitHash, checkpoint.CommitHash)
	}

	if checkpoint.Message != message {
		t.Errorf("message mismatch: expected '%s', got '%s'", message, checkpoint.Message)
	}

	if !checkpoint.WasClean {
		t.Error("checkpoint should be marked as clean")
	}

	// Get checkpoint by ID
	retrieved, err := db.GetCheckpointByID(ctx, checkpoint.ID)
	if err != nil {
		t.Fatalf("failed to get checkpoint by ID: %v", err)
	}

	if retrieved.ID != checkpoint.ID {
		t.Errorf("checkpoint ID mismatch: expected %d, got %d", checkpoint.ID, retrieved.ID)
	}

	// Get latest checkpoint
	latest, err := db.GetLatestCheckpoint(ctx, workspace.WorkspaceID)
	if err != nil {
		t.Fatalf("failed to get latest checkpoint: %v", err)
	}

	if latest.ID != checkpoint.ID {
		t.Errorf("latest checkpoint ID mismatch: expected %d, got %d", checkpoint.ID, latest.ID)
	}
}

// TestCreateTrace tests trace creation
func TestCreateTrace(t *testing.T) {
	db, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create workspace and task
	workspace, err := db.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	task, err := db.CreateTask(ctx, workspace.WorkspaceID, "Test task for trace", nil, nil)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Create trace
	command := "go test ./..."
	exitCode := 0
	output := "All tests passed"
	logPath := "/tmp/test.log"
	durationMS := int64(1500)

	trace, err := db.CreateTrace(ctx, task.ID, command, &exitCode, &output, &logPath, &durationMS)
	if err != nil {
		t.Fatalf("failed to create trace: %v", err)
	}

	if trace.ID <= 0 {
		t.Errorf("invalid trace ID: %d", trace.ID)
	}

	// List traces
	traces, err := db.ListTraces(ctx, task.ID, 0, 0)
	if err != nil {
		t.Fatalf("failed to list traces: %v", err)
	}

	if len(traces) != 1 {
		t.Errorf("expected 1 trace, got %d", len(traces))
	}

	traceRetrieved := traces[0]
	if traceRetrieved.ID != trace.ID {
		t.Errorf("trace ID mismatch: expected %d, got %d", trace.ID, traceRetrieved.ID)
	}

	if traceRetrieved.Command != command {
		t.Errorf("command mismatch: expected '%s', got '%s'", command, traceRetrieved.Command)
	}

	if !traceRetrieved.ExitCode.Valid || traceRetrieved.ExitCode.Int64 != int64(exitCode) {
		t.Errorf("exit code mismatch: expected %d, got %v", exitCode, traceRetrieved.ExitCode)
	}

	if !traceRetrieved.OutputTruncated.Valid || traceRetrieved.OutputTruncated.String != output {
		t.Errorf("output mismatch: expected '%s', got '%s'", output, traceRetrieved.OutputTruncated.String)
	}

	if !traceRetrieved.LogPath.Valid || traceRetrieved.LogPath.String != logPath {
		t.Errorf("log path mismatch: expected '%s', got '%s'", logPath, traceRetrieved.LogPath.String)
	}

	if !traceRetrieved.DurationMS.Valid || traceRetrieved.DurationMS.Int64 != durationMS {
		t.Errorf("duration mismatch: expected %d, got %v", durationMS, traceRetrieved.DurationMS)
	}
}

// TestGetWorkspaceStatus tests workspace status retrieval
func TestGetWorkspaceStatus(t *testing.T) {
	db, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create workspace
	workspace, err := db.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	// Create some tasks
	testCmd := "go test ./..."
	taskIDs := []int64{}
	for i := 0; i < 5; i++ {
		var cmd *string
		if i%2 == 0 { // Even tasks have test commands
			cmd = &testCmd
		}

		task, err := db.CreateTask(ctx, workspace.WorkspaceID, fmt.Sprintf("Task %d", i+1), cmd, nil)
		if err != nil {
			t.Fatalf("failed to create task %d: %v", i, err)
		}
		taskIDs = append(taskIDs, task.ID)
	}

	// Update some task statuses
	err = db.UpdateTaskStatus(ctx, taskIDs[0], TaskStatusActive)
	if err != nil {
		t.Fatalf("failed to update task to active: %v", err)
	}

	err = db.UpdateTaskStatus(ctx, taskIDs[1], TaskStatusDone)
	if err != nil {
		t.Fatalf("failed to update task to done: %v", err)
	}

	err = db.UpdateTaskStatus(ctx, taskIDs[2], TaskStatusFailed)
	if err != nil {
		t.Fatalf("failed to update task to failed: %v", err)
	}

	// Set current task
	err = db.SetCurrentTask(ctx, workspace.WorkspaceID, taskIDs[0])
	if err != nil {
		t.Fatalf("failed to set current task: %v", err)
	}

	// Create a checkpoint
	commitHash := "test123"
	message := "Test checkpoint for status"
	agentState := []byte(`{"version": "1.0"}`)

	_, err = db.CreateCheckpoint(ctx, workspace.WorkspaceID, commitHash, agentState, message, true)
	if err != nil {
		t.Fatalf("failed to create checkpoint: %v", err)
	}

	// Get workspace status
	status, err := db.GetWorkspaceStatus(ctx, workspace.WorkspaceID)
	if err != nil {
		t.Fatalf("failed to get workspace status: %v", err)
	}

	if status.Workspace == nil {
		t.Error("workspace should not be nil in status")
	}

	if status.Workspace.WorkspaceID != workspace.WorkspaceID {
		t.Errorf("workspace ID mismatch in status")
	}

	if status.ActiveTask == nil {
		t.Error("active task should not be nil")
	}

	if status.ActiveTask.ID != taskIDs[0] {
		t.Errorf("active task ID mismatch: expected %d, got %d", taskIDs[0], status.ActiveTask.ID)
	}

	// Check task counts
	expectedCounts := map[TaskStatus]int{
		TaskStatusPending: 2,
		TaskStatusActive:  1,
		TaskStatusDone:    1,
		TaskStatusFailed:  1,
	}

	for statusType, expectedCount := range expectedCounts {
		actualCount, ok := status.TaskCounts[statusType]
		if !ok {
			t.Errorf("task count for status %s not found", statusType)
			continue
		}

		if actualCount != expectedCount {
			t.Errorf("task count mismatch for %s: expected %d, got %d", statusType, expectedCount, actualCount)
		}
	}

	// Verify blocked and skipped are not in the map (no tasks with those statuses)
	if _, ok := status.TaskCounts[TaskStatusBlocked]; ok {
		t.Error("blocked status should not be in task counts map")
	}
	if _, ok := status.TaskCounts[TaskStatusSkipped]; ok {
		t.Error("skipped status should not be in task counts map")
	}

	if status.LastCheckpoint == nil {
		t.Error("last checkpoint should not be nil")
	}
}

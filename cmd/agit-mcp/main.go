package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agit/internal/db"
	"agit/internal/exec"
	"agit/internal/git"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Get database path
	dbPath, err := db.GetDBPath()
	if err != nil {
		log.Fatalf("Failed to get database path: %v", err)
	}

	// Open database
	database, err := db.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Create server
	srv := server.NewDefaultServer("agit", "1.0.0")

	// Define tools
	tools := []mcp.Tool{
		{
			Name:        "workspace_status",
			Description: "Get current workspace state",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					// No parameters for this tool
				},
			},
		},
		{
			Name:        "task_new",
			Description: "Create new task",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Description of the task",
					},
					"test_command": map[string]interface{}{
						"type":        "string",
						"description": "Optional test command to run when task is done",
					},
				},

			},
		},
		{
			Name:        "task_list",
			Description: "List tasks for current workspace",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"status": map[string]interface{}{
						"type":        "string",
						"description": "Filter by status (pending, active, done, failed, blocked, skipped)",
						"enum": []string{
							"pending", "active", "done", "failed", "blocked", "skipped",
						},
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of tasks to return",
					},
				},
			},
		},
		{
			Name:        "task_start",
			Description: "Mark task as active",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id": map[string]interface{}{
						"type":        "integer",
						"description": "ID of the task to start",
					},
				},

			},
		},
		{
			Name:        "task_done",
			Description: "Complete task and optionally run tests",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id": map[string]interface{}{
						"type":        "integer",
						"description": "ID of the task to complete",
					},
					"skip_test": map[string]interface{}{
						"type":        "boolean",
						"description": "Skip running tests even if test command is defined",
					},
				},

			},
		},
		{
			Name:        "checkpoint",
			Description: "Save state + git commit",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"message": map[string]interface{}{
						"type":        "string",
						"description": "Commit message for the checkpoint",
					},
					"allow_dirty": map[string]interface{}{
						"type":        "boolean",
						"description": "Allow checkpoint even with uncommitted changes",
					},
				},

			},
		},
		{
			Name:        "restore",
			Description: "Restore to last checkpoint",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"force": map[string]interface{}{
						"type":        "boolean",
						"description": "Force restore even with uncommitted changes",
					},
					"hard": map[string]interface{}{
						"type":        "boolean",
						"description": "Perform hard reset (discard working tree changes)",
					},
				},
			},
		},
		{
			Name:        "trace",
			Description: "Show test history for a task",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id": map[string]interface{}{
						"type":        "integer",
						"description": "ID of the task to show traces for",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of traces to return",
					},
				},

			},
		},
	}

	// Handle list tools
	srv.HandleListTools(func(ctx context.Context, cursor *string) (*mcp.ListToolsResult, error) {
		return &mcp.ListToolsResult{
			Tools: tools,
		}, nil
	})

	// Handle call tool
	srv.HandleCallTool(func(ctx context.Context, name string, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
		switch name {
		case "workspace_status":
			return handleWorkspaceStatus(ctx, database)
		case "task_new":
			return handleTaskNew(ctx, database, arguments)
		case "task_list":
			return handleTaskList(ctx, database, arguments)
		case "task_start":
			return handleTaskStart(ctx, database, arguments)
		case "task_done":
			return handleTaskDone(ctx, database, arguments)
		case "checkpoint":
			return handleCheckpoint(ctx, database, arguments)
		case "restore":
			return handleRestore(ctx, database, arguments)
		case "trace":
			return handleTrace(ctx, database, arguments)
		default:
			return nil, fmt.Errorf("unknown tool: %s", name)
		}
	})

	// Handle graceful shutdown
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		cancel()
	}()

	// Run server
	if err := server.ServeStdio(srv); err != nil {
		log.Fatal(err)
	}
}

func handleWorkspaceStatus(ctx context.Context, database *db.DB) (*mcp.CallToolResult, error) {
	ws, err := database.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	status, err := database.GetWorkspaceStatus(ctx, ws.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace status: %w", err)
	}

	content, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal workspace status: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: string(content)},
		},
	}, nil
}

func handleTaskNew(ctx context.Context, database *db.DB, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	ws, err := database.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	description, ok := arguments["description"].(string)
	if !ok {
		return nil, fmt.Errorf("description argument is required and must be a string")
	}

	var testCommand *string
	if tc, ok := arguments["test_command"].(string); ok && tc != "" {
		testCommand = &tc
	}

	task, err := database.CreateTask(ctx, ws.WorkspaceID, description, testCommand, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	content, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: fmt.Sprintf("Created task #%d: %s\n%s", task.ID, task.Description, string(content))},
		},
	}, nil
}

func handleTaskList(ctx context.Context, database *db.DB, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	ws, err := database.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	var statusFilter *db.TaskStatus
	if statusStr, ok := arguments["status"].(string); ok && statusStr != "" {
		status := db.TaskStatus(statusStr)
		switch status {
		case db.TaskStatusPending, db.TaskStatusActive, db.TaskStatusDone,
			db.TaskStatusFailed, db.TaskStatusBlocked, db.TaskStatusSkipped:
			statusFilter = &status
		default:
			return nil, fmt.Errorf("invalid status: %s", statusStr)
		}
	}

	limit := 0
	if limitVal, ok := arguments["limit"].(float64); ok {
		limit = int(limitVal)
	}

	tasks, err := database.ListTasks(ctx, ws.WorkspaceID, statusFilter, limit, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	if len(tasks) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: "No tasks found"},
			},
		}, nil
	}

	content, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tasks: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: fmt.Sprintf("Found %d task(s):\n%s", len(tasks), string(content))},
		},
	}, nil
}

func handleTaskStart(ctx context.Context, database *db.DB, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	taskID, ok := arguments["task_id"].(float64)
	if !ok {
		return nil, fmt.Errorf("task_id argument is required and must be a number")
	}

	ws, err := database.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	task, err := database.GetTaskByID(ctx, int64(taskID))
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	if task.WorkspaceID != ws.WorkspaceID {
		return nil, fmt.Errorf("task %d does not belong to current workspace", int64(taskID))
	}

	if err := database.UpdateTaskStatus(ctx, int64(taskID), db.TaskStatusActive); err != nil {
		return nil, fmt.Errorf("failed to start task: %w", err)
	}

	if err := database.SetCurrentTask(ctx, ws.WorkspaceID, int64(taskID)); err != nil {
		return nil, fmt.Errorf("failed to set current task: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: fmt.Sprintf("Started task #%d: %s", int64(taskID), task.Description)},
		},
	}, nil
}

func handleTaskDone(ctx context.Context, database *db.DB, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	taskID, ok := arguments["task_id"].(float64)
	if !ok {
		return nil, fmt.Errorf("task_id argument is required and must be a number")
	}

	skipTest := false
	if skipVal, ok := arguments["skip_test"].(bool); ok {
		skipTest = skipVal
	}

	task, err := database.GetTaskByID(ctx, int64(taskID))
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	ws, err := database.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	if task.WorkspaceID != ws.WorkspaceID {
		return nil, fmt.Errorf("task %d does not belong to current workspace", int64(taskID))
	}

	var testResult *db.TestResult
	if task.TestCommand.Valid && !skipTest {
		// Execute the test command
		timeout := 5 * time.Minute // Default timeout of 5 minutes
		execResult, err := exec.RunTest(ctx, task.TestCommand.String, ".", timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to execute test command: %w", err)
		}

		// Convert to database format
		testResult = &db.TestResult{
			ExitCode:   execResult.ExitCode,
			Output:     exec.TruncateOutput(execResult.Output, 1000),
			DurationMS: execResult.Duration.Milliseconds(),
			Passed:     execResult.Passed,
			Timestamp:  execResult.RunAt,
		}

		resultJSON, err := json.Marshal(testResult)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal test result: %w", err)
		}

		if err := database.UpdateTaskTestResult(ctx, int64(taskID), resultJSON); err != nil {
			return nil, fmt.Errorf("failed to update test result: %w", err)
		}

		// Record trace
		outputTruncated := exec.TruncateOutput(execResult.Output, 1000)
		durationMS := execResult.Duration.Milliseconds()
		_, err = database.CreateTrace(ctx, int64(taskID), task.TestCommand.String, &execResult.ExitCode,
			&outputTruncated, nil, &durationMS)
		if err != nil {
			return nil, fmt.Errorf("failed to record trace: %w", err)
		}
	}

	if err := database.UpdateTaskStatus(ctx, int64(taskID), db.TaskStatusDone); err != nil {
		return nil, fmt.Errorf("failed to mark task as done: %w", err)
	}

	if ws.CurrentTaskID.Valid && ws.CurrentTaskID.Int64 == int64(taskID) {
		if err := database.SetCurrentTask(ctx, ws.WorkspaceID, 0); err != nil {
			return nil, fmt.Errorf("failed to clear current task: %w", err)
		}
	}

	msg := fmt.Sprintf("Completed task #%d: %s", int64(taskID), task.Description)
	if testResult != nil {
		status := "passed"
		if !testResult.Passed {
			status = "failed"
		}
		// Check for timeout (exit code -1)
		if testResult.ExitCode == -1 {
			status = "timed out"
		}
		duration := time.Duration(testResult.DurationMS) * time.Millisecond
		msg += fmt.Sprintf("\nTest %s (exit code: %d, duration: %v)", status, testResult.ExitCode, duration)
	} else if skipTest && task.TestCommand.Valid {
		msg += "\nTest skipped (skip_test=true)"
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: msg},
		},
	}, nil
}

func handleCheckpoint(ctx context.Context, database *db.DB, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	message, ok := arguments["message"].(string)
	if !ok || message == "" {
		return nil, fmt.Errorf("message argument is required and must be a non-empty string")
	}

	allowDirty := false
	if allowVal, ok := arguments["allow_dirty"].(bool); ok {
		allowDirty = allowVal
	}

	ws, err := database.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	isDirty, err := git.IsWorkingTreeDirty()
	if err != nil {
		return nil, fmt.Errorf("failed to check working tree status: %w", err)
	}

	if isDirty && !allowDirty {
		return nil, fmt.Errorf("working tree has uncommitted changes. Use allow_dirty=true to proceed")
	}

	agentState := ws.AgentState
	if len(agentState) == 0 {
		minimalState := db.AgentState{
			Version:      "1.0",
			SessionNotes: "Checkpoint created",
			LastAction:   "checkpoint",
			AgentType:    "agit",
		}
		agentState, err = json.Marshal(minimalState)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal minimal agent state: %w", err)
		}
	}

	commitHash, err := git.CommitAll(message)
	if err != nil {
		return nil, fmt.Errorf("failed to create commit: %w", err)
	}

	checkpoint, err := database.CreateCheckpoint(ctx, ws.WorkspaceID, commitHash, agentState, message, !isDirty)
	if err != nil {
		return nil, fmt.Errorf("failed to create checkpoint: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: fmt.Sprintf("Created checkpoint #%d at commit %s: %s", checkpoint.ID, commitHash[:8], message)},
		},
	}, nil
}

func handleRestore(ctx context.Context, database *db.DB, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	force := false
	if forceVal, ok := arguments["force"].(bool); ok {
		force = forceVal
	}

	hard := false
	if hardVal, ok := arguments["hard"].(bool); ok {
		hard = hardVal
	}

	ws, err := database.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	isDirty, err := git.IsWorkingTreeDirty()
	if err != nil {
		return nil, fmt.Errorf("failed to check working tree status: %w", err)
	}

	if isDirty && !force {
		return nil, fmt.Errorf("working tree has uncommitted changes. Use force=true to proceed")
	}

	checkpoint, err := database.GetLatestCheckpoint(ctx, ws.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest checkpoint: %w", err)
	}

	if checkpoint == nil {
		return nil, fmt.Errorf("no checkpoints found for this workspace")
	}

	resetMode := "--mixed"
	if hard {
		resetMode = "--hard"
	}

	if err := git.Reset(checkpoint.CommitHash, resetMode); err != nil {
		return nil, fmt.Errorf("failed to reset to checkpoint: %w", err)
	}

	if len(checkpoint.AgentState) > 0 {
		if err := database.UpdateWorkspaceAgentState(ctx, ws.WorkspaceID, checkpoint.AgentState); err != nil {
			return nil, fmt.Errorf("failed to restore agent state: %w", err)
		}
	}

	if _, err := database.GetOrCreateCurrentWorkspace(ctx); err != nil {
		return nil, fmt.Errorf("failed to update workspace after restore: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: fmt.Sprintf("Restored to checkpoint #%d at commit %s: %s", checkpoint.ID, checkpoint.CommitHash[:8], checkpoint.Message)},
		},
	}, nil
}

func handleTrace(ctx context.Context, database *db.DB, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	taskID, ok := arguments["task_id"].(float64)
	if !ok {
		return nil, fmt.Errorf("task_id argument is required and must be a number")
	}

	limit := 0
	if limitVal, ok := arguments["limit"].(float64); ok {
		limit = int(limitVal)
	}

	ws, err := database.GetOrCreateCurrentWorkspace(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	task, err := database.GetTaskByID(ctx, int64(taskID))
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	if task.WorkspaceID != ws.WorkspaceID {
		return nil, fmt.Errorf("task %d does not belong to current workspace", int64(taskID))
	}

	traces, err := database.ListTraces(ctx, int64(taskID), limit, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list traces: %w", err)
	}

	if len(traces) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: fmt.Sprintf("No traces found for task #%d", int64(taskID))},
			},
		}, nil
	}

	content, err := json.MarshalIndent(traces, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal traces: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: fmt.Sprintf("Found %d trace(s) for task #%d:\n%s", len(traces), int64(taskID), string(content))},
		},
	}, nil
}

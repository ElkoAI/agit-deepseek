package db

import (
    "context"
    "database/sql"
    "fmt"
    "agit/internal/git"

    "time"
)

// GetOrCreateCurrentWorkspace gets or creates a workspace for the current Git branch and worktree
func (db *DB) GetOrCreateCurrentWorkspace(ctx context.Context) (*Workspace, error) {
	branch, err := getCurrentGitBranch()
	if err != nil {
		return nil, fmt.Errorf("failed to get current git branch: %w", err)
	}

	worktreePath, err := getCurrentWorktreePath()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree path: %w", err)
	}

	commitHash, err := getCurrentCommitHash()
	if err != nil {
		return nil, fmt.Errorf("failed to get current commit hash: %w", err)
	}

	// Try to find existing workspace
	var workspace Workspace
	err = db.conn.QueryRowContext(ctx, `
		SELECT workspace_id, branch, worktree_path, display_name, current_task_id,
		       agent_state, todo_list, last_commit, created_at, updated_at
		FROM workspaces
		WHERE branch = ? AND (worktree_path = ? OR (worktree_path IS NULL AND ? IS NULL))
	`, branch, worktreePath, worktreePath).Scan(
		&workspace.WorkspaceID,
		&workspace.Branch,
		&workspace.WorktreePath,
		&workspace.DisplayName,
		&workspace.CurrentTaskID,
		&workspace.AgentState,
		&workspace.TodoList,
		&workspace.LastCommit,
		&workspace.CreatedAt,
		&workspace.UpdatedAt,
	)

	if err == nil {
		// Update last_commit if it has changed
		if workspace.LastCommit != commitHash {
			_, err = db.conn.ExecContext(ctx, `
				UPDATE workspaces
				SET last_commit = ?, updated_at = CURRENT_TIMESTAMP
				WHERE workspace_id = ?
			`, commitHash, workspace.WorkspaceID)
			if err != nil {
				return nil, fmt.Errorf("failed to update workspace last_commit: %w", err)
			}
			workspace.LastCommit = commitHash
		}
		return &workspace, nil
	}

	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query workspace: %w", err)
	}

	// Create new workspace
	displayName := sql.NullString{String: branch, Valid: true}
	var wp sql.NullString
	if worktreePath != "" {
		wp = sql.NullString{String: worktreePath, Valid: true}
	}

	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO workspaces (branch, worktree_path, display_name, last_commit)
		VALUES (?, ?, ?, ?)
	`, branch, wp, displayName, commitHash)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Get the generated workspace_id
	rows, err := result.RowsAffected()
	if err != nil || rows == 0 {
		return nil, fmt.Errorf("failed to get workspace ID after creation")
	}

	// Fetch the created workspace
	err = db.conn.QueryRowContext(ctx, `
		SELECT workspace_id, branch, worktree_path, display_name, current_task_id,
		       agent_state, todo_list, last_commit, created_at, updated_at
		FROM workspaces
		WHERE rowid = last_insert_rowid()
	`).Scan(
		&workspace.WorkspaceID,
		&workspace.Branch,
		&workspace.WorktreePath,
		&workspace.DisplayName,
		&workspace.CurrentTaskID,
		&workspace.AgentState,
		&workspace.TodoList,
		&workspace.LastCommit,
		&workspace.CreatedAt,
		&workspace.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch created workspace: %w", err)
	}

	return &workspace, nil
}

// GetWorkspaceByID retrieves a workspace by its ID
func (db *DB) GetWorkspaceByID(ctx context.Context, workspaceID string) (*Workspace, error) {
	var workspace Workspace
	err := db.conn.QueryRowContext(ctx, `
		SELECT workspace_id, branch, worktree_path, display_name, current_task_id,
		       agent_state, todo_list, last_commit, created_at, updated_at
		FROM workspaces
		WHERE workspace_id = ?
	`, workspaceID).Scan(
		&workspace.WorkspaceID,
		&workspace.Branch,
		&workspace.WorktreePath,
		&workspace.DisplayName,
		&workspace.CurrentTaskID,
		&workspace.AgentState,
		&workspace.TodoList,
		&workspace.LastCommit,
		&workspace.CreatedAt,
		&workspace.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("workspace not found: %s", workspaceID)
		}
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}
	return &workspace, nil
}

// UpdateWorkspaceAgentState updates the agent state for a workspace
func (db *DB) UpdateWorkspaceAgentState(ctx context.Context, workspaceID string, agentState []byte) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE workspaces
		SET agent_state = ?, updated_at = CURRENT_TIMESTAMP
		WHERE workspace_id = ?
	`, agentState, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to update workspace agent state: %w", err)
	}
	return nil
}

// UpdateWorkspaceTodoList updates the todo list for a workspace
func (db *DB) UpdateWorkspaceTodoList(ctx context.Context, workspaceID, todoList string) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE workspaces
		SET todo_list = ?, updated_at = CURRENT_TIMESTAMP
		WHERE workspace_id = ?
	`, todoList, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to update workspace todo list: %w", err)
	}
	return nil
}

// SetCurrentTask sets the current active task for a workspace
func (db *DB) SetCurrentTask(ctx context.Context, workspaceID string, taskID int64) error {
	var tid sql.NullInt64
	if taskID > 0 {
		tid = sql.NullInt64{Int64: taskID, Valid: true}
	}
	_, err := db.conn.ExecContext(ctx, `
		UPDATE workspaces
		SET current_task_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE workspace_id = ?
	`, tid, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to set current task: %w", err)
	}
	return nil
}

// CreateTask creates a new task in a workspace
func (db *DB) CreateTask(ctx context.Context, workspaceID, description string, testCommand *string, parentTaskID *int64) (*Task, error) {
	var tc sql.NullString
	if testCommand != nil {
		tc = sql.NullString{String: *testCommand, Valid: true}
	}

	var ptid sql.NullInt64
	if parentTaskID != nil {
		ptid = sql.NullInt64{Int64: *parentTaskID, Valid: true}
	}

	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO tasks (workspace_id, description, test_command, parent_task_id, status)
		VALUES (?, ?, ?, ?, ?)
	`, workspaceID, description, tc, ptid, TaskStatusPending)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	taskID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get task ID: %w", err)
	}

	return db.GetTaskByID(ctx, taskID)
}

// GetTaskByID retrieves a task by its ID
func (db *DB) GetTaskByID(ctx context.Context, taskID int64) (*Task, error) {
	var task Task
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, workspace_id, description, status, test_command,
		       test_result, priority, parent_task_id, created_at, completed_at
		FROM tasks
		WHERE id = ?
	`, taskID).Scan(
		&task.ID,
		&task.WorkspaceID,
		&task.Description,
		&task.Status,
		&task.TestCommand,
		&task.TestResult,
		&task.Priority,
		&task.ParentTaskID,
		&task.CreatedAt,
		&task.CompletedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("task not found: %d", taskID)
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	return &task, nil
}

// UpdateTaskStatus updates a task's status
func (db *DB) UpdateTaskStatus(ctx context.Context, taskID int64, status TaskStatus) error {
	var completedAt interface{}
	if status == TaskStatusDone || status == TaskStatusFailed || status == TaskStatusSkipped {
		completedAt = time.Now().UTC()
	} else {
		completedAt = nil
	}

	_, err := db.conn.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, completed_at = ?
		WHERE id = ?
	`, status, completedAt, taskID)
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}
	return nil
}

// UpdateTaskTestResult updates a task's test result
func (db *DB) UpdateTaskTestResult(ctx context.Context, taskID int64, testResult []byte) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE tasks
		SET test_result = ?
		WHERE id = ?
	`, testResult, taskID)
	if err != nil {
		return fmt.Errorf("failed to update task test result: %w", err)
	}
	return nil
}

// ListTasks lists tasks for a workspace with optional status filter
func (db *DB) ListTasks(ctx context.Context, workspaceID string, status *TaskStatus, limit, offset int) ([]*Task, error) {
	query := `
		SELECT id, workspace_id, description, status, test_command,
		       test_result, priority, parent_task_id, created_at, completed_at
		FROM tasks
		WHERE workspace_id = ?
	`
	args := []interface{}{workspaceID}

	if status != nil {
		query += " AND status = ?"
		args = append(args, *status)
	}

	query += " ORDER BY priority DESC, created_at ASC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	if offset > 0 {
		query += " OFFSET ?"
		args = append(args, offset)
	}

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		var task Task
		err := rows.Scan(
			&task.ID,
			&task.WorkspaceID,
			&task.Description,
			&task.Status,
			&task.TestCommand,
			&task.TestResult,
			&task.Priority,
			&task.ParentTaskID,
			&task.CreatedAt,
			&task.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}
		tasks = append(tasks, &task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tasks: %w", err)
	}

	return tasks, nil
}

// GetNextPendingTask returns the highest priority pending task for a workspace
func (db *DB) GetNextPendingTask(ctx context.Context, workspaceID string) (*Task, error) {
	var task Task
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, workspace_id, description, status, test_command,
		       test_result, priority, parent_task_id, created_at, completed_at
		FROM tasks
		WHERE workspace_id = ? AND status = ?
		ORDER BY priority DESC, created_at ASC
		LIMIT 1
	`, workspaceID, TaskStatusPending).Scan(
		&task.ID,
		&task.WorkspaceID,
		&task.Description,
		&task.Status,
		&task.TestCommand,
		&task.TestResult,
		&task.Priority,
		&task.ParentTaskID,
		&task.CreatedAt,
		&task.CompletedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get next pending task: %w", err)
	}
	return &task, nil
}

// CreateCheckpoint creates a new checkpoint for a workspace
func (db *DB) CreateCheckpoint(ctx context.Context, workspaceID, commitHash string, agentState []byte, message string, wasClean bool) (*Checkpoint, error) {
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO checkpoints (workspace_id, commit_hash, agent_state, message, was_clean)
		VALUES (?, ?, ?, ?, ?)
	`, workspaceID, commitHash, agentState, message, wasClean)
	if err != nil {
		return nil, fmt.Errorf("failed to create checkpoint: %w", err)
	}

	checkpointID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get checkpoint ID: %w", err)
	}

	return db.GetCheckpointByID(ctx, checkpointID)
}

// GetCheckpointByID retrieves a checkpoint by its ID
func (db *DB) GetCheckpointByID(ctx context.Context, checkpointID int64) (*Checkpoint, error) {
	var checkpoint Checkpoint
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, workspace_id, commit_hash, agent_state, message, was_clean, created_at
		FROM checkpoints
		WHERE id = ?
	`, checkpointID).Scan(
		&checkpoint.ID,
		&checkpoint.WorkspaceID,
		&checkpoint.CommitHash,
		&checkpoint.AgentState,
		&checkpoint.Message,
		&checkpoint.WasClean,
		&checkpoint.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("checkpoint not found: %d", checkpointID)
		}
		return nil, fmt.Errorf("failed to get checkpoint: %w", err)
	}
	return &checkpoint, nil
}

// GetLatestCheckpoint returns the most recent checkpoint for a workspace
func (db *DB) GetLatestCheckpoint(ctx context.Context, workspaceID string) (*Checkpoint, error) {
	var checkpoint Checkpoint
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, workspace_id, commit_hash, agent_state, message, was_clean, created_at
		FROM checkpoints
		WHERE workspace_id = ?
		ORDER BY created_at DESC
		LIMIT 1
	`, workspaceID).Scan(
		&checkpoint.ID,
		&checkpoint.WorkspaceID,
		&checkpoint.CommitHash,
		&checkpoint.AgentState,
		&checkpoint.Message,
		&checkpoint.WasClean,
		&checkpoint.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest checkpoint: %w", err)
	}
	return &checkpoint, nil
}

// ListCheckpoints lists checkpoints for a workspace
func (db *DB) ListCheckpoints(ctx context.Context, workspaceID string, limit, offset int) ([]*Checkpoint, error) {
	query := `
		SELECT id, workspace_id, commit_hash, agent_state, message, was_clean, created_at
		FROM checkpoints
		WHERE workspace_id = ?
		ORDER BY created_at DESC
	`
	args := []interface{}{workspaceID}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	if offset > 0 {
		query += " OFFSET ?"
		args = append(args, offset)
	}

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query checkpoints: %w", err)
	}
	defer rows.Close()

	var checkpoints []*Checkpoint
	for rows.Next() {
		var checkpoint Checkpoint
		err := rows.Scan(
			&checkpoint.ID,
			&checkpoint.WorkspaceID,
			&checkpoint.CommitHash,
			&checkpoint.AgentState,
			&checkpoint.Message,
			&checkpoint.WasClean,
			&checkpoint.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan checkpoint: %w", err)
		}
		checkpoints = append(checkpoints, &checkpoint)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating checkpoints: %w", err)
	}

	return checkpoints, nil
}

// CreateTrace creates a new trace record for a test execution
func (db *DB) CreateTrace(ctx context.Context, taskID int64, command string, exitCode *int, outputTruncated, logPath *string, durationMS *int64) (*Trace, error) {
	var ec sql.NullInt64
	if exitCode != nil {
		ec = sql.NullInt64{Int64: int64(*exitCode), Valid: true}
	}

	var ot sql.NullString
	if outputTruncated != nil {
		ot = sql.NullString{String: *outputTruncated, Valid: true}
	}

	var lp sql.NullString
	if logPath != nil {
		lp = sql.NullString{String: *logPath, Valid: true}
	}

	var dm sql.NullInt64
	if durationMS != nil {
		dm = sql.NullInt64{Int64: *durationMS, Valid: true}
	}

	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO traces (task_id, command, exit_code, output_truncated, log_path, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?)
	`, taskID, command, ec, ot, lp, dm)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace: %w", err)
	}

	traceID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get trace ID: %w", err)
	}

	return db.GetTraceByID(ctx, traceID)
}

// GetTraceByID retrieves a trace by its ID
func (db *DB) GetTraceByID(ctx context.Context, traceID int64) (*Trace, error) {
	var trace Trace
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, task_id, command, exit_code, output_truncated, log_path, duration_ms, run_at
		FROM traces
		WHERE id = ?
	`, traceID).Scan(
		&trace.ID,
		&trace.TaskID,
		&trace.Command,
		&trace.ExitCode,
		&trace.OutputTruncated,
		&trace.LogPath,
		&trace.DurationMS,
		&trace.RunAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("trace not found: %d", traceID)
		}
		return nil, fmt.Errorf("failed to get trace: %w", err)
	}
	return &trace, nil
}

// ListTraces lists traces for a task
func (db *DB) ListTraces(ctx context.Context, taskID int64, limit, offset int) ([]*Trace, error) {
	query := `
		SELECT id, task_id, command, exit_code, output_truncated, log_path, duration_ms, run_at
		FROM traces
		WHERE task_id = ?
		ORDER BY run_at DESC
	`
	args := []interface{}{taskID}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	if offset > 0 {
		query += " OFFSET ?"
		args = append(args, offset)
	}

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query traces: %w", err)
	}
	defer rows.Close()

	var traces []*Trace
	for rows.Next() {
		var trace Trace
		err := rows.Scan(
			&trace.ID,
			&trace.TaskID,
			&trace.Command,
			&trace.ExitCode,
			&trace.OutputTruncated,
			&trace.LogPath,
			&trace.DurationMS,
			&trace.RunAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trace: %w", err)
		}
		traces = append(traces, &trace)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating traces: %w", err)
	}

	return traces, nil
}

// Helper functions for Git operations
func getCurrentGitBranch() (string, error) {
	return git.GetCurrentBranch()
}

func getCurrentWorktreePath() (string, error) {
	return git.GetWorktreePath()
}

func getCurrentCommitHash() (string, error) {
	return git.GetCurrentCommitHash()
}

// GetWorkspaceStatus returns comprehensive status for a workspace
func (db *DB) GetWorkspaceStatus(ctx context.Context, workspaceID string) (*WorkspaceStatus, error) {
	workspace, err := db.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	// Get active task if set
	var activeTask *Task
	if workspace.CurrentTaskID.Valid {
		activeTask, _ = db.GetTaskByID(ctx, workspace.CurrentTaskID.Int64)
	}

	// Get task counts by status
	rows, err := db.conn.QueryContext(ctx, `
		SELECT status, COUNT(*) as count
		FROM tasks
		WHERE workspace_id = ?
		GROUP BY status
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task counts: %w", err)
	}
	defer rows.Close()

	taskCounts := make(map[TaskStatus]int)
	for rows.Next() {
		var status TaskStatus
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan task count: %w", err)
		}
		taskCounts[status] = count
	}

	// Get latest checkpoint
	lastCheckpoint, _ := db.GetLatestCheckpoint(ctx, workspaceID)

	return &WorkspaceStatus{
		Workspace:      workspace,
		ActiveTask:     activeTask,
		TaskCounts:     taskCounts,
		LastCheckpoint: lastCheckpoint,
	}, nil
}

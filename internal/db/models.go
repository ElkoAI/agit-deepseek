package db

import (
	"database/sql"
	"encoding/json"
	"time"
)

// TaskStatus represents the possible states of a task
type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusActive  TaskStatus = "active"
	TaskStatusDone    TaskStatus = "done"
	TaskStatusFailed  TaskStatus = "failed"
	TaskStatusBlocked TaskStatus = "blocked"
	TaskStatusSkipped TaskStatus = "skipped"
)

// SchemaMeta represents the schema version metadata
type SchemaMeta struct {
	Version     int       `json:"version"`
	Description string    `json:"description"`
	AppliedAt   time.Time `json:"applied_at"`
}

// Workspace represents an agent workspace tied to a Git branch (and optionally a worktree)
type Workspace struct {
	WorkspaceID   string         `json:"workspace_id"`
	Branch        string         `json:"branch"`
	WorktreePath  sql.NullString `json:"worktree_path,omitempty"`
	DisplayName   sql.NullString `json:"display_name,omitempty"`
	CurrentTaskID sql.NullInt64  `json:"current_task_id,omitempty"`
	AgentState    []byte         `json:"agent_state,omitempty"` // JSON-encoded agent state
	TodoList      sql.NullString `json:"todo_list,omitempty"`
	LastCommit    string         `json:"last_commit,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// AgentState represents the structure of agent state JSON
type AgentState struct {
	Version       string `json:"version"`
	SessionNotes  string `json:"session_notes,omitempty"`
	TodoList      string `json:"todo_list,omitempty"`
	LastAction    string `json:"last_action,omitempty"`
	Context       string `json:"context,omitempty"`
	AgentType     string `json:"agent_type,omitempty"`
	CustomFields  json.RawMessage `json:"custom_fields,omitempty"`
}

// Task represents an atomic unit of work with optional test command
type Task struct {
	ID           int64          `json:"id"`
	WorkspaceID  string         `json:"workspace_id"`
	Description  string         `json:"description"`
	Status       TaskStatus     `json:"status"`
	TestCommand  sql.NullString `json:"test_command,omitempty"`
	TestResult   []byte         `json:"test_result,omitempty"` // JSON-encoded test result
	Priority     int            `json:"priority"`
	ParentTaskID sql.NullInt64  `json:"parent_task_id,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	CompletedAt  sql.NullTime   `json:"completed_at,omitempty"`
}

// TestResult represents the structure of test result JSON
type TestResult struct {
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	Passed     bool   `json:"passed"`
	Timestamp  time.Time `json:"timestamp"`
}

// Checkpoint represents a Git commit + agent state snapshot
type Checkpoint struct {
	ID          int64     `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	CommitHash  string    `json:"commit_hash"`
	AgentState  []byte    `json:"agent_state,omitempty"` // JSON snapshot at checkpoint time
	Message     string    `json:"message"`
	WasClean    bool      `json:"was_clean"`
	CreatedAt   time.Time `json:"created_at"`
}

// Trace represents a test execution history record
type Trace struct {
	ID              int64          `json:"id"`
	TaskID          int64          `json:"task_id"`
	Command         string         `json:"command"`
	ExitCode        sql.NullInt64  `json:"exit_code,omitempty"`
	OutputTruncated sql.NullString `json:"output_truncated,omitempty"`
	LogPath         sql.NullString `json:"log_path,omitempty"`
	DurationMS      sql.NullInt64  `json:"duration_ms,omitempty"`
	RunAt           time.Time      `json:"run_at"`
}

// WorkspaceStatus is a convenience struct for returning workspace state
type WorkspaceStatus struct {
	Workspace   *Workspace `json:"workspace"`
	ActiveTask  *Task      `json:"active_task,omitempty"`
	TaskCounts  map[TaskStatus]int `json:"task_counts"`
	LastCheckpoint *Checkpoint `json:"last_checkpoint,omitempty"`
}

// TaskWithWorkspace includes task with workspace information
type TaskWithWorkspace struct {
	Task      *Task      `json:"task"`
	Workspace *Workspace `json:"workspace"`
}

// CheckpointWithWorkspace includes checkpoint with workspace information
type CheckpointWithWorkspace struct {
	Checkpoint *Checkpoint `json:"checkpoint"`
	Workspace  *Workspace  `json:"workspace"`
}

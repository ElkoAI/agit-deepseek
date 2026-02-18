-- AGIT Database Schema
-- Version 1
-- Includes workspace_id for stable identity and schema versioning

-- Schema metadata table (MUST be first)
CREATE TABLE IF NOT EXISTS schema_meta (
    version INTEGER PRIMARY KEY,
    description TEXT NOT NULL,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Workspaces with stable identity (not dependent on branch names)
CREATE TABLE IF NOT EXISTS workspaces (
    workspace_id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    branch TEXT NOT NULL,
    worktree_path TEXT,  -- NULL for main worktree, path for additional worktrees
    display_name TEXT,   -- User-friendly name for display
    current_task_id INTEGER,  -- Currently active task
    agent_state TEXT,    -- JSON-encoded agent state (versioned)
    todo_list TEXT,      -- Markdown todo list
    last_commit TEXT,    -- Last known commit hash
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(branch, worktree_path)  -- Ensure one workspace per (branch, worktree) combination
);

-- Tasks with enhanced status support
CREATE TABLE IF NOT EXISTS tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id TEXT NOT NULL,
    description TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('pending', 'active', 'done', 'failed', 'blocked', 'skipped')) DEFAULT 'pending',
    test_command TEXT,      -- NULL for non-testable tasks
    test_result TEXT,       -- JSON-encoded last test result
    priority INTEGER DEFAULT 0,  -- Higher number = higher priority
    parent_task_id INTEGER, -- For task decomposition
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    FOREIGN KEY (parent_task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    -- Allow done tasks without test_command (non-testable tasks)
    CHECK (NOT (status = 'done' AND test_command IS NOT NULL AND test_result IS NULL))
);

-- Checkpoints: Git commits + agent state snapshots
CREATE TABLE IF NOT EXISTS checkpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id TEXT NOT NULL,
    commit_hash TEXT NOT NULL,
    agent_state TEXT,  -- JSON snapshot of agent state at checkpoint
    message TEXT NOT NULL,
    was_clean BOOLEAN NOT NULL DEFAULT FALSE,  -- Whether working tree was clean
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    UNIQUE(workspace_id, commit_hash)  -- One checkpoint per commit per workspace
);

-- Trace: Test execution history
CREATE TABLE IF NOT EXISTS traces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id INTEGER NOT NULL,
    command TEXT NOT NULL,          -- The command that was executed
    exit_code INTEGER,              -- Exit code of the command
    output_truncated TEXT,          -- First 1000 chars of output
    log_path TEXT,                  -- Path to full output file (optional)
    duration_ms INTEGER,            -- Execution time in milliseconds
    run_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_workspaces_branch_worktree ON workspaces(branch, worktree_path);
CREATE INDEX IF NOT EXISTS idx_workspaces_last_commit ON workspaces(last_commit);
CREATE INDEX IF NOT EXISTS idx_tasks_workspace_status ON tasks(workspace_id, status);
CREATE INDEX IF NOT EXISTS idx_tasks_priority_pending ON tasks(priority DESC) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_task_id);
CREATE INDEX IF NOT EXISTS idx_checkpoints_workspace ON checkpoints(workspace_id);
CREATE INDEX IF NOT EXISTS idx_checkpoints_created ON checkpoints(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_traces_task ON traces(task_id);
CREATE INDEX IF NOT EXISTS idx_traces_run_at ON traces(run_at DESC);

-- Insert initial schema version
INSERT OR IGNORE INTO schema_meta (version, description) VALUES (1, 'Initial schema with workspace_id');

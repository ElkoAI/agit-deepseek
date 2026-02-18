package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite database connection and provides database operations
type DB struct {
	conn *sql.DB
	path string
}

// NewDB creates a new database connection at the given path
func NewDB(path string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Open database with WAL mode for better concurrency
	conn, err := sql.Open("sqlite3", path+"?_journal=WAL&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database %s: %w", path, err)
	}

	db := &DB{
		conn: conn,
		path: path,
	}

	// Enable foreign keys
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Run migrations
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// migrate checks the current schema version and applies necessary migrations
func (db *DB) migrate() error {
	// Start transaction for migration
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if schema_meta table exists to determine current version
	var currentVersion int
	err = tx.QueryRow(`
		SELECT version FROM schema_meta
		ORDER BY version DESC
		LIMIT 1
	`).Scan(&currentVersion)

	if err == sql.ErrNoRows {
		// Fresh database, initialize with version 1
		if err := db.initializeSchema(tx); err != nil {
			return fmt.Errorf("failed to initialize schema: %w", err)
		}
		currentVersion = 1
	} else if err != nil {
		// schema_meta table doesn't exist yet (pre-versioning schema)
		// This is a legacy database, we need to handle it
		return db.migrateLegacy(tx)
	}

	// Apply migrations sequentially
	for version := currentVersion + 1; ; version++ {
		migrationFunc, ok := migrations[version]
		if !ok {
			// No more migrations to apply
			break
		}

		if err := migrationFunc(tx); err != nil {
			return fmt.Errorf("failed to apply migration v%d: %w", version, err)
		}

		// Record migration
		if _, err := tx.Exec(`
			INSERT INTO schema_meta (version, description)
			VALUES (?, ?)
		`, version, fmt.Sprintf("Migration to version %d", version)); err != nil {
			return fmt.Errorf("failed to record migration v%d: %w", version, err)
		}
	}

	return tx.Commit()
}

// initializeSchema creates the initial schema (version 1)
func (db *DB) initializeSchema(tx *sql.Tx) error {
	// Execute embedded schema
	if _, err := tx.Exec(initialSchema); err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	return nil
}

// migrateLegacy handles databases created before schema versioning was added
func (db *DB) migrateLegacy(tx *sql.Tx) error {
	// Check if workspaces table exists with old schema (branch as primary key)
	var tableExists bool
	err := tx.QueryRow(`
		SELECT COUNT(*) > 0 FROM sqlite_master
		WHERE type='table' AND name='workspaces'
	`).Scan(&tableExists)

	if err != nil {
		return fmt.Errorf("failed to check for legacy tables: %w", err)
	}

	if !tableExists {
		// No tables exist, just initialize fresh schema
		return db.initializeSchema(tx)
	}

	// Legacy database exists - need to migrate from branch-based to workspace_id-based
	// This is a complex migration that should be implemented when needed
	// For now, we'll create schema_meta table and mark as version 0 (needs migration)
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS schema_meta (
			version INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("failed to create schema_meta table: %w", err)
	}

	// Mark as version 0 (requires manual migration)
	if _, err := tx.Exec(`
		INSERT INTO schema_meta (version, description)
		VALUES (0, 'Legacy schema - requires manual migration to workspace_id')
	`); err != nil {
		return fmt.Errorf("failed to record legacy version: %w", err)
	}

	return fmt.Errorf("legacy database detected - requires manual migration. Run 'agit migrate-legacy'")
}

// migrations maps version numbers to migration functions
var migrations = map[int]func(*sql.Tx) error{
	// Version 2 and up will be added here as the schema evolves
	// Example:
	// 2: migrateV2,
}

// GetDBPath returns the default database path for the current repository
func GetDBPath() (string, error) {
	// Look for .git directory starting from current directory and going up
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	for {
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			// Found .git directory
			return filepath.Join(gitPath, "agit.db"), nil
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root directory
			return "", fmt.Errorf("not in a git repository")
		}
		dir = parent
	}
}

// initialSchema is the embedded initial schema (version 1)
const initialSchema = `-- AGIT Database Schema
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
`

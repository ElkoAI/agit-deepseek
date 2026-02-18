

# AGIT: Agent Git - MCP Server for Stateful Agent Workflows

## Core Concept

**agit** is a Go MCP server that lives in `.git/agit.db` and provides Git-native state persistence for AI coding agents. It's the missing link between Git's version control and agentic workflows.

## Terminology (Design First)

| Term           | Definition                                    | Example                                   |
| -------------- | --------------------------------------------- | ----------------------------------------- |
| **Workspace**  | Current Git branch + agent context            | `feat/217-impl-check`                     |
| **Session**    | Agent's state for a workspace                 | Todo list, last action, context           |
| **Checkpoint** | Git commit + agent state snapshot             | `agit checkpoint "before risky refactor"` |
| **Task**       | Atomic unit of work with test                 | `Task: implement method set comparison`   |
| **Trace**      | Test run history for a workspace              | Exit codes, output, duration              |
| **Manifest**   | Project's agit configuration                  | `.agit-manifest.json`                     |
| **Sync**       | Export/import state to remote (GitHub Issues) | Future                                    |

## Architecture

text

agit (Go binary)
├── cmd/
│   ├── agit-mcp           # MCP server (stdio)
│   └── agit-hook          # Git hook installer
├── internal/
│   ├── db/                 # SQLite schema/ops
│   ├── git/                # Git porcelain/plumbing
│   ├── mcp/                 # MCP tool handlers
│   ├── task/                # Task execution/tracing
│   └── workspace/          # Workspace state machine
└── .git/agit.db            # Single source of truth

## Data Model (SQLite)

sql

-- One DB to rule them all, in .git/agit.db
-- Workspace = branch + agent context
CREATE TABLE workspaces (
    branch TEXT PRIMARY KEY,           -- git rev-parse --abbrev-ref HEAD
    current_task_id INTEGER,           
    agent_state JSON,                  -- Serialized agent context
    todo_list TEXT,                     -- Markdown checklist
    last_commit TEXT,                    -- git rev-parse HEAD
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
-- Tasks = atomic units of work
CREATE TABLE tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_branch TEXT NOT NULL,
    description TEXT NOT NULL,          -- "Implement X"
    status TEXT CHECK(status IN ('pending','active','done','failed')),
    test_command TEXT,                   -- "go test ./..." 
    test_result JSON,                     -- Last run output
    parent_task_id INTEGER,               -- For decomposition
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    FOREIGN KEY(workspace_branch) REFERENCES workspaces(branch),
    FOREIGN KEY(parent_task_id) REFERENCES tasks(id)
);
-- Checkpoints = git commits + agent state snapshots
CREATE TABLE checkpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_branch TEXT NOT NULL,
    commit_hash TEXT NOT NULL,           -- git commit hash
    agent_state JSON,                     -- Snapshot at commit time
    message TEXT,                          -- User-provided message
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(workspace_branch) REFERENCES workspaces(branch)
);
-- Traces = test run history
CREATE TABLE traces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id INTEGER NOT NULL,
    exit_code INTEGER,
    output TEXT,
    duration_ms INTEGER,
    run_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(task_id) REFERENCES tasks(id)
);
-- Indexes for perf
CREATE INDEX idx_workspaces_branch ON workspaces(branch);
CREATE INDEX idx_tasks_workspace ON tasks(workspace_branch);
CREATE INDEX idx_checkpoints_workspace ON checkpoints(workspace_branch);
CREATE INDEX idx_traces_task ON traces(task_id);

## MCP Tools (What Agents Call)

go

// agit exposes these tools via MCP:
tools := []mcp.Tool{
    {
        Name: "workspace_status",
        Description: "Get current workspace state",
        Handler: func() (*Workspace, error) {
            branch := git.CurrentBranch()
            return db.GetWorkspace(branch)
        }
    },
    {
        Name: "task_new",
        Description: "Create new task",
        Arguments: []string{"description", "test_command"},
        Handler: func(desc, testCmd string) error {
            return db.CreateTask(currentWorkspace(), desc, testCmd)
        }
    },
    {
        Name: "task_list",
        Description: "List tasks (active/done)",
        Handler: func() ([]Task, error) {
            return db.GetTasks(currentWorkspace())
        }
    },
    {
        Name: "task_start",
        Description: "Mark task as active",
        Arguments: []string{"task_id"},
        Handler: func(id int) error {
            return db.UpdateTaskStatus(id, "active")
        }
    },
    {
        Name: "task_done",
        Description: "Complete task + run tests",
        Arguments: []string{"task_id"},
        Handler: func(id int) error {
            task := db.GetTask(id)
            result := runTest(task.TestCommand)
            db.RecordTrace(id, result)
            if result.ExitCode == 0 {
                db.UpdateTaskStatus(id, "done")
            }
            return nil
        }
    },
    {
        Name: "checkpoint",
        Description: "Save state + git commit",
        Arguments: []string{"message"},
        Handler: func(msg string) error {
            commit := git.CommitAll(msg)
            state := db.GetWorkspace(currentWorkspace())
            return db.CreateCheckpoint(currentWorkspace(), commit, state)
        }
    },
    {
        Name: "restore",
        Description: "Restore to last checkpoint",
        Handler: func() error {
            cp := db.GetLatestCheckpoint(currentWorkspace())
            git.Reset(cp.CommitHash)
            db.RestoreWorkspaceState(cp.AgentState)
            return nil
        }
    },
    {
        Name: "trace",
        Description: "Show test history",
        Arguments: []string{"task_id", "limit"},
        Handler: func(taskID, limit int) ([]Trace, error) {
            return db.GetTraces(taskID, limit)
        }
    },
}

## Git Hooks (Automatic State)

bash

# .git/hooks/post-checkout (installed by agit hook install)

#!/bin/bash
branch=$(git rev-parse --abbrev-ref HEAD)
commit=$(git rev-parse HEAD)
echo "{\"branch\":\"$branch\",\"commit\":\"$commit\"}" | \
  agit-mcp tool workspace_switch

bash

# .git/hooks/pre-commit (optional validation)

#!/bin/bash

# Ensure all tasks are done before commit

agit-mcp tool workspace_validate || exit 1

## Agent Integration (Minimal)

### Claude Code

json

// .claude/mcp.json
{
  "mcpServers": {
    "agit": {
      "command": "agit-mcp",
      "args": ["--stdio"]
    }
  }
}

### OpenCode

json

// .opencode/plugins/agit.json
{
  "name": "agit",
  "mcp": {
    "server": {
      "command": "agit-mcp",
      "args": ["--stdio"]
    }
  }
}

### Cursor/VS Code/Zed

Same MCP config pattern. For Zed's built-in agent, MCP support is coming.

## Core Workflow Pattern

**Simple, repeatable, test-driven.**

## Incremental Implementation Plan

### Phase 0: Core Infrastructure (Week 1)

- `agit init` - creates `.git/agit.db` with schema

- `agit hook install` - installs post-checkout hook

- Basic Go module with SQLite

### Phase 1: MCP Server Basics (Week 2)

- `agit-mcp` stdio server with `mark3labs/mcp-go`

- Tool: `workspace_status`

- Tool: `task_new` + `task_list`

- Tool: `task_start` + `task_done` (with test stub)

### Phase 2: Checkpoint System (Week 3)

- Tool: `checkpoint` (git commit + state snapshot)

- Tool: `restore` (git reset + state restore)

- Store agent_state JSON in checkpoints table

### Phase 3: Test Tracing (Week 4)

- Tool: `trace` - record test runs

- Tool: `task_done` runs test_command

- Query: "show me failing tests for this task"

### Phase 4: Agent Integrations (Week 5)

- Claude Code MCP config doc

- OpenCode plugin manifest

- Example skills/commands for each

### Phase 5: Task Decomposition (Week 6)

- parent_task_id support

- `task breakdown "subtasks"` tool

- Visualize task tree

### Phase 6: Remote Sync (Future)

- `agit sync github` - push/pull to GitHub Issues

- `agit sync litestream` - S3 replication

- Web dashboard (read-only)

## Testing Strategy

Every task includes a test command. agit tracks test history:

bash

agit task new "add method set comparison" --test "go test ./internal/analyzer"
agit task start 1

# ... code ...

agit task done 1  # runs test, records trace, marks done if passes

If tests fail:

bash

agit trace 1 --last  # shows last failure

# fix code

agit task done 1  # retry

## Manifest File (`.agit-manifest.json`)

json

{
  "version": "1.0.0",
  "name": "agit",
  "description": "Agent Git MCP Server",
  "tasks": [
    {
      "id": 1,
      "description": "Implement workspace_status tool",
      "test": "go test ./internal/mcp -run TestWorkspaceStatus",
      "dependencies": []
    }
  ],
  "agents": {
    "preferred": "claude-code",
    "integrations": ["opencode", "cursor"]
  }
}

This file can seed the project's own agit workspace.

## Self-Hosting: agit Tracking agit

Meta: Use agit to build agit. The manifest above becomes the initial task list.

## Design Principles

1. **State in .git/** - Never pollute working tree

2. **MCP first** - One server, many clients

3. **Test-driven tasks** - Every task has a test command

4. **Checkpoint everything** - Git commits + agent state

5. **Simple terms** - Workspace, task, checkpoint, trace

6. **Incremental commits** - Every phase is git-committable

## Questions to Resolve

1. **Agent state format** - JSON blob? What minimal fields?

2. **Task test schema** - How to parse test commands from descriptions?

3. **Concurrency** - Multiple agents on same workspace?

4. **Migration** - When schema changes, how to upgrade .git/agit.db?

5. **Checkpoint pruning** - Old checkpoints cleanup?

## Next Action

Create `agit` repo with:

- `README.md` (this doc condensed)

- `go.mod`

- `internal/db/schema.sql`

- `cmd/agit-mcp/main.go` (stub)

- First task: "Initialize database schema"

---

**This document is both design spec and project seed. Feed it to any agent to start building.**

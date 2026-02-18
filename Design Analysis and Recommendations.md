# AGIT Design Analysis and Recommendations

## 1. Introduction

AGIT (Agent Git) is a Go-based MCP server designed to provide Git-native state persistence for AI coding agents. By storing agent state in `.git/agit.db`, it creates a seamless bridge between Git's version control capabilities and agentic workflows, enabling test-driven development with persistent context across sessions.

This analysis synthesizes the original design document with the provided expert review to identify strengths, gaps, and actionable improvements for implementation.

## 2. Overall Assessment

**Strong foundation with pragmatic MVP scope.** The core concept of storing agent state in `.git/` is elegant and aligns perfectly with Git's philosophy. The MCP-first approach ensures broad client compatibility, and the incremental implementation plan demonstrates realistic project management.

**Ready for implementation with minor refinements.** The design has sufficient detail to begin coding immediately, though several architectural decisions require clarification before finalizing the schema.

## 3. Key Strengths

### 3.1 Core Architecture
- **Git-native state persistence:** Storing everything in `.git/agit.db` keeps the working tree clean and leverages Git's existing isolation
- **MCP-first design:** Single server supporting multiple clients (Claude Code, OpenCode, Cursor, etc.)
- **Minimal dependencies:** SQLite + Go provides lightweight, portable solution

### 3.2 Data Model
- **Clear terminology:** Workspace, session, checkpoint, task, trace establish consistent vocabulary
- **Simple SQLite schema:** Adequate for Phases 1-3 without over-engineering
- **Test-driven workflow:** Every task includes a test command by design

### 3.3 Workflow Design
- **Checkpoint system:** Git commits + agent state snapshots provide comprehensive rollback
- **Trace tracking:** Historical test execution data enables debugging and analysis
- **Incremental commits:** Each implementation phase is independently committable

### 3.4 Integration Strategy
- **Standard MCP configuration:** Works with existing agent ecosystems
- **Git hooks:** Automatic state synchronization on branch switches
- **Self-hosting capability:** Can use AGIT to build AGIT (meta workflow)

## 4. Critical Gaps and Risks

### 4.1 Workspace Identity Problem
**Issue:** Using `branch` as primary key for workspaces creates several problems:
- Branch renames break workspace identity
- Detached HEAD states have no branch identifier
- Multiple worktrees on same branch cannot have separate agent states
- Branch-based identity complicates workspace migration between repositories

**Impact:** High risk of data loss or corruption during normal Git operations.

### 4.2 Concurrency and Multi-Agent Support
**Issue:** No defined locking semantics or conflict resolution for:
- Multiple agents operating on same workspace
- Concurrent database access from different processes
- Branch-scoped `agent_state` overwritten by competing agents

**Impact:** Data races and inconsistent state in team or multi-agent scenarios.

### 4.3 Schema Evolution Strategy
**Issue:** No versioning or migration plan for database schema changes.
- How to upgrade `.git/agit.db` when new AGIT versions require schema changes?
- No rollback mechanism for failed migrations
- Distributed teams may have different AGIT versions

**Impact:** Upgrade path becomes complex and error-prone over time.

### 4.4 Git Hook Coverage
**Issue:** Reliance only on `post-checkout` hook misses critical Git operations:
- `post-merge` after `git merge` operations
- `post-rewrite` after `git rebase` and `git commit --amend`
- `post-commit` to reliably update `last_commit` field

**Impact:** Workspace state becomes desynchronized from actual Git state.

### 4.5 Task/Test Model Limitations
**Issue:** `task_done` unconditionally runs tests, but:
- Some tasks may not have tests (design, documentation, research)
- Test commands may fail for reasons unrelated to task completion
- No status for "blocked" or "skipped" tasks

**Impact:** Rigid workflow doesn't accommodate all development activities.

### 4.6 Destructive Restore Operations
**Issue:** `restore` performs `git reset` without safety checks:
- No verification of clean working tree
- No distinction between `--hard`, `--mixed`, or `--soft` reset
- No confirmation or `--force` requirement for destructive operations

**Impact:** Risk of losing uncommitted work without warning.

### 4.7 Agent State Model Undefined
**Issue:** `agent_state JSON` field has no defined schema:
- Different agents may store incompatible state formats
- No versioning of state structure
- Migration between agent types becomes impossible

**Impact:** Agent state becomes unstructured "blob" with limited utility.

### 4.8 CLI Ergonomics Missing
**Issue:** No human-friendly commands for common operations:
- `agit status` for workspace overview
- `agit task next` for retrieving highest priority task
- `agit workspace list` for available workspaces

**Impact:** Developers must use MCP tools directly or remember complex commands.

## 5. Concrete Recommendations

### 5.1 High Priority (MVP Blocking)

#### 5.1.1 Stable Workspace Identity
Replace `branch` as primary key with `workspace_id`:

```sql
CREATE TABLE workspaces (
    workspace_id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    branch TEXT NOT NULL,
    worktree_path TEXT,  -- For multi-worktree support
    -- ... other fields
    UNIQUE(branch, worktree_path)
);
```

**Benefits:** Survives branch renames, supports detached HEAD, enables multi-worktree.

#### 5.1.2 Schema Versioning Table
Add from day one:

```sql
CREATE TABLE schema_meta (
    version INTEGER PRIMARY KEY,
    description TEXT,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO schema_meta (version, description) VALUES (1, 'Initial schema');
```

**Benefits:** Safe migration path, version compatibility checking.

#### 5.1.3 Checkpoint Safety
Modify `restore` behavior:
- Check for dirty working tree, require `--force` if dirty
- Default to `git reset --mixed` (preserve working directory changes)
- Add `--hard` flag for complete reset

**Benefits:** Prevents accidental data loss, gives user control.

### 5.2 Medium Priority (Post-MVP)

#### 5.2.1 Task Lifecycle Enhancements
Add task statuses and flexible completion:

```sql
-- Update tasks table
status TEXT CHECK(status IN ('pending','active','done','failed','blocked','skipped'))
```

Add `task_done --no-test` flag for non-testable tasks.

#### 5.2.2 Complete Git Hook Coverage
Install additional hooks:
- `post-merge`: Update workspace after merges
- `post-rewrite`: Handle rebase and amend operations  
- `post-commit`: Update `last_commit` reliably

#### 5.2.3 Defined Agent State Schema
Establish minimal, versioned JSON structure:

```json
{
  "version": "1.0",
  "session_notes": "Current focus and context",
  "todo_list": "Markdown checklist",
  "last_action": "What the agent just did",
  "context_summary": "Key decisions and rationale",
  "agent_type": "claude-code/opename/etc"
}
```

### 5.3 Nice-to-Have (Future)

#### 5.3.1 Enhanced Trace Storage
Store structured test execution data:

```sql
CREATE TABLE traces (
    -- existing fields plus:
    command TEXT NOT NULL,
    duration_ms INTEGER,
    exit_code INTEGER,
    output_truncated TEXT,  -- First 1000 chars
    log_path TEXT,  -- Path to full output file
    -- ...
);
```

#### 5.3.2 CLI Ergonomics
Add developer-friendly commands:
- `agit status` - Human-readable workspace summary
- `agit task next` - Get highest priority pending task
- `agit workspace switch <name>` - Change active workspace

## 6. Schema Improvements

### 6.1 Proposed Revised Schema

```sql
-- Version metadata (MUST be first table)
CREATE TABLE schema_meta (
    version INTEGER PRIMARY KEY,
    description TEXT,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Workspaces with stable identity
CREATE TABLE workspaces (
    workspace_id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    branch TEXT NOT NULL,
    worktree_path TEXT,  -- NULL for main worktree
    display_name TEXT,   -- User-friendly name
    current_task_id INTEGER REFERENCES tasks(id) ON DELETE SET NULL,
    agent_state JSON,    -- Versioned JSON structure
    todo_list TEXT,
    last_commit TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(branch, worktree_path)
);

-- Tasks with enhanced statuses
CREATE TABLE tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id TEXT NOT NULL REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    description TEXT NOT NULL,
    status TEXT CHECK(status IN ('pending','active','done','failed','blocked','skipped')) DEFAULT 'pending',
    test_command TEXT,  -- NULL for non-testable tasks
    test_result JSON,
    priority INTEGER DEFAULT 0,
    parent_task_id INTEGER REFERENCES tasks(id) ON DELETE CASCADE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    CHECK (test_command IS NOT NULL OR status != 'done' OR status = 'done' AND test_command IS NULL)
);

-- Checkpoints with safety metadata
CREATE TABLE checkpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id TEXT NOT NULL REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    commit_hash TEXT NOT NULL,
    agent_state JSON,
    message TEXT,
    was_clean BOOLEAN,  -- Whether working tree was clean at checkpoint
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Enhanced trace storage
CREATE TABLE traces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    command TEXT NOT NULL,
    exit_code INTEGER,
    output_truncated TEXT,
    log_path TEXT,
    duration_ms INTEGER,
    run_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for performance
CREATE INDEX idx_workspaces_identity ON workspaces(branch, worktree_path);
CREATE INDEX idx_tasks_workspace ON tasks(workspace_id);
CREATE INDEX idx_tasks_priority ON tasks(priority DESC) WHERE status = 'pending';
CREATE INDEX idx_checkpoints_workspace ON checkpoints(workspace_id);
CREATE INDEX idx_traces_task ON traces(task_id);
```

### 6.2 Foreign Key Strategy
- Use `ON DELETE CASCADE` for child records
- Use `ON DELETE SET NULL` for optional references
- Ensures data consistency when workspaces or tasks are removed

## 7. MCP Tool Enhancements

### 7.1 Essential Tool Refinements

```go
// workspace_status should return workspace_id, not just branch
tool {
    Name: "workspace_status",
    Handler: func() (*WorkspaceStatus, error) {
        return db.GetCurrentWorkspace() // Uses worktree detection
    }
}

// task_done with optional test skipping
tool {
    Name: "task_done",
    Arguments: []string{"task_id", "--skip-test"},
    Handler: func(id int, skipTest bool) error {
        task := db.GetTask(id)
        if !skipTest && task.TestCommand != "" {
            result := runTest(task.TestCommand)
            db.RecordTrace(id, result)
            if result.ExitCode != 0 {
                return fmt.Errorf("tests failed: %s", result.Output)
            }
        }
        return db.UpdateTaskStatus(id, "done")
    }
}

// restore with safety checks
tool {
    Name: "restore",
    Arguments: []string{"--force", "--hard"},
    Handler: func(force, hard bool) error {
        if !force && git.IsWorkingTreeDirty() {
            return errors.New("working tree dirty, use --force to proceed")
        }
        cp := db.GetLatestCheckpoint(currentWorkspaceId())
        resetType := "--mixed"
        if hard {
            resetType = "--hard"
        }
        git.Reset(cp.CommitHash, resetType)
        db.RestoreWorkspaceState(cp.AgentState)
        return nil
    }
}
```

### 7.2 Additional Recommended Tools

```go
// For CLI ergonomics
{
    Name: "workspace_list",
    Description: "List all available workspaces",
    Handler: func() ([]Workspace, error) { /* ... */ }
},
{
    Name: "task_next",
    Description: "Get highest priority pending task",
    Handler: func() (*Task, error) { /* ... */ }
},
{
    Name: "workspace_create",
    Description: "Create new workspace",
    Arguments: []string{"branch", "name"},
    Handler: func(branch, name string) error { /* ... */ }
}
```

## 8. Git Hook Strategy

### 8.1 Required Hooks

```bash
# .git/hooks/post-checkout
#!/bin/bash
previous_head=$1
new_head=$2
checkout_type=$3

# Update workspace tracking
agit-mcp tool workspace_switch --branch $(git rev-parse --abbrev-ref HEAD)

# .git/hooks/post-merge
#!/bin/bash
# Update after merge operations
agit-mcp tool workspace_sync

# .git/hooks/post-rewrite  
#!/bin/bash
# Handle rebase, commit --amend
case "$1" in
  rebase) agit-mcp tool workspace_rewrite ;;
esac

# .git/hooks/post-commit
#!/bin/bash
# Update last_commit reliably
agit-mcp tool workspace_commit --hash $(git rev-parse HEAD)
```

### 8.2 Hook Installation Strategy
- `agit hook install` installs all required hooks
- `agit hook uninstall` removes AGIT hooks (preserves others)
- Hook scripts call `agit-mcp` with appropriate tools
- Include hook templates in repository

## 9. MVP Deliverables (Phased Implementation)

### Phase 0: Core Infrastructure (Week 1)
1. `go.mod` with SQLite dependency
2. `internal/db/` package with migration system
3. `agit init` - Creates `.git/agit.db` with versioned schema
4. `agit hook install` - Installs basic hook scaffold

### Phase 1: MCP Server Foundation (Week 2)
1. `cmd/agit-mcp/main.go` with stdio server
2. `mark3labs/mcp-go` integration
3. Tools: `workspace_status`, `workspace_list`
4. Tools: `task_new`, `task_list`, `task_start`

### Phase 2: Task Lifecycle (Week 3)
1. Enhanced task statuses (pending/active/done/failed/blocked/skipped)
2. Tool: `task_done` with `--skip-test` option
3. Basic test execution stub
4. `agit status` CLI command

### Phase 3: Checkpoint System (Week 4)
1. Tool: `checkpoint` with dirty working tree check
2. Tool: `restore` with `--force` and `--hard` flags
3. Agent state JSON schema v1.0
4. Complete Git hook coverage

### Phase 4: Test Integration (Week 5)
1. Tool: `task_done` with real test execution
2. Trace storage with command/output/duration
3. Tool: `trace` for test history viewing
4. Example integrations (Claude Code, OpenCode configs)

### Phase 5: Polish and Documentation (Week 6)
1. `agit task next` CLI command
2. Multi-worktree support detection
3. Comprehensive README and examples
4. Self-hosting demonstration

## 10. Open Questions for Implementation Decision

### 10.1 Architectural Decisions
1. **Workspace Identity:** Should we use UUIDs or composite (worktree+branch) keys?
   - *Recommendation:* UUID primary key with (branch, worktree_path) unique constraint

2. **MCP vs CLI First:** Implement standalone CLI or MCP-only in Phase 1?
   - *Recommendation:* Both from start - CLI wraps MCP tools for developer experience

3. **Restore Default Behavior:** Destructive (`--hard`) or safe (`--mixed`)?
   - *Recommendation:* Safe default (`--mixed`), require `--hard` for destructive

4. **Multi-Worktree Priority:** Support now or defer?
   - *Recommendation:* Design for it now (worktree_path field), implement fully in Phase 5

### 10.2 Technical Decisions
5. **Concurrency Model:** Optimistic locking vs. SQLite busy timeout?
   - *Recommendation:* SQLite WAL mode + busy timeout for Phase 1, add locking later

6. **Agent State Schema:** Minimal fields vs. extensible blob?
   - *Recommendation:* Versioned minimal schema with reserved `custom_fields`

7. **Test Output Storage:** In database or external files?
   - *Recommendation:* Truncated output in DB + optional external log path

8. **Migration Strategy:** SQL scripts vs. Go migration functions?
   - *Recommendation:* Go functions for complex migrations, simple SQL for schema changes

## 11. Conclusion

AGIT presents a compelling solution to the agent state persistence problem with its Git-native approach. The design is 90% complete and ready for implementation with the refinements outlined above.

**Immediate next actions:**
1. Create repository with revised schema (including `workspace_id` and `schema_meta`)
2. Implement `agit init` with migration system
3. Build `agit-mcp` server with `workspace_status` and `task_new` tools
4. Define and document agent state JSON schema v1.0

**Success metrics for MVP:**
- Agent can maintain context across Git branch switches
- Tasks can be created, completed, and tracked with tests
- Checkpoints provide reliable rollback points
- Works with at least one MCP client (Claude Code recommended)

The greatest risk remains the workspace identity problem - addressing this with `workspace_id` before any implementation is critical to avoid costly refactoring later.

With these improvements, AGIT will provide a robust, scalable foundation for stateful agent workflows while maintaining the simplicity and elegance of the original vision.
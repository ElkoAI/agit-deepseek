# AGIT MCP Tools Reference

AGIT (Agent Git) is a Model Context Protocol (MCP) server that provides Git-native state persistence for AI coding agents. It enables agents to maintain context across sessions, track tasks with tests, create checkpoints (Git commits + agent state), and restore to previous states.

## Overview

The AGIT MCP server exposes 8 tools that allow AI agents to:

- **Track workspaces**: Manage agent context tied to Git branches/worktrees
- **Manage tasks**: Create, start, complete, and list development tasks
- **Execute tests**: Run test commands associated with tasks
- **Create checkpoints**: Save Git commits + agent state snapshots
- **Restore state**: Roll back to previous checkpoints
- **Trace execution**: View test run history

All state is stored in `.git/agit.db` - never pollutes your working tree.

## Tool Reference

### `workspace_status`

Get current workspace state including active task, task counts, and last checkpoint.

**Parameters:** None

**Returns:**
```json
{
  "workspace": {
    "workspace_id": "string",
    "branch": "string",
    "worktree_path": "string | null",
    "display_name": "string | null",
    "current_task_id": "number | null",
    "agent_state": "string | null",
    "todo_list": "string | null",
    "last_commit": "string",
    "created_at": "string",
    "updated_at": "string"
  },
  "active_task": {
    "id": "number",
    "workspace_id": "string",
    "description": "string",
    "status": "pending | active | done | failed | blocked | skipped",
    "test_command": "string | null",
    "test_result": "string | null",
    "priority": "number",
    "parent_task_id": "number | null",
    "created_at": "string",
    "completed_at": "string | null"
  } | null,
  "task_counts": {
    "pending": "number",
    "active": "number",
    "done": "number",
    "failed": "number",
    "blocked": "number",
    "skipped": "number"
  },
  "last_checkpoint": {
    "id": "number",
    "workspace_id": "string",
    "commit_hash": "string",
    "agent_state": "string | null",
    "message": "string",
    "was_clean": "boolean",
    "created_at": "string"
  } | null
}
```

**Example Request:**
```json
{}
```

**Example Response:**
```json
{
  "content": [
    {
      "type": "text",
      "text": "{\n  \"workspace\": {\n    \"workspace_id\": \"def40947953dc22c3353942246e5109c\",\n    \"branch\": \"main\",\n    \"worktree_path\": null,\n    \"display_name\": \"main\",\n    \"current_task_id\": null,\n    \"agent_state\": null,\n    \"todo_list\": null,\n    \"last_commit\": \"a1b2c3d4\",\n    \"created_at\": \"2026-02-18T10:06:51Z\",\n    \"updated_at\": \"2026-02-18T10:06:51Z\"\n  },\n  \"active_task\": null,\n  \"task_counts\": {\n    \"pending\": 2,\n    \"active\": 1,\n    \"done\": 3,\n    \"failed\": 0,\n    \"blocked\": 0,\n    \"skipped\": 0\n  },\n  \"last_checkpoint\": {\n    \"id\": 5,\n    \"workspace_id\": \"def40947953dc22c3353942246e5109c\",\n    \"commit_hash\": \"6a44b4b0\",\n    \"agent_state\": \"{\\\"version\\\": \\\"1.0\\\", \\\"session_notes\\\": \\\"Checkpoint created\\\"}\",\n    \"message\": \"Before refactoring\",\n    \"was_clean\": false,\n    \"created_at\": \"2026-02-18T10:12:56Z\"\n  }\n}"
    }
  ]
}
```

---

### `task_new`

Create a new task with optional test command.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `description` | string | Yes | Description of the task |
| `test_command` | string | No | Optional test command to run when task is done |

**Returns:** The created task object

**Example Request:**
```json
{
  "description": "Implement user authentication middleware",
  "test_command": "go test ./internal/auth -v"
}
```

**Example Response:**
```json
{
  "content": [
    {
      "type": "text",
      "text": "Created task #12: Implement user authentication middleware\n{\n  \"id\": 12,\n  \"workspace_id\": \"def40947953dc22c3353942246e5109c\",\n  \"description\": \"Implement user authentication middleware\",\n  \"status\": \"pending\",\n  \"test_command\": \"go test ./internal/auth -v\",\n  \"test_result\": null,\n  \"priority\": 0,\n  \"parent_task_id\": null,\n  \"created_at\": \"2026-02-18T10:15:30Z\",\n  \"completed_at\": null\n}"
    }
  ]
}
```

---

### `task_list`

List tasks for current workspace with optional filtering.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `status` | string | No | Filter by status: `pending`, `active`, `done`, `failed`, `blocked`, `skipped` |
| `limit` | integer | No | Maximum number of tasks to return |

**Returns:** Array of task objects

**Example Request (pending tasks only):**
```json
{
  "status": "pending",
  "limit": 5
}
```

**Example Response:**
```json
{
  "content": [
    {
      "type": "text",
      "text": "Found 3 task(s):\n[\n  {\n    \"id\": 12,\n    \"workspace_id\": \"def40947953dc22c3353942246e5109c\",\n    \"description\": \"Implement user authentication middleware\",\n    \"status\": \"pending\",\n    \"test_command\": \"go test ./internal/auth -v\",\n    \"test_result\": null,\n    \"priority\": 0,\n    \"parent_task_id\": null,\n    \"created_at\": \"2026-02-18T10:15:30Z\",\n    \"completed_at\": null\n  },\n  {\n    \"id\": 13,\n    \"workspace_id\": \"def40947953dc22c3353942246e5109c\",\n    \"description\": \"Fix login form validation\",\n    \"status\": \"pending\",\n    \"test_command\": \"npm test -- login.test.js\",\n    \"test_result\": null,\n    \"priority\": 0,\n    \"parent_task_id\": null,\n    \"created_at\": \"2026-02-18T10:16:45Z\",\n    \"completed_at\": null\n  }\n]"
    }
  ]
}
```

---

### `task_start`

Mark a task as active and set it as current task for the workspace.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task_id` | integer | Yes | ID of the task to start |

**Returns:** Confirmation message

**Example Request:**
```json
{
  "task_id": 12
}
```

**Example Response:**
```json
{
  "content": [
    {
      "type": "text",
      "text": "Started task #12: Implement user authentication middleware"
    }
  ]
}
```

**Note:** Only one task can be active per workspace at a time. Starting a new task will automatically deactivate any previously active task.

---

### `task_done`

Complete a task and optionally run its test command.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task_id` | integer | Yes | ID of the task to complete |
| `skip_test` | boolean | No | Skip running tests even if test command is defined (default: `false`) |

**Returns:** Completion message with test results if applicable

**Example Request (with test):**
```json
{
  "task_id": 12
}
```

**Example Request (skip test):**
```json
{
  "task_id": 12,
  "skip_test": true
}
```

**Example Response (test passed):**
```json
{
  "content": [
    {
      "type": "text",
      "text": "Completed task #12: Implement user authentication middleware\nTest passed (exit code: 0, duration: 1.234s)"
    }
  ]
}
```

**Example Response (test failed):**
```json
{
  "content": [
    {
      "type": "text",
      "text": "Completed task #12: Implement user authentication middleware\nTest failed (exit code: 1, duration: 0.456s)"
    }
  ]
}
```

**Test Execution Details:**
- Tests are executed with a 5-minute timeout
- Output is truncated to 1000 characters for storage
- Exit codes are captured and stored
- Test results are recorded in the `traces` table
- Special exit codes: `-1` = timeout, `-2` = execution error

---

### `checkpoint`

Save current state and create a Git commit.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `message` | string | Yes | Commit message for the checkpoint |
| `allow_dirty` | boolean | No | Allow checkpoint even with uncommitted changes (default: `false`) |

**Returns:** Checkpoint information

**Example Request (clean tree):**
```json
{
  "message": "Before refactoring authentication system",
  "allow_dirty": false
}
```

**Example Request (allow dirty):**
```json
{
  "message": "WIP: Authentication refactor",
  "allow_dirty": true
}
```

**Example Response:**
```json
{
  "content": [
    {
      "type": "text",
      "text": "Created checkpoint #6 at commit 7b8c9d0a: Before refactoring authentication system"
    }
  ]
}
```

**Safety Features:**
- By default, checkpoints fail if the working tree has uncommitted changes
- Use `allow_dirty: true` to force checkpoint with uncommitted changes
- Agent state is serialized and stored with the commit
- Checkpoints include whether the tree was clean at creation time

---

### `restore`

Restore to the last checkpoint.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `force` | boolean | No | Force restore even with uncommitted changes (default: `false`) |
| `hard` | boolean | No | Perform hard reset (discard working tree changes) (default: `false`) |

**Returns:** Restoration confirmation

**Example Request (safe restore):**
```json
{
  "force": false,
  "hard": false
}
```

**Example Request (force hard restore):**
```json
{
  "force": true,
  "hard": true
}
```

**Example Response:**
```json
{
  "content": [
    {
      "type": "text",
      "text": "Restored to checkpoint #6 at commit 7b8c9d0a: Before refactoring authentication system"
    }
  ]
}
```

**Reset Modes:**
- **Mixed (default)**: Reset HEAD to checkpoint, keep working tree changes staged
- **Hard**: Reset HEAD and discard all working tree changes (dangerous!)
- **Soft**: Reset HEAD only, keep working tree and index unchanged

**Safety Features:**
- By default, restore fails if working tree has uncommitted changes
- Use `force: true` to override safety check
- Agent state is restored from checkpoint
- Workspace is updated after restore

---

### `trace`

Show test execution history for a task.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task_id` | integer | Yes | ID of the task to show traces for |
| `limit` | integer | No | Maximum number of traces to return |

**Returns:** Array of trace objects

**Example Request:**
```json
{
  "task_id": 12,
  "limit": 10
}
```

**Example Response:**
```json
{
  "content": [
    {
      "type": "text",
      "text": "Found 2 trace(s) for task #12:\n[\n  {\n    \"id\": 3,\n    \"task_id\": 12,\n    \"command\": \"go test ./internal/auth -v\",\n    \"exit_code\": 1,\n    \"output_truncated\": \"FAIL: TestAuthentication...\",\n    \"log_path\": null,\n    \"duration_ms\": 1234,\n    \"run_at\": \"2026-02-18T10:20:30Z\"\n  },\n  {\n    \"id\": 4,\n    \"task_id\": 12,\n    \"command\": \"go test ./internal/auth -v\",\n    \"exit_code\": 0,\n    \"output_truncated\": \"PASS: TestAuthentication...\",\n    \"log_path\": null,\n    \"duration_ms\": 1456,\n    \"run_at\": \"2026-02-18T10:25:45Z\"\n  }\n]"
    }
  ]
}
```

**Trace Fields:**
- `command`: The test command that was executed
- `exit_code`: Exit code (0 = success, non-zero = failure, -1 = timeout, -2 = error)
- `output_truncated`: First 1000 characters of test output
- `log_path`: Path to full output file (optional, currently not implemented)
- `duration_ms`: Execution time in milliseconds
- `run_at`: When the test was executed

---

## Error Handling

AGIT MCP server returns structured error messages for common failure cases:

### Common Error Scenarios

1. **Not in a Git repository**
   ```
   "failed to get database path: not in a git repository"
   ```

2. **Task not found or wrong workspace**
   ```
   "task 12 does not belong to current workspace"
   ```

3. **Dirty working tree during checkpoint**
   ```
   "working tree has uncommitted changes. Use allow_dirty=true to proceed"
   ```

4. **Dirty working tree during restore**
   ```
   "working tree has uncommitted changes. Use force=true to proceed"
   ```

5. **No checkpoints available**
   ```
   "no checkpoints found for this workspace"
   ```

6. **Test execution failure**
   ```
   "failed to execute test command: exec: \"nonexistent\": executable file not found in $PATH"
   ```

7. **Database migration errors**
   ```
   "legacy database detected - requires manual migration. Run 'agit migrate-legacy'"
   ```

### Error Response Format
All errors are returned as JSON-RPC error responses with appropriate error codes and messages.

---

## Setup and Configuration

### Installation

```bash
# Clone and build
git clone <repository-url>
cd agit
go build -o agit ./cmd/agit
go build -o agit-mcp ./cmd/agit-mcp

# Install to PATH (optional)
sudo cp agit agit-mcp /usr/local/bin/
```

### MCP Client Configuration

**Claude Code** (`~/.config/claude-code/mcp.json`):
```json
{
  "mcpServers": {
    "agit": {
      "command": "agit-mcp",
      "args": ["--stdio"]
    }
  }
}
```

**OpenCode** (`~/.opencode/plugins/agit.json`):
```json
{
  "name": "agit",
  "mcp": {
    "server": {
      "command": "agit-mcp",
      "args": ["--stdio"]
    }
  }
}
```

### Initialization

1. **Initialize AGIT in your repository**:
   ```bash
   agit init
   ```

2. **Check workspace status**:
   ```bash
   agit status
   ```

3. **Install Git hooks (optional)**:
   ```bash
   agit hook install
   ```

---

## Best Practices for AI Agents

### Task Management
1. **Start with `workspace_status`** to understand current context
2. **Create tasks with descriptive names** and test commands when applicable
3. **Start tasks before working on them** to track active context
4. **Complete tasks with tests** to maintain test-driven workflow

### Checkpoint Strategy
1. **Create checkpoints before major changes** (refactoring, risky operations)
2. **Use descriptive checkpoint messages** that explain the state
3. **Consider `allow_dirty: true` for WIP checkpoints** during active development
4. **Restore when experiments fail** to recover known-good state

### Test Integration
1. **Associate meaningful test commands** with each task
2. **Run tests when completing tasks** to validate work
3. **Check trace history** to understand test failures
4. **Use `skip_test: true` for documentation/design tasks**

### Workspace Awareness
1. **AGIT automatically tracks Git branch changes** via hooks
2. **Each branch gets its own workspace** with separate context
3. **Agent state persists across sessions** for the same workspace
4. **Workspace UUIDs are stable** (not dependent on branch names)

---

## Architecture Notes

### Data Storage
- All data stored in `.git/agit.db` (SQLite)
- Never pollutes working tree
- Protected by Git's permission model
- Automatic schema migrations

### State Management
- Workspace = Git branch + worktree + agent context
- Agent state stored as versioned JSON
- Checkpoints = Git commits + agent state snapshots
- Tasks can have parent-child relationships

### Safety Features
- Checkpoint warnings for dirty trees
- Restore safety checks with force requirements
- Schema versioning from day one
- Foreign key constraints and cascading deletes

### Performance
- WAL mode for better concurrency
- Indexes on frequently queried columns
- Output truncation for large test results
- Pagination support for list operations
```

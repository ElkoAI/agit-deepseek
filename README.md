# AGIT: Agent Git - MCP Server for Stateful Agent Workflows

**Git-native state persistence for AI coding agents**

AGIT is a Go-based MCP server that provides Git-native state persistence for AI coding agents. By storing agent state in `.git/agit.db`, it creates a seamless bridge between Git's version control capabilities and agentic workflows, enabling test-driven development with persistent context across sessions.

## Key Features

- **Git-native storage**: All state stored in `.git/agit.db` - never pollutes your working tree
- **MCP-first design**: Works with Claude Code, OpenCode, Cursor, and any MCP-compatible agent
- **Stable workspace identity**: Uses `workspace_id` UUIDs, not branch names - survives renames and detached HEAD
- **Test-driven tasks**: Every task can include a test command with execution history
- **Safe checkpoints**: Git commits + agent state snapshots with safety guards
- **Schema versioning**: Built-in migration system from day one
- **Multi-worktree ready**: Designed to support Git worktrees from the start
## Development Status

**Current Version**: v0.1.0-alpha (Initial Commit: `b80bbac`)

AGIT is currently in active development with MVP foundations laid. The core database, CLI, and Git integration are implemented. The MCP server requires API compatibility fixes before full functionality.

**✅ Implemented**:
- Database schema with `workspace_id` UUIDs (not branch-dependent)
- Schema versioning and migration system
- Git operations integration
- Complete CLI with workspace/task/checkpoint management
- Safety guards for checkpoint/restore operations

**⚠️ Known Issues**:
- MCP server needs API compatibility fixes
- Test execution currently stubbed (requires implementation)
- Some edge cases in Git operations need testing

**📋 Next Steps**:
- Fix MCP server API compatibility
- Implement real test execution
- Add unit tests
- Integrate with MCP clients (Claude Code, OpenCode)

For detailed status, see [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md).

## Installation

### Prerequisites
- Go 1.21 or later
- Git

### Building from source

```bash
git clone <repository-url>
cd agit
go build -o agit ./cmd/agit
go build -o agit-mcp ./cmd/agit-mcp

# Install to PATH (optional)
sudo cp agit agit-mcp /usr/local/bin/
```

## Quick Start

1. **Initialize AGIT in your repository**:
   ```bash
   agit init
   ```

2. **Install Git hooks** (for automatic workspace tracking):
   ```bash
   agit hook install
   ```

3. **Check your workspace status**:
   ```bash
   agit status
   ```

4. **Create your first task**:
   ```bash
   agit task new "Implement feature X" --test "go test ./..."
   ```

5. **Start working on the task**:
   ```bash
   agit task start 1
   ```

6. **Create a checkpoint** (commit + state snapshot):
   ```bash
   agit checkpoint "Progress on feature X"
   ```

7. **Complete the task**:
   ```bash
   agit task done 1
   ```

## CLI Usage

### Core Commands

| Command | Description |
|---------|-------------|
| `agit init` | Initialize AGIT database in current repository |
| `agit status` | Show current workspace status |
| `agit task new <description>` | Create a new task (optionally with `--test`) |
| `agit task list` | List tasks (filter with `--status`) |
| `agit task start <id>` | Mark task as active |
| `agit task done <id>` | Complete task (use `--skip-test` to skip tests) |
| `agit task next` | Show highest priority pending task |
| `agit checkpoint <message>` | Create checkpoint (use `--allow-dirty` for dirty trees) |
| `agit restore` | Restore to last checkpoint (use `--force` and `--hard` flags) |
| `agit hook install` | Install Git hooks for automatic workspace tracking |
| `agit hook uninstall` | Remove AGIT Git hooks |
| `agit workspace list` | List workspaces |
| `agit version` | Show version |

### Workspace Management

AGIT creates workspaces based on Git branches and worktrees:
- Each workspace gets a stable `workspace_id` UUID
- Workspace state includes agent context, todo list, and last commit
- Automatic tracking via Git hooks (`post-checkout`, `post-merge`, `post-commit`)

## MCP Tools for AI Agents

AGIT exposes the following tools via MCP:

### `workspace_status`
Get current workspace state including active task, task counts, and last checkpoint.

### `task_new`
Create a new task with optional test command.
```json
{
  "description": "Implement method X",
  "test_command": "go test ./internal/parser"
}
```

### `task_list`
List tasks with optional status filter.

### `task_start`
Mark a task as active and set it as current task for the workspace.

### `task_done`
Complete a task and optionally run its test command (use `skip_test` parameter to skip).

### `checkpoint`
Save current state and create a Git commit.
```json
{
  "message": "Before refactoring",
  "allow_dirty": false
}
```

### `restore`
Restore to the last checkpoint.
```json
{
  "force": false,
  "hard": false
}
```

### `trace`
Show test execution history for a task.

## Architecture

### Data Model (SQLite in `.git/agit.db`)

```sql
-- Versioned schema with migration support
CREATE TABLE schema_meta (version, description, applied_at);

-- Workspaces with stable UUID identities
CREATE TABLE workspaces (
  workspace_id TEXT PRIMARY KEY,  -- UUID
  branch TEXT NOT NULL,
  worktree_path TEXT,
  agent_state TEXT,  -- JSON
  todo_list TEXT,
  last_commit TEXT
);

-- Tasks with enhanced statuses
CREATE TABLE tasks (
  id INTEGER PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  description TEXT NOT NULL,
  status TEXT CHECK(status IN ('pending','active','done','failed','blocked','skipped')),
  test_command TEXT,
  test_result TEXT,  -- JSON
  priority INTEGER DEFAULT 0
);

-- Checkpoints: Git commits + agent state
CREATE TABLE checkpoints (
  id INTEGER PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  commit_hash TEXT NOT NULL,
  agent_state TEXT,
  message TEXT,
  was_clean BOOLEAN
);

-- Test execution traces
CREATE TABLE traces (
  id INTEGER PRIMARY KEY,
  task_id INTEGER NOT NULL,
  command TEXT NOT NULL,
  exit_code INTEGER,
  output_truncated TEXT,
  duration_ms INTEGER
);
```

### Safety Features

1. **Checkpoint guards**: Warns about dirty working trees unless `--allow-dirty`
2. **Restore safety**: Requires `--force` for dirty trees, defaults to `--mixed` reset
3. **Schema migrations**: Automatic version tracking and migration path
4. **Foreign keys**: Cascading deletes for data consistency

## Configuration

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

## Development Status

**MVP (Phase 1-3 Complete)**:
- [x] Database schema with workspace_id
- [x] Schema versioning and migration system
- [x] Basic Git operations (branch, commit, reset)
- [x] MCP server with core tools
- [x] CLI with essential commands
- [x] Git hooks for automatic tracking

**Planned Features**:
- [ ] Real test execution (currently stubbed)
- [ ] Multi-worktree full support
- [ ] Agent state JSON schema validation
- [ ] Concurrency handling for multi-agent
- [ ] Remote sync (GitHub Issues, S3)
- [ ] Web dashboard

## Design Principles

1. **State in `.git/`**: Never pollute working tree
2. **MCP first**: One server, many clients
3. **Test-driven tasks**: Every task has optional test command
4. **Checkpoint everything**: Git commits + agent state
5. **Simple terms**: Workspace, task, checkpoint, trace
6. **Incremental commits**: Every phase is git-committable

## Critical Design Decisions

1. **Workspace Identity**: Using `workspace_id` UUIDs instead of branch names prevents issues with branch renames and detached HEAD states.
2. **Schema Versioning**: Migration system built-in from day one for future compatibility.
3. **Safety Defaults**: Checkpoints warn about dirty trees, restores require `--force` for safety.
4. **Agent State JSON**: Versioned minimal schema with extensible `custom_fields`.

## Troubleshooting

### "Not a git repository"
Run `agit init` from within a Git repository.

### "Database already exists"
AGIT is already initialized. Use `agit status` to check.

### Hooks not working
Ensure `agit-mcp` is in your PATH. Test with:
```bash
which agit-mcp
```

### MCP connection issues
Check your MCP client configuration and ensure the server starts correctly:
```bash
echo '{}' | agit-mcp
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes with tests
4. Submit a pull request

Please follow the existing code style and include tests for new features.

## License

MIT License - see LICENSE file for details.

---

**AGIT** - The missing link between Git's version control and agentic workflows.
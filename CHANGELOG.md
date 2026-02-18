# Changelog

All notable changes to AGIT (Agent Git) will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.1.0-alpha] - 2025-02-18

### Added
- **Database Schema**: SQLite schema with `workspace_id` UUIDs (not branch-dependent) for stable workspace identity
- **Schema Versioning**: Built-in migration system with `schema_meta` table from day one
- **Git Integration**: Basic Git operations for branch detection, commits, resets, and worktree detection
- **MCP Server Foundation**: Server structure with 8 tool definitions (requires API compatibility fixes)
- **CLI Implementation**: Complete command-line interface for workspace, task, and checkpoint management
- **Safety Features**: Checkpoint/restore operations with `--force` and `--hard` flags for safe defaults
- **Enhanced Task Lifecycle**: Tasks support optional test commands with `--skip-test` flag
- **Multi-worktree Design**: Schema includes `worktree_path` field for future multi-worktree support
- **Documentation**: Comprehensive README, design analysis, and implementation status documentation

### Database Features
- `workspaces` table with UUID-based identity (survives branch renames and detached HEAD)
- `tasks` table with enhanced statuses (pending, active, done, failed, blocked, skipped)
- `checkpoints` table for Git commits + agent state snapshots
- `traces` table for test execution history
- Foreign key constraints with cascading deletes
- Indexes for performance optimization
- SQLite WAL mode for better concurrency

### CLI Commands
- `agit init` - Initialize database in current Git repository
- `agit status` - Show workspace status with task counts
- `agit task new` - Create new task with optional test command
- `agit task list` - List tasks with status filtering
- `agit task start` / `agit task done` - Task lifecycle management
- `agit task next` - Show highest priority pending task
- `agit checkpoint` - Create checkpoint with Git commit (requires `--allow-dirty` for dirty trees)
- `agit restore` - Restore to last checkpoint (requires `--force` for dirty trees)
- `agit hook install` / `agit hook uninstall` - Git hook management
- `agit workspace list` - List workspaces
- `agit version` - Show version information

### MCP Tools (Foundation)
- `workspace_status` - Get current workspace state
- `task_new` - Create new task with optional test command
- `task_list` - List tasks for current workspace
- `task_start` - Mark task as active
- `task_done` - Complete task and optionally run tests
- `checkpoint` - Save state + git commit
- `restore` - Restore to last checkpoint
- `trace` - Show test history for a task

### Known Issues
- **MCP Server API Compatibility**: Uses incorrect field names (`Required` not in `ToolInputSchema`)
- **Missing `NewTextContent` Function**: Need to construct `TextContent` struct directly
- **TaskStatus References**: Need to use `db.TaskStatus` constants correctly
- **Test Execution**: Currently stubbed - requires real test command implementation
- **Multi-worktree Support**: Schema designed but not fully implemented
- **Agent State Validation**: JSON schema defined but no validation implemented
- **Concurrency Handling**: No locking mechanisms for multi-agent scenarios

### Security Considerations
- SQL injection prevention via parameterized queries
- Database stored in `.git/agit.db` (protected by Git permissions)
- Git hooks installed in `.git/hooks` (standard Git security model)

### Documentation
- `README.md` - User guide with installation and usage instructions
- `Design Analysis and Recommendations.md` - Critical design review addressing original spec issues
- `IMPLEMENTATION_STATUS.md` - Detailed implementation status and roadmap
- `.gitignore` - Properly configured for Go project

### Technical Details
- **Language**: Go 1.21+
- **Dependencies**: 
  - `github.com/mark3labs/mcp-go` v0.1.0 (MCP protocol)
  - `github.com/mattn/go-sqlite3` v1.14.22 (SQLite database)
- **Database**: SQLite with WAL mode and foreign key support
- **Architecture**: Modular design with separate packages for db, git, and mcp

**Commit**: `b80bbac`  
**Tag**: `v0.1.0-alpha`

---

## [Unreleased]

### Planned Features
- Fix MCP server API compatibility issues
- Implement real test command execution
- Add comprehensive unit tests
- Implement multi-worktree full support
- Add agent state JSON schema validation
- Implement concurrency locking for multi-agent scenarios
- Add workspace listing and switching via MCP
- Implement remote sync (GitHub Issues, S3 backup)
- Create VS Code/Cursor extension
- Add CI/CD pipeline templates

[v0.1.0-alpha]: https://github.com/agit/agit/commit/b80bbac
package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitError represents an error from a git command
type GitError struct {
	Command string
	Err     error
	Stderr  string
}

func (e *GitError) Error() string {
	return fmt.Sprintf("git %s failed: %v\n%s", e.Command, e.Err, e.Stderr)
}

func (e *GitError) Unwrap() error {
	return e.Err
}

// runGit executes a git command and returns stdout/stderr
func runGit(args ...string) (string, string, error) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// GetCurrentBranch returns the current branch name.
// Returns "detached-<short-hash>" if in detached HEAD state.
func GetCurrentBranch() (string, error) {
	stdout, stderr, err := runGit("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", &GitError{Command: "rev-parse --abbrev-ref HEAD", Err: err, Stderr: stderr}
	}

	branch := strings.TrimSpace(stdout)
	// git returns "HEAD" when in detached state
	if branch == "HEAD" {
		// Get the commit hash for this detached state
		hash, err := GetCurrentCommitHash()
		if err != nil {
			// Fallback to generic placeholder
			return "detached-HEAD", nil
		}
		// Use first 8 characters of hash for brevity
		if len(hash) >= 8 {
			return "detached-" + hash[:8], nil
		}
		return "detached-" + hash, nil
	}
	return branch, nil
}

// GetCurrentCommitHash returns the current commit hash
func GetCurrentCommitHash() (string, error) {
	stdout, stderr, err := runGit("rev-parse", "HEAD")
	if err != nil {
		return "", &GitError{Command: "rev-parse HEAD", Err: err, Stderr: stderr}
	}
	return strings.TrimSpace(stdout), nil
}

// GetWorktreePath returns the worktree path if in a linked worktree.
// Returns empty string for the main worktree.
func GetWorktreePath() (string, error) {
	// Check if we're in a worktree by looking for .git file
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	gitPath := filepath.Join(cwd, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat .git: %w", err)
	}

	// If .git is a file, we're in a worktree
	if !info.IsDir() {
		// Read the gitdir file to find the worktree path
		content, err := os.ReadFile(gitPath)
		if err != nil {
			return "", fmt.Errorf("failed to read .git file: %w", err)
		}

		// Format is "gitdir: /path/to/.git/worktrees/name"
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "gitdir: ") {
				gitdir := strings.TrimPrefix(line, "gitdir: ")
				gitdir = strings.TrimSpace(gitdir)

				// Extract worktree name from path
				if strings.Contains(gitdir, "/.git/worktrees/") {
					// This is a worktree
					return cwd, nil
				}
			}
		}
	}

	// Not in a worktree or main worktree
	return "", nil
}

// IsWorkingTreeDirty checks if there are uncommitted changes
func IsWorkingTreeDirty() (bool, error) {
	stdout, stderr, err := runGit("status", "--porcelain")
	if err != nil {
		return false, &GitError{Command: "status --porcelain", Err: err, Stderr: stderr}
	}

	return stdout != "", nil
}

// Reset performs a git reset to the specified commit
// mode can be "--hard", "--mixed", or "--soft"
func Reset(commitHash, mode string) error {
	if mode != "--hard" && mode != "--mixed" && mode != "--soft" {
		return fmt.Errorf("invalid reset mode: %s", mode)
	}

	_, stderr, err := runGit("reset", mode, commitHash)
	if err != nil {
		return &GitError{Command: fmt.Sprintf("reset %s %s", mode, commitHash), Err: err, Stderr: stderr}
	}
	return nil
}

// CommitAll stages all changes and creates a commit with the given message
func CommitAll(message string) (string, error) {
	// Stage all changes
	_, stderr, err := runGit("add", "-A")
	if err != nil {
		return "", &GitError{Command: "add -A", Err: err, Stderr: stderr}
	}

	// Create commit
	stdout, stderr, err := runGit("commit", "-m", message)
	if err != nil {
		return "", &GitError{Command: fmt.Sprintf("commit -m \"%s\"", message), Err: err, Stderr: stderr}
	}

	// Get the new commit hash
	commitHash, err := GetCurrentCommitHash()
	if err != nil {
		return "", fmt.Errorf("failed to get new commit hash: %w", err)
	}

	// Also capture the commit output for debugging
	_ = stdout

	return commitHash, nil
}

// GetGitCommonDir returns the git common directory path
// For main repository: <repo>/.git
// For worktrees: <repo>/.git/worktrees/<name>
func GetGitCommonDir() (string, error) {
	stdout, stderr, err := runGit("rev-parse", "--git-common-dir")
	if err != nil {
		return "", &GitError{Command: "rev-parse --git-common-dir", Err: err, Stderr: stderr}
	}

	path := strings.TrimSpace(stdout)
	// Make path absolute
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
		path = filepath.Join(cwd, path)
	}

	return path, nil
}

// GetGitDir returns the git directory path
func GetGitDir() (string, error) {
	stdout, stderr, err := runGit("rev-parse", "--git-dir")
	if err != nil {
		return "", &GitError{Command: "rev-parse --git-dir", Err: err, Stderr: stderr}
	}

	path := strings.TrimSpace(stdout)
	// Make path absolute
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
		path = filepath.Join(cwd, path)
	}

	return path, nil
}

// FindGitRepo walks up from the current directory to find a .git directory
func FindGitRepo(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	for {
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("not a git repository")
}

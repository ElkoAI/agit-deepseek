package exec

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// TestResult represents the result of a test execution
type TestResult struct {
	ExitCode   int           `json:"exit_code"`
	Output     string        `json:"output"`
	Duration   time.Duration `json:"duration"`
	Passed     bool          `json:"passed"`
	Command    string        `json:"command"`
	RunAt      time.Time     `json:"run_at"`
	TimedOut   bool          `json:"timed_out,omitempty"`
}

// RunTest executes a shell command and returns the result.
// The command is executed in the specified working directory.
// If timeout is 0, no timeout is applied.
func RunTest(ctx context.Context, command string, workDir string, timeout time.Duration) (*TestResult, error) {
	start := time.Now()
	result := &TestResult{
		Command: command,
		RunAt:   start,
	}

	// Create a context with timeout if specified
	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Parse command - support simple commands for now
	// In the future, we might want to support shell features like pipes, redirects
	cmdParts := strings.Fields(command)
	if len(cmdParts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	cmd := exec.CommandContext(runCtx, cmdParts[0], cmdParts[1:]...)
	cmd.Dir = workDir

	var outputBuf bytes.Buffer
	cmd.Stdout = &outputBuf
	cmd.Stderr = &outputBuf

	err := cmd.Run()
	result.Duration = time.Since(start)

	// Capture output
	result.Output = outputBuf.String()

	// Determine exit code and success
	if err != nil {
		// Check if it was a timeout
		if runCtx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.ExitCode = -1 // Special code for timeout
			result.Passed = false
			return result, nil
		}

		// Try to get exit code from exec.ExitError
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			result.Passed = false
			return result, nil
		}

		// Some other error (e.g., command not found)
		result.ExitCode = -2 // Special code for other errors
		result.Passed = false
		return result, fmt.Errorf("failed to execute command: %w", err)
	}

	// Command succeeded
	result.ExitCode = 0
	result.Passed = true
	return result, nil
}

// TruncateOutput truncates output to a maximum length, adding an ellipsis if truncated.
// This is useful for storing in the database where output might be large.
func TruncateOutput(output string, maxLength int) string {
	if len(output) <= maxLength {
		return output
	}
	if maxLength <= 3 {
		return "..."
	}
	return output[:maxLength-3] + "..."
}

package supervisor

import (
	"context"

	"github.com/swamp-dev/agentbox/internal/ralph"
)

// AgentRunner defines the interface for executing a task via an agent.
// The default implementation wraps ralph.Loop.RunSingleTask().
// Tests can provide a mock implementation.
type AgentRunner interface {
	RunTask(ctx context.Context, task *ralph.Task, prompt string) *ralph.IterationResult
}

// RalphAgentRunner adapts ralph.Loop to the AgentRunner interface.
type RalphAgentRunner struct {
	loop *ralph.Loop
}

// NewRalphAgentRunner creates an AgentRunner backed by a ralph loop.
func NewRalphAgentRunner(loop *ralph.Loop) *RalphAgentRunner {
	return &RalphAgentRunner{loop: loop}
}

// RunTask executes a task using the ralph loop.
func (r *RalphAgentRunner) RunTask(ctx context.Context, task *ralph.Task, prompt string) *ralph.IterationResult {
	return r.loop.RunSingleTask(ctx, task, prompt)
}

// NoopAgentRunner is a stub that always returns failure.
// Used when no real agent is configured (e.g., dry-run mode or testing).
type NoopAgentRunner struct{}

// RunTask returns a failure result without executing anything.
func (n *NoopAgentRunner) RunTask(_ context.Context, task *ralph.Task, _ string) *ralph.IterationResult {
	return &ralph.IterationResult{
		TaskID:  task.ID,
		Success: false,
		Error:   "no agent runner configured",
	}
}

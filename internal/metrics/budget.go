package metrics

import (
	"fmt"
	"time"
)

// Budget defines resource limits for a supervisor session.
type Budget struct {
	MaxTokens     int           `json:"max_tokens" yaml:"max_tokens"`
	MaxDuration   time.Duration `json:"max_duration" yaml:"max_duration"`
	MaxIterations int           `json:"max_iterations" yaml:"max_iterations"`
	WarnThreshold float64       `json:"warn_threshold" yaml:"warn_threshold"` // 0.0-1.0, default 0.8
}

// DefaultBudget returns a budget with sensible defaults.
func DefaultBudget() Budget {
	return Budget{
		MaxTokens:     1000000,
		MaxDuration:   8 * time.Hour,
		MaxIterations: 100,
		WarnThreshold: 0.8,
	}
}

// BudgetStatus represents the current budget consumption state.
type BudgetStatus struct {
	TokensUsed     int           `json:"tokens_used"`
	TokensMax      int           `json:"tokens_max"`
	DurationUsed   time.Duration `json:"duration_used"`
	DurationMax    time.Duration `json:"duration_max"`
	IterationsUsed int           `json:"iterations_used"`
	IterationsMax  int           `json:"iterations_max"`
	Warning        bool          `json:"warning"`
	Exceeded       bool          `json:"exceeded"`
	Reason         string        `json:"reason,omitempty"`
}

// BudgetEnforcer tracks resource consumption against limits.
type BudgetEnforcer struct {
	budget    Budget
	startTime time.Time
}

// NewBudgetEnforcer creates a new enforcer with the given budget.
func NewBudgetEnforcer(budget Budget) *BudgetEnforcer {
	if budget.WarnThreshold == 0 {
		budget.WarnThreshold = 0.8
	}
	return &BudgetEnforcer{
		budget:    budget,
		startTime: time.Now(),
	}
}

// Check evaluates current consumption against the budget.
func (e *BudgetEnforcer) Check(tokensUsed, iterationsUsed int) *BudgetStatus {
	elapsed := time.Since(e.startTime)

	status := &BudgetStatus{
		TokensUsed:     tokensUsed,
		TokensMax:      e.budget.MaxTokens,
		DurationUsed:   elapsed,
		DurationMax:    e.budget.MaxDuration,
		IterationsUsed: iterationsUsed,
		IterationsMax:  e.budget.MaxIterations,
	}

	// Check exceeded.
	if e.budget.MaxTokens > 0 && tokensUsed >= e.budget.MaxTokens {
		status.Exceeded = true
		status.Reason = fmt.Sprintf("token budget exceeded: %d/%d", tokensUsed, e.budget.MaxTokens)
		return status
	}
	if e.budget.MaxDuration > 0 && elapsed >= e.budget.MaxDuration {
		status.Exceeded = true
		status.Reason = fmt.Sprintf("duration budget exceeded: %s/%s", elapsed.Round(time.Second), e.budget.MaxDuration)
		return status
	}
	if e.budget.MaxIterations > 0 && iterationsUsed >= e.budget.MaxIterations {
		status.Exceeded = true
		status.Reason = fmt.Sprintf("iteration budget exceeded: %d/%d", iterationsUsed, e.budget.MaxIterations)
		return status
	}

	// Check warnings.
	threshold := e.budget.WarnThreshold
	if e.budget.MaxTokens > 0 && float64(tokensUsed) >= float64(e.budget.MaxTokens)*threshold {
		status.Warning = true
		status.Reason = fmt.Sprintf("approaching token limit: %d/%d (%.0f%%)", tokensUsed, e.budget.MaxTokens, float64(tokensUsed)/float64(e.budget.MaxTokens)*100)
	}
	if e.budget.MaxDuration > 0 && float64(elapsed) >= float64(e.budget.MaxDuration)*threshold {
		status.Warning = true
		status.Reason = fmt.Sprintf("approaching duration limit: %s/%s", elapsed.Round(time.Second), e.budget.MaxDuration)
	}
	if e.budget.MaxIterations > 0 && float64(iterationsUsed) >= float64(e.budget.MaxIterations)*threshold {
		status.Warning = true
		status.Reason = fmt.Sprintf("approaching iteration limit: %d/%d", iterationsUsed, e.budget.MaxIterations)
	}

	return status
}

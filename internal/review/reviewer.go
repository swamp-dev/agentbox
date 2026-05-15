// Package review provides code review orchestration via a separate agent.
package review

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/swamp-dev/agentbox/internal/agent"
	"github.com/swamp-dev/agentbox/internal/config"
	"github.com/swamp-dev/agentbox/internal/container"
)

// ReviewSeverity categorizes the impact of a finding.
type ReviewSeverity string

const (
	SeverityCritical    ReviewSeverity = "critical"
	SeveritySignificant ReviewSeverity = "significant"
	SeverityMinor       ReviewSeverity = "minor"
	SeverityNit         ReviewSeverity = "nit"
)

// ReviewFinding represents a single issue found during review.
type ReviewFinding struct {
	Severity    ReviewSeverity `json:"severity"`
	File        string         `json:"file"`
	Line        int            `json:"line,omitempty"`
	Description string         `json:"description"`
	Suggestion  string         `json:"suggestion,omitempty"`
}

// ReviewResult holds the complete review outcome.
type ReviewResult struct {
	Findings    []ReviewFinding `json:"findings"`
	Summary     string          `json:"summary"`
	Approved    bool            `json:"approved"`
	ReviewedAt  time.Time       `json:"reviewed_at"`
	ReviewAgent string          `json:"review_agent"`
}

// HasBlockers returns true if there are critical or significant findings.
func (r *ReviewResult) HasBlockers() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityCritical || f.Severity == SeveritySignificant {
			return true
		}
	}
	return false
}

// BlockerFindings returns only critical and significant findings.
func (r *ReviewResult) BlockerFindings() []ReviewFinding {
	var blockers []ReviewFinding
	for _, f := range r.Findings {
		if f.Severity == SeverityCritical || f.Severity == SeveritySignificant {
			blockers = append(blockers, f)
		}
	}
	return blockers
}

// CountBySeverity returns finding counts grouped by severity.
func (r *ReviewResult) CountBySeverity() map[ReviewSeverity]int {
	counts := make(map[ReviewSeverity]int)
	for _, f := range r.Findings {
		counts[f.Severity]++
	}
	return counts
}

// Reviewer orchestrates code review using a separate agent session.
type Reviewer struct {
	agentName string
	cfg       *config.Config
	container *container.Manager
	logger    *slog.Logger
}

// NewReviewer creates a new Reviewer.
func NewReviewer(agentName string, cfg *config.Config, cm *container.Manager, logger *slog.Logger) *Reviewer {
	return &Reviewer{
		agentName: agentName,
		cfg:       cfg,
		container: cm,
		logger:    logger,
	}
}

// Review runs the review agent on the current diff.
func (r *Reviewer) Review(ctx context.Context, projectPath, diff string, changedFiles []string, testSummary string) (*ReviewResult, error) {
	prompt := r.buildPrompt(diff, changedFiles, testSummary)

	ag, err := agent.New(r.agentName)
	if err != nil {
		return nil, fmt.Errorf("creating review agent: %w", err)
	}

	cmd := ag.Command(prompt)
	env := ag.Environment()

	containerCfg, err := container.ConfigToContainerConfig(r.cfg, projectPath, cmd, env)
	if err != nil {
		return nil, fmt.Errorf("configuring review container: %w", err)
	}
	containerCfg.Name = fmt.Sprintf("agentbox-review-%s", r.agentName)

	r.logger.Info("running code review", "agent", r.agentName)

	output, err := r.container.Run(ctx, containerCfg)
	if err != nil {
		return nil, fmt.Errorf("running review agent: %w", err)
	}

	return r.parseReviewOutput(output)
}

// buildPrompt constructs the review prompt.
func (r *Reviewer) buildPrompt(diff string, changedFiles []string, testSummary string) string {
	var sb strings.Builder

	sb.WriteString("You are a code reviewer. Review the following changes carefully.\n\n")
	sb.WriteString("Focus on:\n")
	sb.WriteString("1. Bugs and logic errors\n")
	sb.WriteString("2. Security vulnerabilities\n")
	sb.WriteString("3. Test coverage gaps\n")
	sb.WriteString("4. Architectural issues\n")
	sb.WriteString("5. Performance problems\n\n")

	sb.WriteString("Changed files:\n")
	for _, f := range changedFiles {
		sb.WriteString(fmt.Sprintf("- %s\n", f))
	}
	sb.WriteString("\n")

	if testSummary != "" {
		sb.WriteString("Test results:\n")
		sb.WriteString(testSummary)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Diff:\n```\n")
	// Truncate very large diffs.
	if len(diff) > 50000 {
		sb.WriteString(diff[:50000])
		sb.WriteString("\n... (truncated)\n")
	} else {
		sb.WriteString(diff)
	}
	sb.WriteString("```\n\n")

	sb.WriteString("Respond with JSON only:\n")
	sb.WriteString(`{
  "findings": [
    {"severity": "critical|significant|minor|nit", "file": "path", "line": N, "description": "...", "suggestion": "..."}
  ],
  "summary": "Overall assessment",
  "approved": true|false
}`)
	sb.WriteString("\n\napproved=true means no critical or significant issues.\n")

	return sb.String()
}

// parseReviewOutput extracts ReviewResult from agent output.
func (r *Reviewer) parseReviewOutput(output string) (*ReviewResult, error) {
	// Try to find JSON in the output.
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return &ReviewResult{
			Summary:     "Could not parse review output",
			Approved:    false,
			ReviewedAt:  time.Now(),
			ReviewAgent: r.agentName,
		}, nil
	}

	result := &ReviewResult{}
	if err := json.Unmarshal([]byte(jsonStr), result); err != nil {
		return &ReviewResult{
			Summary:     fmt.Sprintf("Failed to parse review JSON: %s", err),
			Approved:    false,
			ReviewedAt:  time.Now(),
			ReviewAgent: r.agentName,
		}, nil
	}

	result.ReviewedAt = time.Now()
	result.ReviewAgent = r.agentName

	// Validate approved flag against findings.
	if result.HasBlockers() {
		result.Approved = false
	}

	return result, nil
}

// extractJSON attempts to find a JSON object in the output.
func extractJSON(output string) string {
	// Find the first { and last }.
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return output[start : end+1]
}

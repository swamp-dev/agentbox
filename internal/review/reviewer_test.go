package review

import (
	"context"
	"strings"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean json",
			input: `{"summary": "looks good", "approved": true, "findings": []}`,
			want:  `{"summary": "looks good", "approved": true, "findings": []}`,
		},
		{
			name:  "json with surrounding text",
			input: "Here is my review:\n```json\n{\"summary\": \"ok\", \"findings\": []}\n```\n",
			want:  `{"summary": "ok", "findings": []}`,
		},
		{
			name:  "no json",
			input: "This is just text with no JSON",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.want {
				t.Errorf("extractJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReviewResult_HasBlockers(t *testing.T) {
	tests := []struct {
		name     string
		findings []ReviewFinding
		want     bool
	}{
		{
			name:     "no findings",
			findings: nil,
			want:     false,
		},
		{
			name: "only minor",
			findings: []ReviewFinding{
				{Severity: SeverityMinor, Description: "style issue"},
			},
			want: false,
		},
		{
			name: "has critical",
			findings: []ReviewFinding{
				{Severity: SeverityCritical, Description: "sql injection"},
			},
			want: true,
		},
		{
			name: "has significant",
			findings: []ReviewFinding{
				{Severity: SeveritySignificant, Description: "missing validation"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReviewResult{Findings: tt.findings}
			if got := r.HasBlockers(); got != tt.want {
				t.Errorf("HasBlockers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReviewResult_BlockerFindings(t *testing.T) {
	r := &ReviewResult{
		Findings: []ReviewFinding{
			{Severity: SeverityCritical, Description: "A"},
			{Severity: SeverityMinor, Description: "B"},
			{Severity: SeveritySignificant, Description: "C"},
			{Severity: SeverityNit, Description: "D"},
		},
	}

	blockers := r.BlockerFindings()
	if len(blockers) != 2 {
		t.Errorf("expected 2 blockers, got %d", len(blockers))
	}
}

func TestReviewResult_CountBySeverity(t *testing.T) {
	r := &ReviewResult{
		Findings: []ReviewFinding{
			{Severity: SeverityCritical},
			{Severity: SeverityMinor},
			{Severity: SeverityMinor},
			{Severity: SeverityNit},
		},
	}

	counts := r.CountBySeverity()
	if counts[SeverityCritical] != 1 {
		t.Errorf("expected 1 critical, got %d", counts[SeverityCritical])
	}
	if counts[SeverityMinor] != 2 {
		t.Errorf("expected 2 minor, got %d", counts[SeverityMinor])
	}
}

func TestParseReviewOutput(t *testing.T) {
	r := &Reviewer{agentName: "test-agent"}

	output := `Here is my review:
{
  "findings": [
    {"severity": "minor", "file": "main.go", "line": 10, "description": "unused import"}
  ],
  "summary": "Looks good overall",
  "approved": true
}
Done.`

	result, err := r.parseReviewOutput(output)
	if err != nil {
		t.Fatalf("parseReviewOutput: %v", err)
	}
	if len(result.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result.Findings))
	}
	if !result.Approved {
		t.Error("expected approved=true")
	}
	if result.ReviewAgent != "test-agent" {
		t.Errorf("expected agent 'test-agent', got %q", result.ReviewAgent)
	}
}

func TestParseReviewOutput_NoJSON(t *testing.T) {
	r := &Reviewer{agentName: "test-agent"}
	result, err := r.parseReviewOutput("Just some text, no JSON here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Approved {
		t.Error("expected not approved when JSON not found")
	}
}

func TestBuildPrompt_Basic(t *testing.T) {
	r := &Reviewer{agentName: "test-agent"}
	diff := "--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,4 @@\n+import \"fmt\"\n"
	changedFiles := []string{"main.go", "utils.go"}
	testSummary := "10 passed, 1 failed"

	prompt := r.buildPrompt(diff, changedFiles, testSummary)

	checks := []string{
		"You are a code reviewer",
		"- main.go",
		"- utils.go",
		"Test results:",
		"10 passed, 1 failed",
		"Diff:",
		"+import \"fmt\"",
		"Respond with JSON only",
		"approved=true means no critical",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Errorf("prompt missing expected content: %q", check)
		}
	}
}

func TestBuildPrompt_LargeDiff(t *testing.T) {
	r := &Reviewer{agentName: "test-agent"}

	// Create a diff larger than 50000 chars.
	largeDiff := strings.Repeat("x", 60000)
	prompt := r.buildPrompt(largeDiff, []string{"big.go"}, "")

	if !strings.Contains(prompt, "... (truncated)") {
		t.Error("expected large diff to be truncated")
	}
	// The prompt should not contain the full 60000-char diff.
	if strings.Contains(prompt, strings.Repeat("x", 60000)) {
		t.Error("expected diff to be truncated, but full diff found")
	}
}

func TestBuildPrompt_SmallDiff(t *testing.T) {
	r := &Reviewer{agentName: "test-agent"}
	smallDiff := "small change"
	prompt := r.buildPrompt(smallDiff, []string{"file.go"}, "")

	if strings.Contains(prompt, "truncated") {
		t.Error("small diff should not be truncated")
	}
	if !strings.Contains(prompt, "small change") {
		t.Error("expected small diff to be included in full")
	}
}

func TestBuildPrompt_NoTestSummary(t *testing.T) {
	r := &Reviewer{agentName: "test-agent"}
	prompt := r.buildPrompt("diff content", []string{"file.go"}, "")

	if strings.Contains(prompt, "Test results:") {
		t.Error("expected no test results section when testSummary is empty")
	}
}

func TestBuildPrompt_NoChangedFiles(t *testing.T) {
	r := &Reviewer{agentName: "test-agent"}
	prompt := r.buildPrompt("diff content", nil, "all pass")

	// Should still have "Changed files:" header but no file entries
	if !strings.Contains(prompt, "Changed files:") {
		t.Error("expected 'Changed files:' header even with no files")
	}
	// Test summary should still be present
	if !strings.Contains(prompt, "all pass") {
		t.Error("expected test summary to be present")
	}
}

func TestParseReviewOutput_InvalidJSON(t *testing.T) {
	r := &Reviewer{agentName: "test-agent"}

	// Output with JSON-like braces but invalid JSON content
	output := `Here is my review: {"invalid: json, missing quotes}`

	result, err := r.parseReviewOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return a result (not panic) with Approved=false
	if result.Approved {
		t.Error("expected not approved for invalid JSON")
	}
	if !strings.Contains(result.Summary, "Failed to parse") {
		t.Errorf("expected summary to mention parse failure, got %q", result.Summary)
	}
	if result.ReviewAgent != "test-agent" {
		t.Errorf("expected agent 'test-agent', got %q", result.ReviewAgent)
	}
}

func TestCountBySeverity_Empty(t *testing.T) {
	r := &ReviewResult{Findings: nil}
	counts := r.CountBySeverity()
	if len(counts) != 0 {
		t.Errorf("expected empty counts for no findings, got %v", counts)
	}
}

func TestBlockerFindings_NonePresent(t *testing.T) {
	r := &ReviewResult{
		Findings: []ReviewFinding{
			{Severity: SeverityMinor, Description: "style"},
			{Severity: SeverityNit, Description: "whitespace"},
		},
	}
	blockers := r.BlockerFindings()
	if len(blockers) != 0 {
		t.Errorf("expected 0 blockers for minor/nit findings, got %d", len(blockers))
	}
}

func TestNewReviewer(t *testing.T) {
	r := NewReviewer("test-agent", nil, nil, nil)
	if r == nil {
		t.Fatal("expected non-nil reviewer")
	}
	if r.agentName != "test-agent" {
		t.Errorf("expected agent name 'test-agent', got %q", r.agentName)
	}
}

func TestReview_UnknownAgent(t *testing.T) {
	r := NewReviewer("nonexistent-agent", nil, nil, nil)
	_, err := r.Review(context.Background(), "/tmp", "diff", []string{"file.go"}, "all pass")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
	if !strings.Contains(err.Error(), "creating review agent") {
		t.Errorf("expected 'creating review agent' error, got: %v", err)
	}
}

func TestParseReviewOutput_BlockersOverrideApproved(t *testing.T) {
	r := &Reviewer{agentName: "test-agent"}

	output := `{
  "findings": [
    {"severity": "critical", "file": "auth.go", "description": "SQL injection vulnerability"}
  ],
  "summary": "Critical issue found",
  "approved": true
}`

	result, err := r.parseReviewOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Approved {
		t.Error("expected approved=false when blockers exist, even if JSON says true")
	}
}

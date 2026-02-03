package review

import (
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

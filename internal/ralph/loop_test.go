package ralph

import (
	"encoding/hex"
	"testing"
)

func TestValidateQualityCheckCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr bool
	}{
		// Allowed commands
		{"npm test", "npm test", false},
		{"go test", "go test ./...", false},
		{"cargo test", "cargo test", false},
		{"pytest", "pytest -v", false},
		{"make lint", "make lint", false},
		{"npx prettier", "npx prettier --check .", false},
		{"pnpm test", "pnpm test", false},
		{"yarn test", "yarn test", false},
		{"bun test", "bun test", false},
		{"eslint", "eslint src/", false},
		{"prettier", "prettier --check .", false},
		{"tsc", "tsc --noEmit", false},
		{"jest", "jest --coverage", false},
		{"vitest", "vitest run", false},
		{"mocha", "mocha tests/", false},
		{"python test", "python -m pytest", false},
		{"python3 test", "python3 -m pytest", false},
		{"gradle build", "gradle build", false},
		{"mvn test", "mvn test", false},
		{"rustc check", "rustc --edition 2021 main.rs", false},
		{"pip check", "pip check", false},

		// Path-prefixed commands should work (filepath.Base extracts the binary name)
		{"/usr/bin/go", "/usr/bin/go test ./...", false},
		{"/usr/local/bin/npm", "/usr/local/bin/npm test", false},

		// Disallowed commands
		{"sh", "sh -c 'rm -rf /'", true},
		{"bash", "bash script.sh", true},
		{"rm", "rm -rf /", true},
		{"curl", "curl http://evil.com", true},
		{"wget", "wget http://evil.com", true},
		{"unknown", "unknown-tool run", true},

		// Edge cases
		{"empty command", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateQualityCheckCommand(tt.command)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateQualityCheckCommand(%q) error = %v, wantErr %v", tt.command, err, tt.wantErr)
			}
		})
	}
}

func TestExtractLearnings(t *testing.T) {
	loop := &Loop{}

	tests := []struct {
		name     string
		output   string
		expected []string
	}{
		{
			name:     "learning prefix",
			output:   "Some text\nLearning: Go tests are fast\nMore text",
			expected: []string{"Go tests are fast"},
		},
		{
			name:     "note prefix",
			output:   "Note: Use t.TempDir for test dirs",
			expected: []string{"Use t.TempDir for test dirs"},
		},
		{
			name:     "important prefix",
			output:   "Important: Always check errors",
			expected: []string{"Always check errors"},
		},
		{
			name:     "mixed case learning",
			output:   "learning: lowercase works too",
			expected: []string{"lowercase works too"},
		},
		{
			name:     "mixed case note",
			output:   "note: lowercase note",
			expected: []string{"lowercase note"},
		},
		{
			name:     "mixed case important",
			output:   "important: lowercase important",
			expected: []string{"lowercase important"},
		},
		{
			name:   "multiple learnings",
			output: "Learning: first\nSome text\nNote: second\nImportant: third",
			expected: []string{
				"first",
				"second",
				"third",
			},
		},
		{
			name:     "no matches",
			output:   "Just regular output\nNothing special here",
			expected: nil,
		},
		{
			name:     "empty output",
			output:   "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := loop.extractLearnings(tt.output)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d learnings, got %d: %v", len(tt.expected), len(result), result)
			}
			for i, exp := range tt.expected {
				if result[i] != exp {
					t.Errorf("learning[%d] = %q, want %q", i, result[i], exp)
				}
			}
		})
	}
}

func TestLoopStatusString(t *testing.T) {
	status := &LoopStatus{
		PRDName:       "Test Project",
		TotalTasks:    10,
		Completed:     3,
		InProgress:    1,
		Pending:       6,
		Progress:      30.0,
		MaxIterations: 50,
		Iteration:     5,
	}

	result := status.String()

	expected := "PRD: Test Project\nProgress: 30.0% (3/10 tasks)\nStatus: 3 completed, 1 in progress, 6 pending\nIteration: 5/50"
	if result != expected {
		t.Errorf("expected:\n%s\n\ngot:\n%s", expected, result)
	}
}

func TestLoopStatusStringZero(t *testing.T) {
	status := &LoopStatus{
		PRDName:       "Empty",
		TotalTasks:    0,
		Completed:     0,
		InProgress:    0,
		Pending:       0,
		Progress:      0.0,
		MaxIterations: 10,
		Iteration:     0,
	}

	result := status.String()

	expected := "PRD: Empty\nProgress: 0.0% (0/0 tasks)\nStatus: 0 completed, 0 in progress, 0 pending\nIteration: 0/10"
	if result != expected {
		t.Errorf("expected:\n%s\n\ngot:\n%s", expected, result)
	}
}

func TestRandomSuffix(t *testing.T) {
	s1 := randomSuffix()
	s2 := randomSuffix()

	// Should be 8 hex characters (4 bytes = 8 hex chars)
	if len(s1) != 8 {
		t.Errorf("expected 8 char suffix, got %d: %s", len(s1), s1)
	}

	// Should be valid hex
	if _, err := hex.DecodeString(s1); err != nil {
		t.Errorf("expected valid hex, got error: %v", err)
	}

	// Two calls should (almost certainly) differ
	if s1 == s2 {
		t.Error("expected different suffixes from two calls")
	}
}

package cli

import (
	"strings"
	"testing"
)

func TestRenderProgressBar(t *testing.T) {
	tests := []struct {
		name    string
		percent float64
		width   int
		wantLen int // total length including brackets
	}{
		{"0 percent", 0.0, 20, 22},
		{"50 percent", 50.0, 20, 22},
		{"100 percent", 100.0, 20, 22},
		{"25 percent", 25.0, 40, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderProgressBar(tt.percent, tt.width)

			// Check total length (width + 2 brackets)
			// Note: Unicode chars may have different byte lengths but we check rune count
			runes := []rune(result)
			if len(runes) != tt.wantLen {
				t.Errorf("renderProgressBar(%.0f, %d) rune length = %d, want %d", tt.percent, tt.width, len(runes), tt.wantLen)
			}

			// Should start and end with brackets
			if result[0] != '[' {
				t.Error("expected bar to start with '['")
			}
			if runes[len(runes)-1] != ']' {
				t.Error("expected bar to end with ']'")
			}
		})
	}

	// 0% should have no filled blocks
	bar0 := renderProgressBar(0.0, 10)
	if strings.Contains(bar0, "█") {
		t.Error("0% bar should have no filled blocks")
	}

	// 100% should have no empty blocks
	bar100 := renderProgressBar(100.0, 10)
	if strings.Contains(bar100, "░") {
		t.Error("100% bar should have no empty blocks")
	}

	// >100% should be clamped to full
	barOver := renderProgressBar(150.0, 10)
	if strings.Contains(barOver, "░") {
		t.Error(">100% bar should be clamped to full (no empty blocks)")
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{"COMPLETED", "✓"},
		{"STARTED", "▶"},
		{"FAILED", "✗"},
		{"ITERATION", "↻"},
		{"unknown", "○"},
		{"", "○"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := statusIcon(tt.status)
			if result != tt.expected {
				t.Errorf("statusIcon(%q) = %q, want %q", tt.status, result, tt.expected)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		max      int
		expected string
	}{
		{
			name:     "short string unchanged",
			input:    "hello",
			max:      10,
			expected: "hello",
		},
		{
			name:     "exact length unchanged",
			input:    "hello",
			max:      5,
			expected: "hello",
		},
		{
			name:     "over length gets ellipsis",
			input:    "hello world",
			max:      8,
			expected: "hello...",
		},
		{
			name:     "empty string",
			input:    "",
			max:      10,
			expected: "",
		},
		{
			name:     "one over max",
			input:    "abcdef",
			max:      5,
			expected: "ab...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.max)
			if result != tt.expected {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, result, tt.expected)
			}
			if len(result) > tt.max {
				t.Errorf("truncate result length %d exceeds max %d", len(result), tt.max)
			}
		})
	}
}

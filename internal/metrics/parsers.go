package metrics

import (
	"regexp"
	"strconv"
	"strings"
)

// TestStats holds parsed test execution results.
type TestStats struct {
	Total   int      `json:"total"`
	Passed  int      `json:"passed"`
	Failed  int      `json:"failed"`
	Skipped int      `json:"skipped"`
	FailedTests []string `json:"failed_tests,omitempty"`
}

// PassRate returns the pass rate as a float between 0 and 1.
func (s *TestStats) PassRate() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Passed) / float64(s.Total)
}

var (
	// Go test patterns.
	goPassRe  = regexp.MustCompile(`^ok\s+`)
	goFailRe  = regexp.MustCompile(`^FAIL\s+`)
	goTestRe  = regexp.MustCompile(`^--- (PASS|FAIL|SKIP): (\S+)`)
	goSummRe  = regexp.MustCompile(`^(ok|FAIL)\s+\S+\s+[\d.]+s`)

	// Jest patterns.
	jestSummRe = regexp.MustCompile(`Tests:\s+(?:(\d+) failed,\s+)?(?:(\d+) skipped,\s+)?(?:(\d+) passed,\s+)?(\d+) total`)
	jestFailRe = regexp.MustCompile(`●\s+(.+)`)

	// Generic patterns.
	genericPassRe = regexp.MustCompile(`(?i)^(PASS|✓|√|ok)\s`)
	genericFailRe = regexp.MustCompile(`(?i)^(FAIL|✗|✘|×|not ok)\s`)
)

// ParseGoTestOutput parses `go test` output into TestStats.
func ParseGoTestOutput(output string) *TestStats {
	stats := &TestStats{}
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if m := goTestRe.FindStringSubmatch(line); len(m) == 3 {
			stats.Total++
			switch m[1] {
			case "PASS":
				stats.Passed++
			case "FAIL":
				stats.Failed++
				stats.FailedTests = append(stats.FailedTests, m[2])
			case "SKIP":
				stats.Skipped++
			}
		}
	}

	// If no individual test results, try summary lines.
	if stats.Total == 0 {
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if goPassRe.MatchString(line) {
				stats.Total++
				stats.Passed++
			} else if goFailRe.MatchString(line) {
				stats.Total++
				stats.Failed++
			}
		}
	}

	return stats
}

// ParseJestOutput parses Jest test runner output into TestStats.
func ParseJestOutput(output string) *TestStats {
	stats := &TestStats{}
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if m := jestSummRe.FindStringSubmatch(line); len(m) == 5 {
			if m[1] != "" {
				stats.Failed, _ = strconv.Atoi(m[1])
			}
			if m[2] != "" {
				stats.Skipped, _ = strconv.Atoi(m[2])
			}
			if m[3] != "" {
				stats.Passed, _ = strconv.Atoi(m[3])
			}
			stats.Total, _ = strconv.Atoi(m[4])
			break
		}
	}

	// Collect failed test names.
	for _, line := range lines {
		if m := jestFailRe.FindStringSubmatch(line); len(m) == 2 {
			stats.FailedTests = append(stats.FailedTests, strings.TrimSpace(m[1]))
		}
	}

	return stats
}

// ParseGenericTestOutput attempts to parse test output by counting PASS/FAIL lines.
func ParseGenericTestOutput(output string) *TestStats {
	stats := &TestStats{}
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if genericPassRe.MatchString(line) {
			stats.Total++
			stats.Passed++
		} else if genericFailRe.MatchString(line) {
			stats.Total++
			stats.Failed++
			stats.FailedTests = append(stats.FailedTests, line)
		}
	}

	return stats
}

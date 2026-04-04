package ralph

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// =============================================================================
// commitChanges tests
// =============================================================================

// initTestRepo creates a temporary git repo with an initial commit.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v: %v\n%s", args, err, out)
		}
	}

	// Create initial file and commit.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", "initial commit"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init commit %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// newTestLoop creates a minimal Loop for testing commitChanges.
func newTestLoop(projectPath string) *Loop {
	return &Loop{
		projectPath: projectPath,
		iteration:   1,
		logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}
}

// lastCommitFiles returns the files changed in the last commit.
func lastCommitFiles(t *testing.T, dir string) []string {
	t.Helper()
	cmd := exec.Command("git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git diff-tree: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func TestCommitChangesIncludesUntrackedFiles(t *testing.T) {
	dir := initTestRepo(t)
	loop := newTestLoop(dir)

	// Create a new untracked file.
	if err := os.WriteFile(filepath.Join(dir, "new-feature.ts"), []byte("export const x = 1;\n"), 0644); err != nil {
		t.Fatal(err)
	}

	task := &Task{ID: "task-1", Title: "Add new feature"}
	if err := loop.commitChanges(context.Background(), task); err != nil {
		t.Fatalf("commitChanges failed: %v", err)
	}

	files := lastCommitFiles(t, dir)
	found := false
	for _, f := range files {
		if f == "new-feature.ts" {
			found = true
		}
	}
	if !found {
		t.Errorf("untracked file 'new-feature.ts' not committed; committed files: %v", files)
	}
}

func TestCommitChangesIncludesNewDirectories(t *testing.T) {
	dir := initTestRepo(t)
	loop := newTestLoop(dir)

	// Create a new directory tree with files.
	newDir := filepath.Join(dir, "plugins.local", "agentbox")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "plugin.ts"), []byte("export default {};\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "config.ts"), []byte("export const cfg = {};\n"), 0644); err != nil {
		t.Fatal(err)
	}

	task := &Task{ID: "task-1", Title: "Add plugin skeleton"}
	if err := loop.commitChanges(context.Background(), task); err != nil {
		t.Fatalf("commitChanges failed: %v", err)
	}

	files := lastCommitFiles(t, dir)
	expected := map[string]bool{
		"plugins.local/agentbox/plugin.ts": true,
		"plugins.local/agentbox/config.ts": true,
	}
	for _, f := range files {
		delete(expected, f)
	}
	if len(expected) > 0 {
		var missing []string
		for f := range expected {
			missing = append(missing, f)
		}
		t.Errorf("new directory files not committed; missing: %v; committed: %v", missing, files)
	}
}

func TestCommitChangesNoChangesNoCommit(t *testing.T) {
	dir := initTestRepo(t)
	loop := newTestLoop(dir)

	headBefore := exec.Command("git", "rev-parse", "HEAD")
	headBefore.Dir = dir
	before, _ := headBefore.Output()

	task := &Task{ID: "task-1", Title: "Nothing"}
	if err := loop.commitChanges(context.Background(), task); err != nil {
		t.Fatalf("commitChanges failed: %v", err)
	}

	headAfter := exec.Command("git", "rev-parse", "HEAD")
	headAfter.Dir = dir
	after, _ := headAfter.Output()

	if string(before) != string(after) {
		t.Error("commitChanges created a commit when there were no changes")
	}
}

func TestCommitChangesAfterGitignoreRemoval(t *testing.T) {
	dir := initTestRepo(t)

	// Add .gitignore that ignores plugins.local/ and commit it.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("plugins.local/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", "add gitignore"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	loop := newTestLoop(dir)

	// Simulate agent: remove the gitignore entry AND create new files in one step.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	newDir := filepath.Join(dir, "plugins.local")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "agentbox.ts"), []byte("export default {};\n"), 0644); err != nil {
		t.Fatal(err)
	}

	task := &Task{ID: "task-1", Title: "Add plugin"}
	if err := loop.commitChanges(context.Background(), task); err != nil {
		t.Fatalf("commitChanges failed: %v", err)
	}

	files := lastCommitFiles(t, dir)
	foundPlugin := false
	foundGitignore := false
	for _, f := range files {
		if f == "plugins.local/agentbox.ts" {
			foundPlugin = true
		}
		if f == ".gitignore" {
			foundGitignore = true
		}
	}
	if !foundGitignore {
		t.Error(".gitignore change not committed")
	}
	if !foundPlugin {
		t.Errorf("plugins.local/agentbox.ts not committed after .gitignore removal; committed: %v", files)
	}
}

func TestCommitChangesPreviouslyIgnoredDirectory(t *testing.T) {
	dir := initTestRepo(t)

	// Start with .gitignore that ignores plugins.local/.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("plugins.local/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create the ignored directory — it exists on disk but is not tracked.
	newDir := filepath.Join(dir, "plugins.local")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "old.ts"), []byte("// old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", "setup with gitignore"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Verify old.ts is NOT tracked.
	cmd := exec.Command("git", "ls-files", "plugins.local/old.ts")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != "" {
		t.Fatal("setup error: plugins.local/old.ts should not be tracked")
	}

	loop := newTestLoop(dir)

	// Now the agent removes the gitignore entry and creates a new file.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "new.ts"), []byte("// new\n"), 0644); err != nil {
		t.Fatal(err)
	}

	task := &Task{ID: "task-1", Title: "Unignore and add"}
	if err := loop.commitChanges(context.Background(), task); err != nil {
		t.Fatalf("commitChanges failed: %v", err)
	}

	files := lastCommitFiles(t, dir)
	foundNew := false
	foundOld := false
	for _, f := range files {
		if f == "plugins.local/new.ts" {
			foundNew = true
		}
		if f == "plugins.local/old.ts" {
			foundOld = true
		}
	}
	if !foundNew {
		t.Errorf("new file not committed after gitignore removal; committed: %v", files)
	}
	if !foundOld {
		t.Errorf("previously-ignored file not committed after gitignore removal; committed: %v", files)
	}
}

func TestCommitChangesDoesNotPollutGlobalGitConfig(t *testing.T) {
	// Regression: an earlier fix used `git config --global --add safe.directory`
	// which permanently mutated ~/.gitconfig on every call. The inline -c approach
	// must leave no trace in global config.
	dir := initTestRepo(t)
	loop := newTestLoop(dir)

	// Record safe.directory entries before.
	beforeCmd := exec.Command("git", "config", "--global", "--get-all", "safe.directory")
	beforeOut, _ := beforeCmd.Output()
	beforeCount := len(strings.Split(strings.TrimSpace(string(beforeOut)), "\n"))
	if strings.TrimSpace(string(beforeOut)) == "" {
		beforeCount = 0
	}

	// Create a file and commit.
	if err := os.WriteFile(filepath.Join(dir, "test.ts"), []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	task := &Task{ID: "task-1", Title: "Test"}
	if err := loop.commitChanges(context.Background(), task); err != nil {
		t.Fatalf("commitChanges failed: %v", err)
	}

	// Record safe.directory entries after.
	afterCmd := exec.Command("git", "config", "--global", "--get-all", "safe.directory")
	afterOut, _ := afterCmd.Output()
	afterCount := len(strings.Split(strings.TrimSpace(string(afterOut)), "\n"))
	if strings.TrimSpace(string(afterOut)) == "" {
		afterCount = 0
	}

	if afterCount != beforeCount {
		t.Errorf("commitChanges polluted global git config: safe.directory entries before=%d after=%d", beforeCount, afterCount)
	}
}

func TestCommitChangesIncludesStderrInErrors(t *testing.T) {
	// Regression: earlier implementation discarded stderr from git commands,
	// making failures invisible. Errors must include stderr content.
	dir := initTestRepo(t)
	loop := newTestLoop(dir)

	// Corrupt the git index to force git add to fail.
	indexPath := filepath.Join(dir, ".git", "index")
	if err := os.WriteFile(indexPath, []byte("corrupted"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "file.ts"), []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}

	task := &Task{ID: "task-1", Title: "Test"}
	err := loop.commitChanges(context.Background(), task)
	if err == nil {
		t.Fatal("expected error from commitChanges with corrupted index")
	}

	// The error message must include actual git stderr output, not just the
	// format template. Git prints "fatal:" on index corruption errors.
	errMsg := err.Error()
	if !strings.Contains(errMsg, "stderr:") {
		t.Errorf("error message should include stderr label; got: %s", errMsg)
	}
	// Verify actual git error content is present (not just an empty stderr field).
	if !strings.Contains(errMsg, "fatal") && !strings.Contains(errMsg, "index") && !strings.Contains(errMsg, "bad signature") {
		t.Errorf("error message should include actual git error content; got: %s", errMsg)
	}
}

func TestCommitChangesMultipleCallsDoNotAccumulate(t *testing.T) {
	// Regression: an earlier fix appended to global config on every call.
	// Running commitChanges N times must not leave N config entries.
	dir := initTestRepo(t)
	loop := newTestLoop(dir)

	for i := 0; i < 5; i++ {
		fname := filepath.Join(dir, fmt.Sprintf("file-%d.ts", i))
		if err := os.WriteFile(fname, []byte(fmt.Sprintf("// iteration %d\n", i)), 0644); err != nil {
			t.Fatal(err)
		}
		task := &Task{ID: fmt.Sprintf("task-%d", i+1), Title: fmt.Sprintf("Iter %d", i)}
		if err := loop.commitChanges(context.Background(), task); err != nil {
			t.Fatalf("commitChanges iteration %d failed: %v", i, err)
		}
	}

	// Verify all 5 files were committed (one per commit).
	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	// 5 commits from loop + 1 initial = 6 total.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 6 {
		t.Errorf("expected 6 commits (1 initial + 5 iterations), got %d: %v", len(lines), lines)
	}

	// Verify no global config pollution.
	safeCmd := exec.Command("git", "config", "--global", "--get-all", "safe.directory")
	safeOut, _ := safeCmd.Output()
	if strings.Contains(string(safeOut), dir) {
		t.Errorf("global git config contains safe.directory entry for test dir %s", dir)
	}
}

func TestCommitChangesSkipsNonGitDirectory(t *testing.T) {
	dir := t.TempDir()
	loop := newTestLoop(dir)

	task := &Task{ID: "task-1", Title: "Test"}
	err := loop.commitChanges(context.Background(), task)
	if err != nil {
		t.Errorf("expected nil error for non-git dir, got: %v", err)
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

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

	"github.com/swamp-dev/agentbox/internal/agent"
	"github.com/swamp-dev/agentbox/internal/config"
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

// =============================================================================
// Mock agent for loop tests
// =============================================================================

// mockAgent implements agent.Agent for testing.
type mockAgent struct {
	parseOutputFn func(output string) *agent.AgentOutput
}

func (m *mockAgent) Name() string               { return "mock" }
func (m *mockAgent) Command(_ string) []string  { return []string{"echo", "mock"} }
func (m *mockAgent) Environment() []string      { return nil }
func (m *mockAgent) StopSignal() string         { return "<promise>COMPLETE</promise>" }
func (m *mockAgent) AllowedEndpoints() []string { return nil }
func (m *mockAgent) ParseOutput(output string) *agent.AgentOutput {
	if m.parseOutputFn != nil {
		return m.parseOutputFn(output)
	}
	return &agent.AgentOutput{Success: true, Message: "ok"}
}

// newTestableLoop creates a Loop wired with mock functions for testing.
// It writes a temporary PRD and progress file so the Loop is self-contained.
// NOTE: container is intentionally nil — tests use injected runAgentFn/runQualityChecksFn
// instead of real Docker execution. Do not call Close() on test loops.
func newTestableLoop(t *testing.T, tasks []Task, maxIterations int) *Loop {
	t.Helper()
	dir := t.TempDir()

	prd := &PRD{
		Name:  "Test PRD",
		Tasks: tasks,
	}
	prd.updateMetadata()

	// Write PRD file so Save() works.
	prdPath := filepath.Join(dir, "prd.json")
	if err := prd.Save(prdPath); err != nil {
		t.Fatal(err)
	}

	progress := NewProgress(filepath.Join(dir, "progress.txt"))

	cfg := config.DefaultConfig()
	cfg.Ralph.MaxIterations = maxIterations
	cfg.Ralph.AutoCommit = false
	cfg.Ralph.PRDFile = "prd.json"
	cfg.Ralph.ProgressFile = "progress.txt"

	l := &Loop{
		cfg:         cfg,
		prd:         prd,
		progress:    progress,
		agent:       &mockAgent{},
		logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		projectPath: dir,
	}
	// Default mock functions: agent succeeds, quality checks pass.
	l.runAgentFn = func(_ context.Context, _ string) (string, error) {
		return "task done\n<promise>COMPLETE</promise>", nil
	}
	l.runQualityChecksFn = func(_ context.Context) error {
		return nil
	}
	return l
}

// =============================================================================
// buildPrompt tests
// =============================================================================

func TestBuildPrompt(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "Setup project", Description: "Initialize the project structure", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 10)

	task := &tasks[0]
	prompt := loop.buildPrompt(task)

	tests := []struct {
		name     string
		contains string
	}{
		{"contains PRD name", "Test PRD"},
		{"contains task ID", "task-1"},
		{"contains task title", "Setup project"},
		{"contains task description", "Initialize the project structure"},
		{"contains stop signal", loop.cfg.Ralph.StopSignal},
		{"contains instructions", "Complete the task"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(prompt, tt.contains) {
				t.Errorf("prompt missing %q; got:\n%s", tt.contains, prompt)
			}
		})
	}
}

// =============================================================================
// Run() loop logic tests
// =============================================================================

func TestRunCompletesWhenAllTasksDone(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "Only task", Description: "Do something", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 10)

	err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if !loop.prd.IsComplete() {
		t.Error("expected PRD to be complete after Run()")
	}
}

func TestRunStopsAtMaxIterations(t *testing.T) {
	// Two tasks but max 1 iteration — should fail with max iterations reached.
	tasks := []Task{
		{ID: "task-1", Title: "First", Description: "first", Status: "pending"},
		{ID: "task-2", Title: "Second", Description: "second", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 1)

	err := loop.Run(context.Background())
	if err == nil {
		t.Fatal("expected error when max iterations reached")
	}
	if !strings.Contains(err.Error(), "max iterations") {
		t.Errorf("expected 'max iterations' error, got: %v", err)
	}
}

func TestRunRespectsContextCancellation(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "First", Description: "first", Status: "pending"},
		{ID: "task-2", Title: "Second", Description: "second", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 100)

	// Cancel the context inside runAgentFn so the loop exits on the ctx check.
	ctx, cancel := context.WithCancel(context.Background())
	loop.runAgentFn = func(ctx context.Context, _ string) (string, error) {
		cancel()             // cancel immediately
		return "", ctx.Err() // return context error
	}

	err := loop.Run(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected error containing 'context canceled', got: %v", err)
	}
}

func TestRunEarlyExitWhenPRDAlreadyComplete(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "Done", Description: "already done", Status: "completed"},
	}
	loop := newTestableLoop(t, tasks, 10)

	agentCalled := false
	loop.runAgentFn = func(_ context.Context, _ string) (string, error) {
		agentCalled = true
		return "", nil
	}

	err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if agentCalled {
		t.Error("agent should not be called when PRD is already complete")
	}
}

func TestRunMultipleTasksSequentially(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "First", Description: "first task", Status: "pending"},
		{ID: "task-2", Title: "Second", Description: "second task", Status: "pending"},
		{ID: "task-3", Title: "Third", Description: "third task", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 10)

	var promptsSeen []string
	loop.runAgentFn = func(_ context.Context, prompt string) (string, error) {
		promptsSeen = append(promptsSeen, prompt)
		return "done", nil
	}

	err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if len(promptsSeen) != 3 {
		t.Fatalf("expected 3 agent calls, got %d", len(promptsSeen))
	}
	// Verify tasks were processed in order.
	for i, id := range []string{"task-1", "task-2", "task-3"} {
		if !strings.Contains(promptsSeen[i], id) {
			t.Errorf("prompt %d should contain %s", i, id)
		}
	}
}

// =============================================================================
// runIteration tests
// =============================================================================

func TestRunIterationAgentFailure(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "Fail task", Description: "will fail", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 10)

	loop.runAgentFn = func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("connection refused")
	}

	err := loop.runIteration(context.Background())
	if err == nil {
		t.Fatal("expected error from agent failure")
	}
	if !strings.Contains(err.Error(), "agent execution failed") {
		t.Errorf("expected 'agent execution failed' error, got: %v", err)
	}
}

func TestRunIterationAgentReportsFailure(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "Agent fails", Description: "agent says no", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 10)

	loop.agent = &mockAgent{
		parseOutputFn: func(_ string) *agent.AgentOutput {
			return &agent.AgentOutput{Success: false, Message: "could not complete task"}
		},
	}

	err := loop.runIteration(context.Background())
	if err == nil {
		t.Fatal("expected error from agent reported failure")
	}
	if !strings.Contains(err.Error(), "agent reported failure") {
		t.Errorf("expected 'agent reported failure' error, got: %v", err)
	}
}

func TestRunIterationQualityCheckFailure(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "QC fail", Description: "quality check fails", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 10)

	loop.runQualityChecksFn = func(_ context.Context) error {
		return fmt.Errorf("lint: 5 errors found")
	}

	err := loop.runIteration(context.Background())
	if err == nil {
		t.Fatal("expected error from quality check failure")
	}
	if !strings.Contains(err.Error(), "quality checks failed") {
		t.Errorf("expected 'quality checks failed' error, got: %v", err)
	}
}

func TestRunIterationNoAvailableTasks(t *testing.T) {
	// All tasks completed — NextTask() returns nil.
	tasks := []Task{
		{ID: "task-1", Title: "Done", Description: "done", Status: "completed"},
	}
	loop := newTestableLoop(t, tasks, 10)

	err := loop.runIteration(context.Background())
	if err == nil {
		t.Fatal("expected error when no tasks available")
	}
	if !strings.Contains(err.Error(), "no available tasks") {
		t.Errorf("expected 'no available tasks' error, got: %v", err)
	}
}

func TestRunIterationMarksTaskComplete(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "Complete me", Description: "should complete", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 10)

	loop.runAgentFn = func(_ context.Context, _ string) (string, error) {
		return "Learning: always check errors\ndone", nil
	}

	err := loop.runIteration(context.Background())
	if err != nil {
		t.Fatalf("runIteration() returned error: %v", err)
	}

	task := loop.prd.GetTask("task-1")
	if task.Status != "completed" {
		t.Errorf("expected task status 'completed', got %q", task.Status)
	}
	if !strings.Contains(task.Learnings, "always check errors") {
		t.Errorf("expected learnings to contain 'always check errors', got %q", task.Learnings)
	}
}

// =============================================================================
// RunSingleTask tests
// =============================================================================

func TestRunSingleTaskSuccess(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "Single task", Description: "run once", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 10)

	loop.runAgentFn = func(_ context.Context, _ string) (string, error) {
		return "Learning: tests are important\nall done", nil
	}

	result := loop.RunSingleTask(context.Background(), &tasks[0], "do the thing")
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !result.QualityOK {
		t.Error("expected QualityOK to be true")
	}
	if len(result.Learnings) != 1 || result.Learnings[0] != "tests are important" {
		t.Errorf("unexpected learnings: %v", result.Learnings)
	}
}

func TestRunSingleTaskAgentError(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "Fail task", Description: "will fail", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 10)

	loop.runAgentFn = func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("timeout")
	}

	result := loop.RunSingleTask(context.Background(), &tasks[0], "do the thing")
	if result.Success {
		t.Fatal("expected failure")
	}
	if !strings.Contains(result.Error, "agent execution failed") {
		t.Errorf("expected 'agent execution failed' error, got: %s", result.Error)
	}
}

func TestRunSingleTaskQualityCheckFailure(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "QC fail", Description: "qc fails", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 10)

	loop.runQualityChecksFn = func(_ context.Context) error {
		return fmt.Errorf("tests failed: 3 errors")
	}

	result := loop.RunSingleTask(context.Background(), &tasks[0], "do the thing")
	if result.Success {
		t.Fatal("expected failure")
	}
	if result.QualityOK {
		t.Error("expected QualityOK to be false")
	}
	if !strings.Contains(result.Error, "quality check failed") {
		t.Errorf("expected 'quality check failed' error, got: %s", result.Error)
	}
}

func TestRunSingleTaskIncrementsIteration(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "First", Description: "first", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 10)

	before := loop.GetIteration()
	loop.RunSingleTask(context.Background(), &tasks[0], "prompt")
	after := loop.GetIteration()

	if after != before+1 {
		t.Errorf("expected iteration to increment from %d to %d, got %d", before, before+1, after)
	}
}

func TestRunSingleTaskInvalidTaskID(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "Real task", Description: "exists", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 10)

	// Pass a task that doesn't exist in the PRD.
	bogusTask := &Task{ID: "task-999", Title: "Ghost"}
	result := loop.RunSingleTask(context.Background(), bogusTask, "prompt")
	if result.Success {
		t.Fatal("expected failure for invalid task ID")
	}
	if result.Error == "" {
		t.Error("expected error message for invalid task ID")
	}
}

// =============================================================================
// Status / GetPRD / GetIteration tests
// =============================================================================

func TestLoopStatusReflectsPRDState(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "Done", Status: "completed"},
		{ID: "task-2", Title: "Doing", Status: "in_progress"},
		{ID: "task-3", Title: "Todo", Status: "pending"},
		{ID: "task-4", Title: "Also todo", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 20)
	loop.iteration = 5

	status := loop.Status()
	if status.PRDName != "Test PRD" {
		t.Errorf("expected PRD name 'Test PRD', got %q", status.PRDName)
	}
	if status.TotalTasks != 4 {
		t.Errorf("expected 4 total tasks, got %d", status.TotalTasks)
	}
	if status.Completed != 1 {
		t.Errorf("expected 1 completed, got %d", status.Completed)
	}
	if status.InProgress != 1 {
		t.Errorf("expected 1 in_progress, got %d", status.InProgress)
	}
	if status.Pending != 2 {
		t.Errorf("expected 2 pending, got %d", status.Pending)
	}
	if status.Iteration != 5 {
		t.Errorf("expected iteration 5, got %d", status.Iteration)
	}
	if status.MaxIterations != 20 {
		t.Errorf("expected max iterations 20, got %d", status.MaxIterations)
	}
}

func TestGetPRDReturnsUnderlyingPRD(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "Test", Status: "pending"},
	}
	loop := newTestableLoop(t, tasks, 10)

	prd := loop.GetPRD()
	if prd == nil {
		t.Fatal("GetPRD() returned nil")
	}
	if prd.Name != "Test PRD" {
		t.Errorf("expected PRD name 'Test PRD', got %q", prd.Name)
	}
}

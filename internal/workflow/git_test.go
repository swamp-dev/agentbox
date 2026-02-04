package workflow

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "checkout", "-b", "main"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("init command %v failed: %v", args, err)
		}
	}

	// Create initial commit.
	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}
	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	return dir
}

func TestRepoNameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/user/project", "project"},
		{"https://github.com/user/project.git", "project"},
		{"git@github.com:user/my-repo.git", "my-repo"},
	}
	for _, tt := range tests {
		got := repoNameFromURL(tt.url)
		if got != tt.want {
			t.Errorf("repoNameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestCloneOrOpen_ExistingRepo(t *testing.T) {
	dir := initTestRepo(t)
	repoDir := filepath.Join(dir, "repo")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", repoDir, logger)

	ctx := context.Background()
	if err := gw.CloneOrOpen(ctx); err != nil {
		t.Fatalf("CloneOrOpen: %v", err)
	}
}

func TestCloneOrOpen_NotARepo(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", dir, logger)

	ctx := context.Background()
	err := gw.CloneOrOpen(ctx)
	if err == nil {
		t.Fatal("expected error for non-repo directory")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateWorktree(t *testing.T) {
	dir := initTestRepo(t)
	repoDir := filepath.Join(dir, "repo")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", repoDir, logger)

	ctx := context.Background()
	if err := gw.CloneOrOpen(ctx); err != nil {
		t.Fatalf("CloneOrOpen: %v", err)
	}

	if err := gw.CreateWorktree(ctx, "feat/test-branch"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	if gw.BranchName() != "feat/test-branch" {
		t.Errorf("expected branch 'feat/test-branch', got %q", gw.BranchName())
	}

	// Verify worktree directory exists.
	if _, err := os.Stat(gw.WorktreePath()); os.IsNotExist(err) {
		t.Errorf("worktree path does not exist: %s", gw.WorktreePath())
	}
}

func TestCommit(t *testing.T) {
	dir := initTestRepo(t)
	repoDir := filepath.Join(dir, "repo")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", repoDir, logger)

	ctx := context.Background()
	if err := gw.CloneOrOpen(ctx); err != nil {
		t.Fatalf("CloneOrOpen: %v", err)
	}
	if err := gw.CreateWorktree(ctx, "feat/commit-test"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Create a file in the worktree.
	testFile := filepath.Join(gw.WorktreePath(), "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := gw.Commit(ctx, "feat: add test file", nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Verify commit.
	sha, err := gw.CurrentCommit(ctx)
	if err != nil {
		t.Fatalf("CurrentCommit: %v", err)
	}
	if sha == "" {
		t.Error("expected non-empty commit SHA")
	}
}

func TestCommitNothingToCommit(t *testing.T) {
	dir := initTestRepo(t)
	repoDir := filepath.Join(dir, "repo")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", repoDir, logger)

	ctx := context.Background()
	if err := gw.CloneOrOpen(ctx); err != nil {
		t.Fatalf("CloneOrOpen: %v", err)
	}
	if err := gw.CreateWorktree(ctx, "feat/empty-test"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Commit with nothing staged — should succeed silently.
	if err := gw.Commit(ctx, "feat: empty", nil); err != nil {
		t.Fatalf("Commit with nothing to commit: %v", err)
	}
}

func TestCurrentCommitAndRollback(t *testing.T) {
	dir := initTestRepo(t)
	repoDir := filepath.Join(dir, "repo")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", repoDir, logger)

	ctx := context.Background()
	if err := gw.CloneOrOpen(ctx); err != nil {
		t.Fatalf("CloneOrOpen: %v", err)
	}
	if err := gw.CreateWorktree(ctx, "feat/rollback-test"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	beforeSHA, _ := gw.CurrentCommit(ctx)

	// Make a commit.
	if err := os.WriteFile(filepath.Join(gw.WorktreePath(), "new.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := gw.Commit(ctx, "feat: new file", nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	afterSHA, _ := gw.CurrentCommit(ctx)
	if beforeSHA == afterSHA {
		t.Fatal("expected different SHA after commit")
	}

	// Rollback.
	if err := gw.Rollback(ctx, beforeSHA); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	currentSHA, _ := gw.CurrentCommit(ctx)
	if currentSHA != beforeSHA {
		t.Errorf("expected SHA %s after rollback, got %s", beforeSHA, currentSHA)
	}
}

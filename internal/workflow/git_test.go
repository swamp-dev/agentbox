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

func TestDiff(t *testing.T) {
	dir := initTestRepo(t)
	repoDir := filepath.Join(dir, "repo")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", repoDir, logger)

	ctx := context.Background()
	if err := gw.CloneOrOpen(ctx); err != nil {
		t.Fatalf("CloneOrOpen: %v", err)
	}
	if err := gw.CreateWorktree(ctx, "feat/diff-test"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Create and commit a file in the worktree.
	testFile := filepath.Join(gw.WorktreePath(), "diff-file.txt")
	if err := os.WriteFile(testFile, []byte("new content\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := gw.Commit(ctx, "feat: add diff file", nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	diff, err := gw.Diff(ctx, "main")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !strings.Contains(diff, "diff-file.txt") {
		t.Errorf("expected diff to mention diff-file.txt, got: %s", diff)
	}
	if !strings.Contains(diff, "new content") {
		t.Errorf("expected diff to contain 'new content'")
	}
}

func TestDiffFiles(t *testing.T) {
	dir := initTestRepo(t)
	repoDir := filepath.Join(dir, "repo")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", repoDir, logger)

	ctx := context.Background()
	if err := gw.CloneOrOpen(ctx); err != nil {
		t.Fatalf("CloneOrOpen: %v", err)
	}
	if err := gw.CreateWorktree(ctx, "feat/difffiles-test"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Create two files and commit.
	for _, name := range []string{"file-a.txt", "file-b.txt"} {
		path := filepath.Join(gw.WorktreePath(), name)
		if err := os.WriteFile(path, []byte("content\n"), 0644); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
	if err := gw.Commit(ctx, "feat: add two files", nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	files, err := gw.DiffFiles(ctx, "main")
	if err != nil {
		t.Fatalf("DiffFiles: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 changed files, got %d: %v", len(files), files)
	}
}

func TestDiffFiles_Empty(t *testing.T) {
	dir := initTestRepo(t)
	repoDir := filepath.Join(dir, "repo")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", repoDir, logger)

	ctx := context.Background()
	if err := gw.CloneOrOpen(ctx); err != nil {
		t.Fatalf("CloneOrOpen: %v", err)
	}
	if err := gw.CreateWorktree(ctx, "feat/empty-diff-test"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// No changes — DiffFiles should return nil/empty.
	files, err := gw.DiffFiles(ctx, "main")
	if err != nil {
		t.Fatalf("DiffFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 changed files, got %d: %v", len(files), files)
	}
}

func TestCreateWorktree_EmptyBranch(t *testing.T) {
	dir := initTestRepo(t)
	repoDir := filepath.Join(dir, "repo")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", repoDir, logger)

	ctx := context.Background()
	if err := gw.CloneOrOpen(ctx); err != nil {
		t.Fatalf("CloneOrOpen: %v", err)
	}

	// Empty branch name should auto-generate.
	if err := gw.CreateWorktree(ctx, ""); err != nil {
		t.Fatalf("CreateWorktree with empty branch: %v", err)
	}

	if gw.BranchName() == "" {
		t.Error("expected auto-generated branch name, got empty")
	}
	if !strings.HasPrefix(gw.BranchName(), "feat/agentbox-sprint-") {
		t.Errorf("expected branch to start with 'feat/agentbox-sprint-', got %q", gw.BranchName())
	}
}

func TestCommit_SpecificFiles(t *testing.T) {
	dir := initTestRepo(t)
	repoDir := filepath.Join(dir, "repo")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", repoDir, logger)

	ctx := context.Background()
	if err := gw.CloneOrOpen(ctx); err != nil {
		t.Fatalf("CloneOrOpen: %v", err)
	}
	if err := gw.CreateWorktree(ctx, "feat/specific-files"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Create two files.
	fileA := filepath.Join(gw.WorktreePath(), "include.txt")
	fileB := filepath.Join(gw.WorktreePath(), "exclude.txt")
	if err := os.WriteFile(fileA, []byte("include me"), 0644); err != nil {
		t.Fatalf("WriteFile(include): %v", err)
	}
	if err := os.WriteFile(fileB, []byte("exclude me"), 0644); err != nil {
		t.Fatalf("WriteFile(exclude): %v", err)
	}

	// Commit only include.txt.
	if err := gw.Commit(ctx, "feat: add include only", []string{"include.txt"}); err != nil {
		t.Fatalf("Commit specific files: %v", err)
	}

	// Verify exclude.txt is still untracked via git status.
	out, err := exec.CommandContext(ctx, "git", "status", "--porcelain").
		Output()
	// Run in worktree directory.
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = gw.WorktreePath()
	statusOut, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	_ = out // ignore first attempt
	if !strings.Contains(string(statusOut), "exclude.txt") {
		t.Error("expected exclude.txt to still be untracked")
	}
}

func TestWorkDir_WithWorktree(t *testing.T) {
	dir := initTestRepo(t)
	repoDir := filepath.Join(dir, "repo")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", repoDir, logger)

	ctx := context.Background()
	if err := gw.CloneOrOpen(ctx); err != nil {
		t.Fatalf("CloneOrOpen: %v", err)
	}
	if err := gw.CreateWorktree(ctx, "feat/workdir-test"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// workDir() should return the worktree path.
	wd := gw.workDir()
	if wd != gw.WorktreePath() {
		t.Errorf("expected workDir to return worktree path %q, got %q", gw.WorktreePath(), wd)
	}
}

func TestRepoDir_WithRepoURL(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("https://github.com/user/myrepo.git", "/base", logger)

	expected := filepath.Join("/base", "myrepo")
	if gw.RepoDir() != expected {
		t.Errorf("expected %q, got %q", expected, gw.RepoDir())
	}
}

func TestRepoDir_WithoutRepoURL(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", "/my/repo/path", logger)

	if gw.RepoDir() != "/my/repo/path" {
		t.Errorf("expected '/my/repo/path', got %q", gw.RepoDir())
	}
}

func TestCloneOrOpen_CloneAndFetch(t *testing.T) {
	// Set up a local "remote" repo.
	dir := initTestRepo(t)
	remoteDir := filepath.Join(dir, "repo")

	// Clone from local path.
	cloneBase := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow(remoteDir, cloneBase, logger)

	ctx := context.Background()
	if err := gw.CloneOrOpen(ctx); err != nil {
		t.Fatalf("CloneOrOpen (clone): %v", err)
	}

	// Verify it was cloned.
	clonedDir := gw.RepoDir()
	gitDir := filepath.Join(clonedDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Errorf("expected .git dir at %s", gitDir)
	}

	// Call again — should fetch, not re-clone.
	if err := gw.CloneOrOpen(ctx); err != nil {
		t.Fatalf("CloneOrOpen (fetch): %v", err)
	}
}

func TestRepoNameFromURL_Empty(t *testing.T) {
	got := repoNameFromURL("")
	if got != "" {
		t.Errorf("expected empty string for empty URL, got %q", got)
	}
}

func TestDetectBaseBranch_Master(t *testing.T) {
	// Test detectBaseBranch with a repo that has only master.
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	os.MkdirAll(repoDir, 0755)

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "checkout", "-b", "master"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("init command %v: %v", args, err)
		}
	}
	// Create a commit.
	os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Test\n"), 0644)
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = repoDir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "init")
	cmd.Dir = repoDir
	cmd.Run()

	// Create a bare remote clone with origin/master.
	bareDir := filepath.Join(dir, "bare.git")
	cmd = exec.Command("git", "clone", "--bare", repoDir, bareDir)
	cmd.Run()

	// Clone from bare to get origin refs.
	cloneDir := filepath.Join(dir, "clone")
	cmd = exec.Command("git", "clone", bareDir, cloneDir)
	cmd.Run()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", cloneDir, logger)
	ctx := context.Background()

	branch, err := gw.detectBaseBranch(ctx, cloneDir)
	if err != nil {
		t.Fatalf("detectBaseBranch: %v", err)
	}
	// Should detect origin/master.
	if branch != "origin/master" {
		t.Errorf("expected 'origin/master', got %q", branch)
	}
}

func TestDetectBaseBranch_Main(t *testing.T) {
	// Create a repo with main branch and clone it so origin/main exists.
	dir := initTestRepo(t) // creates repo with "main" branch
	remoteDir := filepath.Join(dir, "repo")

	bareDir := filepath.Join(dir, "bare.git")
	cmd := exec.Command("git", "clone", "--bare", remoteDir, bareDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("bare clone: %v", err)
	}

	cloneDir := filepath.Join(dir, "clone")
	cmd = exec.Command("git", "clone", bareDir, cloneDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("clone: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", cloneDir, logger)
	ctx := context.Background()

	branch, err := gw.detectBaseBranch(ctx, cloneDir)
	if err != nil {
		t.Fatalf("detectBaseBranch: %v", err)
	}
	if branch != "origin/main" {
		t.Errorf("expected 'origin/main', got %q", branch)
	}
}

func TestDetectBaseBranch_NeitherMainNorMaster(t *testing.T) {
	// Create a repo where the only remote branch is something other than main/master.
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	os.MkdirAll(repoDir, 0755)

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "checkout", "-b", "develop"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("init command %v: %v", args, err)
		}
	}
	os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Test\n"), 0644)
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = repoDir
	_ = cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "init")
	cmd.Dir = repoDir
	_ = cmd.Run()

	// Clone to get origin refs (only origin/develop, no main or master).
	bareDir := filepath.Join(dir, "bare.git")
	cmd = exec.Command("git", "clone", "--bare", repoDir, bareDir)
	_ = cmd.Run()

	cloneDir := filepath.Join(dir, "clone")
	cmd = exec.Command("git", "clone", bareDir, cloneDir)
	_ = cmd.Run()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", cloneDir, logger)
	ctx := context.Background()

	branch, err := gw.detectBaseBranch(ctx, cloneDir)
	if err != nil {
		t.Fatalf("detectBaseBranch: %v", err)
	}
	if branch != "HEAD" {
		t.Errorf("expected 'HEAD' when no main/master, got %q", branch)
	}
}

func TestCommit_AddError(t *testing.T) {
	// Test Commit with specific files that don't exist — should error on git add.
	dir := initTestRepo(t)
	repoDir := filepath.Join(dir, "repo")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", repoDir, logger)

	ctx := context.Background()
	if err := gw.CloneOrOpen(ctx); err != nil {
		t.Fatalf("CloneOrOpen: %v", err)
	}
	if err := gw.CreateWorktree(ctx, "feat/add-error-test"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Try to commit a non-existent file — git add should fail.
	err := gw.Commit(ctx, "feat: bad file", []string{"nonexistent-file.go"})
	if err == nil {
		t.Error("expected error when adding nonexistent file")
	}
}

func TestDetectBaseBranch_Error(t *testing.T) {
	// When git branch -r fails, detectBaseBranch should return "main" as fallback.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", t.TempDir(), logger) // not a git repo, so git branch -r will fail

	branch, err := gw.detectBaseBranch(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("detectBaseBranch: %v", err)
	}
	if branch != "main" {
		t.Errorf("expected 'main' fallback, got %q", branch)
	}
}

func TestCurrentCommit_Error(t *testing.T) {
	// Test CurrentCommit on a non-git directory.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", t.TempDir(), logger)

	_, err := gw.CurrentCommit(context.Background())
	if err == nil {
		t.Error("expected error for CurrentCommit on non-git dir")
	}
}

func TestDiffFiles_Error(t *testing.T) {
	// Test DiffFiles when git fails.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", t.TempDir(), logger)

	_, err := gw.DiffFiles(context.Background(), "main")
	if err == nil {
		t.Error("expected error for DiffFiles on non-git dir")
	}
}

func TestWorkDir_WithoutWorktree(t *testing.T) {
	dir := initTestRepo(t)
	repoDir := filepath.Join(dir, "repo")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw := NewGitWorkflow("", repoDir, logger)

	// Without calling CreateWorktree, workDir should return RepoDir.
	wd := gw.workDir()
	expected := gw.RepoDir()
	if wd != expected {
		t.Errorf("expected workDir to return repo dir %q, got %q", expected, wd)
	}
}

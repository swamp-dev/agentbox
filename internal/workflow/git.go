// Package workflow provides git workflow automation for agentbox.
package workflow

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GitWorkflow manages git operations for the supervisor lifecycle.
type GitWorkflow struct {
	repoURL      string
	baseDir      string
	worktreePath string
	branchName   string
	logger       *slog.Logger
}

// NewGitWorkflow creates a new GitWorkflow.
func NewGitWorkflow(repoURL, baseDir string, logger *slog.Logger) *GitWorkflow {
	return &GitWorkflow{
		repoURL: repoURL,
		baseDir: baseDir,
		logger:  logger,
	}
}

// WorktreePath returns the current worktree path.
func (g *GitWorkflow) WorktreePath() string {
	return g.worktreePath
}

// BranchName returns the current branch name.
func (g *GitWorkflow) BranchName() string {
	return g.branchName
}

// RepoDir returns the path to the main repo clone.
func (g *GitWorkflow) RepoDir() string {
	if g.repoURL == "" {
		return g.baseDir
	}
	name := repoNameFromURL(g.repoURL)
	return filepath.Join(g.baseDir, name)
}

// CloneOrOpen clones the repo if needed, or validates the existing directory.
func (g *GitWorkflow) CloneOrOpen(ctx context.Context) error {
	if g.repoURL == "" {
		// Use baseDir as the repo directly.
		gitDir := filepath.Join(g.baseDir, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			return fmt.Errorf("not a git repository: %s", g.baseDir)
		}
		g.logger.Info("using existing repository", "path", g.baseDir)
		return nil
	}

	repoDir := g.RepoDir()
	gitDir := filepath.Join(repoDir, ".git")

	if _, err := os.Stat(gitDir); err == nil {
		g.logger.Info("repository already cloned", "path", repoDir)
		return g.git(ctx, repoDir, "fetch", "origin")
	}

	g.logger.Info("cloning repository", "url", g.repoURL, "path", repoDir)
	return g.gitGlobal(ctx, "clone", g.repoURL, repoDir)
}

// CreateWorktree creates a new worktree with a feature branch from main/master.
func (g *GitWorkflow) CreateWorktree(ctx context.Context, branchName string) error {
	if branchName == "" {
		branchName = fmt.Sprintf("feat/agentbox-sprint-%s", time.Now().Format("20060102-1504"))
	}
	g.branchName = branchName

	repoDir := g.RepoDir()

	// Determine base branch.
	baseBranch, err := g.detectBaseBranch(ctx, repoDir)
	if err != nil {
		return fmt.Errorf("detecting base branch: %w", err)
	}

	// Create worktree path as sibling to repo.
	worktreeName := strings.ReplaceAll(branchName, "/", "-")
	g.worktreePath = filepath.Join(filepath.Dir(repoDir), worktreeName)

	g.logger.Info("creating worktree",
		"branch", branchName,
		"base", baseBranch,
		"path", g.worktreePath,
	)

	err = g.git(ctx, repoDir, "worktree", "add", "-b", branchName, g.worktreePath, baseBranch)
	if err != nil {
		return fmt.Errorf("creating worktree: %w", err)
	}

	return nil
}

// Commit stages specified files and commits with a conventional message.
func (g *GitWorkflow) Commit(ctx context.Context, msg string, files []string) error {
	dir := g.workDir()

	if len(files) == 0 {
		// Stage all changes.
		if err := g.git(ctx, dir, "add", "-A"); err != nil {
			return err
		}
	} else {
		args := append([]string{"add"}, files...)
		if err := g.git(ctx, dir, args...); err != nil {
			return err
		}
	}

	// Check if there's anything to commit.
	out, err := g.gitOutput(ctx, dir, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		g.logger.Debug("nothing to commit")
		return nil
	}

	commitMsg := msg + "\n\nCo-Authored-By: agentbox <noreply@agentbox.dev>"
	return g.git(ctx, dir, "commit", "-m", commitMsg)
}

// OpenPR creates a pull request via gh CLI.
func (g *GitWorkflow) OpenPR(ctx context.Context, title, body string) (string, error) {
	dir := g.workDir()

	// Push the branch.
	if err := g.git(ctx, dir, "push", "-u", "origin", g.branchName); err != nil {
		return "", fmt.Errorf("pushing branch: %w", err)
	}

	// Create PR.
	out, err := g.cmdOutput(ctx, dir, "gh", "pr", "create", "--title", title, "--body", body)
	if err != nil {
		return "", fmt.Errorf("creating PR: %w", err)
	}

	prURL := strings.TrimSpace(out)
	g.logger.Info("pull request created", "url", prURL)
	return prURL, nil
}

// CurrentCommit returns the HEAD SHA.
func (g *GitWorkflow) CurrentCommit(ctx context.Context) (string, error) {
	out, err := g.gitOutput(ctx, g.workDir(), "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Rollback resets the worktree to a specific commit.
func (g *GitWorkflow) Rollback(ctx context.Context, commitSHA string) error {
	g.logger.Warn("rolling back", "commit", commitSHA)
	return g.git(ctx, g.workDir(), "reset", "--hard", commitSHA)
}

// Diff returns the diff between the current branch and its base.
func (g *GitWorkflow) Diff(ctx context.Context, baseBranch string) (string, error) {
	return g.gitOutput(ctx, g.workDir(), "diff", baseBranch+"...HEAD")
}

// DiffFiles returns the list of changed files compared to base.
func (g *GitWorkflow) DiffFiles(ctx context.Context, baseBranch string) ([]string, error) {
	out, err := g.gitOutput(ctx, g.workDir(), "diff", "--name-only", baseBranch+"...HEAD")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	return strings.Split(strings.TrimSpace(out), "\n"), nil
}

// workDir returns the directory to run git commands in.
func (g *GitWorkflow) workDir() string {
	if g.worktreePath != "" {
		return g.worktreePath
	}
	return g.RepoDir()
}

// detectBaseBranch finds main or master.
func (g *GitWorkflow) detectBaseBranch(ctx context.Context, repoDir string) (string, error) {
	out, err := g.gitOutput(ctx, repoDir, "branch", "-r")
	if err != nil {
		return "main", nil
	}
	if strings.Contains(out, "origin/main") {
		return "origin/main", nil
	}
	if strings.Contains(out, "origin/master") {
		return "origin/master", nil
	}
	return "HEAD", nil
}

// git runs a git command in the given directory.
func (g *GitWorkflow) git(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	g.logger.Debug("git", "args", args, "dir", dir)
	return cmd.Run()
}

// gitOutput runs a git command and returns stdout.
func (g *GitWorkflow) gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), stderr.String(), err)
	}
	return stdout.String(), nil
}

// gitGlobal runs a git command without a specific directory.
func (g *GitWorkflow) gitGlobal(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// cmdOutput runs any command and returns stdout.
func (g *GitWorkflow) cmdOutput(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %s: %w", name, stderr.String(), err)
	}
	return stdout.String(), nil
}

// repoNameFromURL extracts the repository name from a URL.
func repoNameFromURL(url string) string {
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		return "repo"
	}
	return parts[len(parts)-1]
}

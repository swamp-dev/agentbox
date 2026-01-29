package ralph

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swamp-dev/agentbox/internal/agent"
	"github.com/swamp-dev/agentbox/internal/config"
	"github.com/swamp-dev/agentbox/internal/container"
)

// randomSuffix generates a random hex string for unique container names.
func randomSuffix() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Loop manages the Ralph pattern execution.
type Loop struct {
	cfg       *config.Config
	prd       *PRD
	progress  *Progress
	agent     agent.Agent
	container *container.Manager
	logger    *slog.Logger

	projectPath string
	iteration   int
}

// NewLoop creates a new Ralph loop executor.
func NewLoop(cfg *config.Config, projectPath string, logger *slog.Logger) (*Loop, error) {
	ag, err := agent.New(cfg.Agent.Name)
	if err != nil {
		return nil, err
	}

	cm, err := container.NewManager()
	if err != nil {
		return nil, err
	}

	prd, err := LoadPRD(projectPath + "/" + cfg.Ralph.PRDFile)
	if err != nil {
		return nil, fmt.Errorf("loading PRD: %w", err)
	}

	progress := NewProgress(projectPath + "/" + cfg.Ralph.ProgressFile)
	if err := progress.Load(); err != nil {
		return nil, fmt.Errorf("loading progress: %w", err)
	}

	return &Loop{
		cfg:         cfg,
		prd:         prd,
		progress:    progress,
		agent:       ag,
		container:   cm,
		logger:      logger,
		projectPath: projectPath,
	}, nil
}

// Close releases resources.
func (l *Loop) Close() error {
	return l.container.Close()
}

// Run executes the Ralph loop until completion or max iterations.
func (l *Loop) Run(ctx context.Context) error {
	l.logger.Info("starting Ralph loop",
		"max_iterations", l.cfg.Ralph.MaxIterations,
		"prd", l.prd.Name,
		"progress", fmt.Sprintf("%.1f%%", l.prd.Progress()),
	)

	for l.iteration = 1; l.iteration <= l.cfg.Ralph.MaxIterations; l.iteration++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if l.prd.IsComplete() {
			l.logger.Info("all tasks completed", "iterations", l.iteration-1)
			return nil
		}

		if err := l.runIteration(ctx); err != nil {
			l.logger.Error("iteration failed", "iteration", l.iteration, "error", err)
			return err
		}
	}

	l.logger.Warn("max iterations reached", "max", l.cfg.Ralph.MaxIterations)
	return fmt.Errorf("max iterations (%d) reached", l.cfg.Ralph.MaxIterations)
}

// runIteration executes a single iteration of the Ralph loop.
func (l *Loop) runIteration(ctx context.Context) error {
	task := l.prd.NextTask()
	if task == nil {
		return fmt.Errorf("no available tasks")
	}

	l.logger.Info("starting iteration",
		"iteration", l.iteration,
		"task", task.ID,
		"title", task.Title,
	)

	if err := l.prd.MarkTaskInProgress(task.ID); err != nil {
		return err
	}
	l.progress.RecordStart(task.ID, task.Title)

	prompt := l.buildPrompt(task)

	output, err := l.runAgent(ctx, prompt)
	if err != nil {
		l.progress.RecordFailed(task.ID, task.Title, err.Error())
		return fmt.Errorf("agent execution failed: %w", err)
	}

	result := l.agent.ParseOutput(output)

	if !result.Success {
		l.progress.RecordFailed(task.ID, task.Title, result.Message)
		return fmt.Errorf("agent reported failure: %s", result.Message)
	}

	if err := l.runQualityChecks(ctx); err != nil {
		l.progress.RecordFailed(task.ID, task.Title, fmt.Sprintf("quality check failed: %s", err))
		return fmt.Errorf("quality checks failed: %w", err)
	}

	if l.cfg.Ralph.AutoCommit {
		if err := l.commitChanges(ctx, task); err != nil {
			l.logger.Warn("commit failed", "error", err)
		}
	}

	learnings := l.extractLearnings(output)
	if err := l.prd.MarkTaskComplete(task.ID, strings.Join(learnings, "; ")); err != nil {
		return err
	}

	if err := l.prd.Save(l.projectPath + "/" + l.cfg.Ralph.PRDFile); err != nil {
		return fmt.Errorf("saving PRD: %w", err)
	}

	l.progress.RecordComplete(task.ID, task.Title, "Task completed successfully", learnings)

	l.logger.Info("iteration completed",
		"iteration", l.iteration,
		"task", task.ID,
		"progress", fmt.Sprintf("%.1f%%", l.prd.Progress()),
	)

	return nil
}

// buildPrompt creates the agent prompt for a task.
func (l *Loop) buildPrompt(task *Task) string {
	var sb strings.Builder

	sb.WriteString("You are working on: ")
	sb.WriteString(l.prd.Name)
	sb.WriteString("\n\n")

	sb.WriteString("Current task:\n")
	sb.WriteString(fmt.Sprintf("ID: %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("Title: %s\n", task.Title))
	sb.WriteString(fmt.Sprintf("Description: %s\n", task.Description))
	sb.WriteString("\n")

	sb.WriteString("Instructions:\n")
	sb.WriteString("1. Complete the task described above\n")
	sb.WriteString("2. Make small, focused changes\n")
	sb.WriteString("3. Ensure your changes are complete and tested\n")
	sb.WriteString("4. When the task is FULLY complete, output: ")
	sb.WriteString(l.cfg.Ralph.StopSignal)
	sb.WriteString("\n\n")

	sb.WriteString("Important: Only output the completion signal when the task is truly done.\n")

	return sb.String()
}

// runAgent executes the agent in a container.
func (l *Loop) runAgent(ctx context.Context, prompt string) (string, error) {
	cmd := l.agent.Command(prompt)
	env := l.agent.Environment()

	containerCfg, err := container.ConfigToContainerConfig(l.cfg, l.projectPath, cmd, env)
	if err != nil {
		return "", err
	}

	containerCfg.Name = fmt.Sprintf("agentbox-%s-iter-%d-%s", l.cfg.Project.Name, l.iteration, randomSuffix())

	return l.container.Run(ctx, containerCfg)
}

// allowedQualityCheckCommands is a whitelist of safe command prefixes.
var allowedQualityCheckCommands = []string{
	"npm", "npx", "pnpm", "yarn", "bun",
	"go", "cargo", "rustc",
	"python", "python3", "pytest", "pip",
	"make", "gradle", "mvn",
	"eslint", "prettier", "tsc", "jest", "vitest", "mocha",
}

// validateQualityCheckCommand ensures the command starts with an allowed prefix.
func validateQualityCheckCommand(command string) error {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	base := filepath.Base(parts[0])
	for _, allowed := range allowedQualityCheckCommands {
		if base == allowed {
			return nil
		}
	}

	return fmt.Errorf("command not in allowlist: %s (allowed: %v)", base, allowedQualityCheckCommands)
}

// runQualityChecks executes all configured quality checks.
func (l *Loop) runQualityChecks(ctx context.Context) error {
	for _, check := range l.cfg.Ralph.QualityChecks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := validateQualityCheckCommand(check.Command); err != nil {
			return fmt.Errorf("invalid quality check %s: %w", check.Name, err)
		}

		l.logger.Debug("running quality check", "name", check.Name)

		cmd := exec.CommandContext(ctx, "sh", "-c", check.Command)
		cmd.Dir = l.projectPath

		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %s", check.Name, string(output))
		}
	}

	return nil
}

// commitChanges commits the current changes to git.
func (l *Loop) commitChanges(ctx context.Context, task *Task) error {
	gitDir := filepath.Join(l.projectPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		l.logger.Debug("skipping git commit - not a git repository")
		return nil
	}

	addCmd := exec.CommandContext(ctx, "git", "add", "-A")
	addCmd.Dir = l.projectPath
	if err := addCmd.Run(); err != nil {
		return err
	}

	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	statusCmd.Dir = l.projectPath
	output, _ := statusCmd.Output()
	if len(output) == 0 {
		return nil
	}

	message := fmt.Sprintf("feat: %s\n\nTask ID: %s\nIteration: %d\n\nGenerated by agentbox Ralph loop",
		task.Title, task.ID, l.iteration)

	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	commitCmd.Dir = l.projectPath
	return commitCmd.Run()
}

// extractLearnings attempts to extract learnings from the agent output.
func (l *Loop) extractLearnings(output string) []string {
	var learnings []string

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(strings.ToLower(line), "learning:") ||
			strings.HasPrefix(strings.ToLower(line), "note:") ||
			strings.HasPrefix(strings.ToLower(line), "important:") {
			learning := strings.TrimPrefix(line, "Learning:")
			learning = strings.TrimPrefix(learning, "learning:")
			learning = strings.TrimPrefix(learning, "Note:")
			learning = strings.TrimPrefix(learning, "note:")
			learning = strings.TrimPrefix(learning, "Important:")
			learning = strings.TrimPrefix(learning, "important:")
			learnings = append(learnings, strings.TrimSpace(learning))
		}
	}

	return learnings
}

// Status returns the current loop status.
func (l *Loop) Status() *LoopStatus {
	return &LoopStatus{
		PRDName:       l.prd.Name,
		TotalTasks:    l.prd.Metadata.TotalTasks,
		Completed:     l.prd.Metadata.Completed,
		InProgress:    l.prd.Metadata.InProgress,
		Pending:       l.prd.Metadata.Pending,
		Progress:      l.prd.Progress(),
		MaxIterations: l.cfg.Ralph.MaxIterations,
		Iteration:     l.iteration,
	}
}

// LoopStatus represents the current state of the Ralph loop.
type LoopStatus struct {
	PRDName       string
	TotalTasks    int
	Completed     int
	InProgress    int
	Pending       int
	Progress      float64
	MaxIterations int
	Iteration     int
}

// String returns a formatted status string.
func (s *LoopStatus) String() string {
	return fmt.Sprintf(
		"PRD: %s\nProgress: %.1f%% (%d/%d tasks)\nStatus: %d completed, %d in progress, %d pending\nIteration: %d/%d",
		s.PRDName,
		s.Progress,
		s.Completed,
		s.TotalTasks,
		s.Completed,
		s.InProgress,
		s.Pending,
		s.Iteration,
		s.MaxIterations,
	)
}

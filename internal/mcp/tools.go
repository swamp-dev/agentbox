package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/swamp-dev/agentbox/internal/agent"
	"github.com/swamp-dev/agentbox/internal/config"
	"github.com/swamp-dev/agentbox/internal/container"
	"github.com/swamp-dev/agentbox/internal/journal"
	"github.com/swamp-dev/agentbox/internal/ralph"
	"github.com/swamp-dev/agentbox/internal/store"
	"github.com/swamp-dev/agentbox/internal/supervisor"
)

const (
	// defaultRunTimeout is the maximum time an agentbox_run call can take before
	// the container is killed. Prevents the MCP server from hanging indefinitely.
	defaultRunTimeout = 30 * time.Minute

	// defaultAsyncTimeout is the maximum time async operations (ralph, sprint)
	// can run before being cancelled.
	defaultAsyncTimeout = 4 * time.Hour
)

// ToolHandler dispatches MCP tool calls to implementations.
type ToolHandler struct {
	mu       sync.Mutex
	sessions map[string]*asyncSession
	logger   *slog.Logger
}

type asyncSession struct {
	ID     string
	Status string // "running", "completed", "failed"
	Error  string
}

// NewToolHandler creates a new tool handler with the given logger.
func NewToolHandler(logger *slog.Logger) *ToolHandler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	}
	return &ToolHandler{
		sessions: make(map[string]*asyncSession),
		logger:   logger,
	}
}

// Call dispatches a tool call by name and returns the MCP result.
func (h *ToolHandler) Call(name string, argsJSON json.RawMessage) *ToolCallResult {
	switch name {
	case "agentbox_run":
		return h.handleRun(argsJSON)
	case "agentbox_ralph_start":
		return h.handleRalphStart(argsJSON)
	case "agentbox_sprint_start":
		return h.handleSprintStart(argsJSON)
	case "agentbox_status":
		return h.handleStatus(argsJSON)
	case "agentbox_journal":
		return h.handleJournal(argsJSON)
	case "agentbox_task_list":
		return h.handleTaskList(argsJSON)
	case "agentbox_sprint_status":
		return h.handleSprintStatus(argsJSON)
	default:
		return textError(fmt.Sprintf("unknown tool: %s", name))
	}
}

// --- agentbox_run ---

type runArgs struct {
	ProjectDir string `json:"project_dir"`
	Agent      string `json:"agent"`
	Prompt     string `json:"prompt"`
	Image      string `json:"image,omitempty"`
	Network    string `json:"network,omitempty"`
	Timeout    int    `json:"timeout,omitempty"` // timeout in minutes (default: 30)
}

func (h *ToolHandler) handleRun(argsJSON json.RawMessage) *ToolCallResult {
	var args runArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return textError(fmt.Sprintf("invalid arguments: %v", err))
	}

	if args.ProjectDir == "" || args.Agent == "" || args.Prompt == "" {
		return textError("project_dir, agent, and prompt are required")
	}

	if err := agent.ValidateAPIKey(args.Agent); err != nil {
		return textError(fmt.Sprintf("API key validation failed: %v", err))
	}

	cfg, err := config.Load("")
	if err != nil {
		return textError(fmt.Sprintf("loading config: %v", err))
	}

	cfg.Agent.Name = args.Agent
	if args.Image != "" {
		cfg.Docker.Image = args.Image
	}
	if args.Network != "" {
		cfg.Docker.Network = args.Network
	}
	if args.Agent == "claude-cli" && cfg.Docker.Network == "none" {
		cfg.Docker.Network = "bridge"
	}

	if err := cfg.Validate(); err != nil {
		return textError(fmt.Sprintf("invalid configuration: %v", err))
	}

	ag, err := agent.New(args.Agent)
	if err != nil {
		return textError(fmt.Sprintf("creating agent: %v", err))
	}

	cm, err := container.NewManager()
	if err != nil {
		return textError(fmt.Sprintf("creating container manager: %v", err))
	}
	defer cm.Close()

	agentCmd := ag.Command(args.Prompt)
	env := ag.Environment()

	containerCfg, err := container.ConfigToContainerConfig(cfg, args.ProjectDir, agentCmd, env)
	if err != nil {
		return textError(fmt.Sprintf("building container config: %v", err))
	}

	timeout := defaultRunTimeout
	if args.Timeout > 0 {
		timeout = time.Duration(args.Timeout) * time.Minute
	}
	if timeout > defaultAsyncTimeout {
		return textError(fmt.Sprintf("timeout %d minutes exceeds maximum of %d minutes", args.Timeout, int(defaultAsyncTimeout.Minutes())))
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	output, err := cm.Run(ctx, containerCfg)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return textError(fmt.Sprintf("agent execution timed out after %s\n\nPartial output:\n%s", timeout, output))
		}
		return textError(fmt.Sprintf("agent execution failed: %v\n\nOutput:\n%s", err, output))
	}

	result := ag.ParseOutput(output)

	data, err := json.Marshal(map[string]interface{}{
		"success":   result.Success,
		"completed": result.Completed,
		"output":    output,
	})
	if err != nil {
		return textError(fmt.Sprintf("marshaling result: %v", err))
	}
	return textResult(string(data))
}

// --- agentbox_ralph_start ---

type ralphStartArgs struct {
	ProjectDir    string `json:"project_dir"`
	Agent         string `json:"agent,omitempty"`
	PRDFile       string `json:"prd_file,omitempty"`
	MaxIterations int    `json:"max_iterations,omitempty"`
}

func (h *ToolHandler) handleRalphStart(argsJSON json.RawMessage) *ToolCallResult {
	var args ralphStartArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return textError(fmt.Sprintf("invalid arguments: %v", err))
	}

	if args.ProjectDir == "" {
		return textError("project_dir is required")
	}

	cfg, err := config.Load("")
	if err != nil {
		return textError(fmt.Sprintf("loading config: %v", err))
	}

	if args.Agent != "" {
		cfg.Agent.Name = args.Agent
	}
	if args.PRDFile != "" {
		cfg.Ralph.PRDFile = args.PRDFile
	}
	if args.MaxIterations > 0 {
		cfg.Ralph.MaxIterations = args.MaxIterations
	}

	sessionID := uuid.New().String()

	h.mu.Lock()
	h.sessions[sessionID] = &asyncSession{ID: sessionID, Status: "running"}
	h.mu.Unlock()

	go func() {
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
		loop, err := ralph.NewLoop(cfg, args.ProjectDir, logger)
		if err != nil {
			h.mu.Lock()
			h.sessions[sessionID].Status = "failed"
			h.sessions[sessionID].Error = err.Error()
			h.mu.Unlock()
			return
		}
		defer loop.Close()

		ctx, cancel := context.WithTimeout(context.Background(), defaultAsyncTimeout)
		defer cancel()
		if err := loop.Run(ctx); err != nil {
			h.mu.Lock()
			h.sessions[sessionID].Status = "failed"
			h.sessions[sessionID].Error = err.Error()
			h.mu.Unlock()
			return
		}

		h.mu.Lock()
		h.sessions[sessionID].Status = "completed"
		h.mu.Unlock()
	}()

	data, err := json.Marshal(map[string]string{
		"session_id": sessionID,
		"message":    "Ralph loop started",
	})
	if err != nil {
		return textError(fmt.Sprintf("marshaling result: %v", err))
	}
	return textResult(string(data))
}

// --- agentbox_sprint_start ---

type sprintStartArgs struct {
	ProjectDir string `json:"project_dir,omitempty"`
	RepoURL    string `json:"repo_url,omitempty"`
	PRDFile    string `json:"prd_file,omitempty"`
	Agent      string `json:"agent,omitempty"`
	SprintSize int    `json:"sprint_size,omitempty"`
	MaxSprints int    `json:"max_sprints,omitempty"`
}

func (h *ToolHandler) handleSprintStart(argsJSON json.RawMessage) *ToolCallResult {
	var args sprintStartArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return textError(fmt.Sprintf("invalid arguments: %v", err))
	}

	cfg := supervisor.DefaultConfig()
	if args.ProjectDir != "" {
		cfg.WorkDir = args.ProjectDir
	}
	if args.RepoURL != "" {
		cfg.RepoURL = args.RepoURL
	}
	if args.PRDFile != "" {
		cfg.PRDFile = args.PRDFile
	}
	if args.Agent != "" {
		cfg.Agent = args.Agent
	}
	if args.SprintSize > 0 {
		cfg.SprintSize = args.SprintSize
	}
	if args.MaxSprints > 0 {
		cfg.MaxSprints = args.MaxSprints
	}

	sessionID := uuid.New().String()

	h.mu.Lock()
	h.sessions[sessionID] = &asyncSession{ID: sessionID, Status: "running"}
	h.mu.Unlock()

	go func() {
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
		sup, err := supervisor.New(cfg, logger)
		if err != nil {
			h.mu.Lock()
			h.sessions[sessionID].Status = "failed"
			h.sessions[sessionID].Error = err.Error()
			h.mu.Unlock()
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultAsyncTimeout)
		defer cancel()
		if err := sup.Run(ctx); err != nil {
			h.mu.Lock()
			h.sessions[sessionID].Status = "failed"
			h.sessions[sessionID].Error = err.Error()
			h.mu.Unlock()
			return
		}

		h.mu.Lock()
		h.sessions[sessionID].Status = "completed"
		h.mu.Unlock()
	}()

	data, err := json.Marshal(map[string]string{
		"session_id": sessionID,
		"message":    "Sprint started",
	})
	if err != nil {
		return textError(fmt.Sprintf("marshaling result: %v", err))
	}
	return textResult(string(data))
}

// --- agentbox_status ---

type statusArgs struct {
	SessionID  string `json:"session_id,omitempty"`
	ProjectDir string `json:"project_dir,omitempty"`
}

func (h *ToolHandler) handleStatus(argsJSON json.RawMessage) *ToolCallResult {
	var args statusArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return textError(fmt.Sprintf("invalid arguments: %v", err))
	}

	// Check in-memory async sessions first.
	if args.SessionID != "" {
		h.mu.Lock()
		sess, ok := h.sessions[args.SessionID]
		var snap asyncSession
		if ok {
			snap = *sess
		}
		h.mu.Unlock()
		if ok {
			data, err := json.Marshal(map[string]string{
				"session_id": snap.ID,
				"status":     snap.Status,
				"error":      snap.Error,
			})
			if err != nil {
				return textError(fmt.Sprintf("marshaling result: %v", err))
			}
			return textResult(string(data))
		}
	}

	// Try to open the store and get session info.
	projectDir := args.ProjectDir
	if projectDir == "" {
		projectDir = "."
	}

	dbPath := filepath.Join(projectDir, ".agentbox", "agentbox.db")
	s, err := store.Open(dbPath)
	if err != nil {
		return textResult(fmt.Sprintf("No active sessions found (store not available: %v)", err))
	}
	defer s.Close()

	sess, err := s.LatestSession()
	if err != nil {
		return textResult("No sessions found")
	}

	tasks, err := s.ListTasks(sess.ID)
	if err != nil {
		tasks = nil
	}

	var completed, inProgress, pending int
	for _, t := range tasks {
		switch t.Status {
		case "completed":
			completed++
		case "in_progress":
			inProgress++
		default:
			pending++
		}
	}

	total := len(tasks)
	var progress float64
	if total > 0 {
		progress = float64(completed) / float64(total) * 100
	}

	result := map[string]interface{}{
		"session_id": sess.ID,
		"status":     sess.Status,
		"started_at": sess.StartedAt.Format("2006-01-02T15:04:05Z"),
		"tasks": map[string]interface{}{
			"total":       total,
			"completed":   completed,
			"in_progress": inProgress,
			"pending":     pending,
		},
		"progress_pct": fmt.Sprintf("%.1f%%", progress),
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return textError(fmt.Sprintf("marshaling result: %v", err))
	}
	return textResult(string(data))
}

// --- agentbox_journal ---

type journalArgs struct {
	SessionID  string `json:"session_id"`
	Limit      int    `json:"limit,omitempty"`
	ProjectDir string `json:"project_dir,omitempty"`
}

func (h *ToolHandler) handleJournal(argsJSON json.RawMessage) *ToolCallResult {
	var args journalArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return textError(fmt.Sprintf("invalid arguments: %v", err))
	}

	if args.SessionID == "" {
		return textError("session_id is required")
	}

	projectDir := args.ProjectDir
	if projectDir == "" {
		projectDir = "."
	}

	dbPath := filepath.Join(projectDir, ".agentbox", "agentbox.db")
	s, err := store.Open(dbPath)
	if err != nil {
		return textError(fmt.Sprintf("opening store: %v", err))
	}
	defer s.Close()

	// The journal/store layer uses int64 session IDs internally.
	// Look up the latest session from the store to resolve the numeric ID.
	sess, err := s.LatestSession()
	if err != nil {
		return textError(fmt.Sprintf("looking up session: %v", err))
	}

	j := journal.New(s, sess.ID)
	query := &store.JournalQuery{}
	if args.Limit > 0 {
		query.Limit = args.Limit
	}

	entries, err := j.Entries(query)
	if err != nil {
		return textError(fmt.Sprintf("reading journal: %v", err))
	}

	if len(entries) == 0 {
		return textResult("No journal entries found for this session")
	}

	var result string
	for _, e := range entries {
		result += journal.RenderEntry(e) + "\n---\n\n"
	}

	return textResult(result)
}

// --- agentbox_task_list ---

type taskListArgs struct {
	PRDFile    string `json:"prd_file,omitempty"`
	ProjectDir string `json:"project_dir,omitempty"`
}

func (h *ToolHandler) handleTaskList(argsJSON json.RawMessage) *ToolCallResult {
	var args taskListArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return textError(fmt.Sprintf("invalid arguments: %v", err))
	}

	projectDir := args.ProjectDir
	if projectDir == "" {
		projectDir = "."
	}

	prdFile := args.PRDFile
	if prdFile == "" {
		prdFile = "prd.json"
	}

	prdPath := filepath.Join(projectDir, prdFile)
	prd, err := ralph.LoadPRD(prdPath)
	if err != nil {
		return textResult(fmt.Sprintf("Could not load PRD from %s: %v", prdPath, err))
	}

	tasks := prd.ExportTasks()
	type taskSummary struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Status      string `json:"status"`
		Description string `json:"description"`
	}

	summaries := make([]taskSummary, len(tasks))
	for i, t := range tasks {
		summaries[i] = taskSummary{
			ID:          t.ID,
			Title:       t.Title,
			Status:      t.Status,
			Description: t.Description,
		}
	}

	data, err := json.MarshalIndent(map[string]interface{}{
		"prd_name": prd.Name,
		"progress": fmt.Sprintf("%.1f%%", prd.Progress()),
		"tasks":    summaries,
	}, "", "  ")
	if err != nil {
		return textError(fmt.Sprintf("marshaling result: %v", err))
	}

	return textResult(string(data))
}

// --- agentbox_sprint_status ---

type sprintStatusArgs struct {
	SessionID  string `json:"session_id"`
	ProjectDir string `json:"project_dir,omitempty"`
}

func (h *ToolHandler) handleSprintStatus(argsJSON json.RawMessage) *ToolCallResult {
	var args sprintStatusArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return textError(fmt.Sprintf("invalid arguments: %v", err))
	}

	if args.SessionID == "" {
		return textError("session_id is required")
	}

	// Check in-memory async sessions, copying fields while lock is held.
	h.mu.Lock()
	sess, ok := h.sessions[args.SessionID]
	var snap asyncSession
	if ok {
		snap = *sess
	}
	h.mu.Unlock()
	if ok {
		data, err := json.MarshalIndent(map[string]interface{}{
			"session_id": snap.ID,
			"status":     snap.Status,
			"error":      snap.Error,
		}, "", "  ")
		if err != nil {
			return textError(fmt.Sprintf("marshaling result: %v", err))
		}
		return textResult(string(data))
	}

	// Try to look up in the store.
	projectDir := args.ProjectDir
	if projectDir == "" {
		projectDir = "."
	}
	dbPath := filepath.Join(projectDir, ".agentbox", "agentbox.db")
	s, err := store.Open(dbPath)
	if err != nil {
		return textResult(fmt.Sprintf("No active sprint found for session %s (store not available)", args.SessionID))
	}
	defer s.Close()

	latestSess, err := s.LatestSession()
	if err != nil {
		return textResult("No sessions found")
	}

	tasks, err := s.ListTasks(latestSess.ID)
	if err != nil {
		tasks = nil
	}

	var completed, total int
	total = len(tasks)
	for _, t := range tasks {
		if t.Status == "completed" {
			completed++
		}
	}

	result := map[string]interface{}{
		"session_id": latestSess.ID,
		"status":     latestSess.Status,
		"tasks": map[string]int{
			"total":     total,
			"completed": completed,
		},
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return textError(fmt.Sprintf("marshaling result: %v", err))
	}
	return textResult(string(data))
}

// --- helpers ---

func textResult(text string) *ToolCallResult {
	return &ToolCallResult{
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

func textError(text string) *ToolCallResult {
	return &ToolCallResult{
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
		IsError: true,
	}
}

// AllTools returns the definitions of all available MCP tools.
func AllTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "agentbox_run",
			Description: "Run a single agent task in a sandboxed Docker container. Returns the agent output text and success status.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_dir": map[string]interface{}{
						"type":        "string",
						"description": "Path to the project directory to mount in the container",
					},
					"agent": map[string]interface{}{
						"type":        "string",
						"description": "Agent to use (claude, claude-cli, amp, aider)",
						"enum":        []string{"claude", "claude-cli", "amp", "aider"},
					},
					"prompt": map[string]interface{}{
						"type":        "string",
						"description": "Prompt to send to the agent",
					},
					"image": map[string]interface{}{
						"type":        "string",
						"description": "Docker image type (node, python, go, rust, full)",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network mode (none, bridge, host)",
					},
					"timeout": map[string]interface{}{
						"type":        "integer",
						"description": "Timeout in minutes (default: 30). Container is killed if exceeded.",
					},
				},
				"required": []string{"project_dir", "agent", "prompt"},
			},
		},
		{
			Name:        "agentbox_ralph_start",
			Description: "Start a Ralph loop for a PRD (async). Returns a session ID immediately that can be polled with agentbox_status.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_dir": map[string]interface{}{
						"type":        "string",
						"description": "Path to the project directory",
					},
					"agent": map[string]interface{}{
						"type":        "string",
						"description": "Agent to use (default: claude)",
					},
					"prd_file": map[string]interface{}{
						"type":        "string",
						"description": "PRD file name relative to project_dir (default: prd.json)",
					},
					"max_iterations": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of iterations (default: 10)",
					},
				},
				"required": []string{"project_dir"},
			},
		},
		{
			Name:        "agentbox_sprint_start",
			Description: "Start an autonomous sprint (async). Returns a session ID immediately that can be polled with agentbox_sprint_status.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_dir": map[string]interface{}{
						"type":        "string",
						"description": "Path to the project directory",
					},
					"repo_url": map[string]interface{}{
						"type":        "string",
						"description": "Git repository URL to clone",
					},
					"prd_file": map[string]interface{}{
						"type":        "string",
						"description": "PRD file name (default: prd.json)",
					},
					"agent": map[string]interface{}{
						"type":        "string",
						"description": "Agent to use (default: claude)",
					},
					"sprint_size": map[string]interface{}{
						"type":        "integer",
						"description": "Number of tasks per sprint (default: 5)",
					},
					"max_sprints": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of sprints (default: 20)",
					},
				},
			},
		},
		{
			Name:        "agentbox_status",
			Description: "Check session/task progress. Returns session status, PRD completion percentage, and task counts.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"session_id": map[string]interface{}{
						"type":        "string",
						"description": "Session ID to check (from ralph_start or sprint_start)",
					},
					"project_dir": map[string]interface{}{
						"type":        "string",
						"description": "Project directory to find the store database",
					},
				},
			},
		},
		{
			Name:        "agentbox_journal",
			Description: "Read dev diary entries for a session. Returns formatted journal entries with reflections, confidence, and difficulty scores.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"session_id": map[string]interface{}{
						"type":        "string",
						"description": "Session ID to read journal entries for",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of entries to return",
					},
					"project_dir": map[string]interface{}{
						"type":        "string",
						"description": "Project directory containing the store database (default: current directory)",
					},
				},
				"required": []string{"session_id"},
			},
		},
		{
			Name:        "agentbox_task_list",
			Description: "List tasks from a PRD with their status. Returns an array of tasks with id, title, status, and description.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"prd_file": map[string]interface{}{
						"type":        "string",
						"description": "PRD file name relative to project_dir (default: prd.json)",
					},
					"project_dir": map[string]interface{}{
						"type":        "string",
						"description": "Project directory containing the PRD file (default: current directory)",
					},
				},
			},
		},
		{
			Name:        "agentbox_sprint_status",
			Description: "Monitor an active sprint. Returns session_id, status, and task counts (total and completed).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"session_id": map[string]interface{}{
						"type":        "string",
						"description": "Session ID of the sprint to monitor",
					},
					"project_dir": map[string]interface{}{
						"type":        "string",
						"description": "Project directory containing the store database (default: current directory)",
					},
				},
				"required": []string{"session_id"},
			},
		},
	}
}

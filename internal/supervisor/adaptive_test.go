package supervisor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swamp-dev/agentbox/internal/retro"
	"github.com/swamp-dev/agentbox/internal/store"
	"github.com/swamp-dev/agentbox/internal/taskdb"
)

// mockCommandExecutor records calls and returns configured responses.
type mockCommandExecutor struct {
	calls  []mockCall
	output string
	err    error
}

type mockCall struct {
	Dir  string
	Name string
	Args []string
}

func (m *mockCommandExecutor) Execute(_ context.Context, dir string, name string, args ...string) (string, error) {
	m.calls = append(m.calls, mockCall{Dir: dir, Name: name, Args: args})
	return m.output, m.err
}

func TestApply_SimpleActions(t *testing.T) {
	tests := []struct {
		name          string
		action        retro.RecommendationType
		description   string
		wantCount     int
		wantSubstring string
	}{
		{
			name:          "reorder tasks without taskDB",
			action:        retro.RecReorderTasks,
			description:   "reorder for better flow",
			wantCount:     1,
			wantSubstring: "reorder tasks",
		},
		{
			name:          "switch agent",
			action:        retro.RecSwitchAgent,
			description:   "try fallback agent",
			wantCount:     1,
			wantSubstring: "switch agent",
		},
		{
			name:          "unknown action",
			action:        retro.RecommendationType("unknown_action"),
			description:   "something weird",
			wantCount:     0,
			wantSubstring: "",
		},
		{
			name:          "escalate",
			action:        retro.RecEscalate,
			description:   "need human help",
			wantCount:     1,
			wantSubstring: "Escalation",
		},
		{
			name:          "rollback",
			action:        retro.RecRollback,
			description:   "quality degrading",
			wantCount:     1,
			wantSubstring: "rollback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openTestStore(t)
			sessionID, _ := s.CreateSession("", "main", "")
			logger := testLogger()
			ac := NewAdaptiveController(s, sessionID, nil, logger)

			recs := []retro.Recommendation{
				{Action: tt.action, Description: tt.description},
			}
			actions := ac.Apply(recs)
			if len(actions) != tt.wantCount {
				t.Errorf("expected %d action(s), got %d: %v", tt.wantCount, len(actions), actions)
			}
			if tt.wantSubstring != "" && len(actions) > 0 && !strings.Contains(actions[0], tt.wantSubstring) {
				t.Errorf("expected action containing %q, got %q", tt.wantSubstring, actions[0])
			}
		})
	}
}

func TestApply_StoreActions(t *testing.T) {
	tests := []struct {
		name       string
		action     retro.RecommendationType
		taskID     string
		desc       string
		wantStatus string
	}{
		{
			name:       "defer task",
			action:     retro.RecDeferTask,
			taskID:     "t-1",
			desc:       "Too many failures",
			wantStatus: "deferred",
		},
		{
			name:       "skip task",
			action:     retro.RecSkipTask,
			taskID:     "t-1",
			desc:       "Not relevant",
			wantStatus: "deferred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openTestStore(t)
			sessionID, _ := s.CreateSession("", "main", "")
			if err := s.InsertTask(&store.Task{
				ID: tt.taskID, SessionID: sessionID, Title: "Test task", Status: "pending", MaxAttempts: 3,
			}); err != nil {
				t.Fatalf("InsertTask: %v", err)
			}

			logger := testLogger()
			ac := NewAdaptiveController(s, sessionID, nil, logger)

			recs := []retro.Recommendation{
				{Action: tt.action, TaskID: tt.taskID, Description: tt.desc},
			}
			actions := ac.Apply(recs)
			if len(actions) != 1 {
				t.Errorf("expected 1 action, got %d", len(actions))
			}

			task, _ := s.GetTask(tt.taskID)
			if task.Status != tt.wantStatus {
				t.Errorf("expected status %s, got %s", tt.wantStatus, task.Status)
			}
		})
	}
}

func TestApply_UpdateContext(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	if err := s.InsertTask(&store.Task{
		ID: "t-1", SessionID: sessionID, Title: "Test",
		Status: "pending", MaxAttempts: 3, ContextNotes: "original",
	}); err != nil {
		t.Fatalf("InsertTask: %v", err)
	}

	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecUpdateContext, TaskID: "t-1", Description: "compile errors on imports"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}

	task, _ := s.GetTask("t-1")
	if !strings.Contains(task.ContextNotes, "original") {
		t.Error("expected original context notes to be preserved")
	}
	if !strings.Contains(task.ContextNotes, "compile errors on imports") {
		t.Error("expected updated context to include new info")
	}
	if !strings.Contains(task.ContextNotes, "[Retro") {
		t.Error("expected retro timestamp prefix in context notes")
	}
}

func TestWriteEscalation_CreatesFile(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)

	dir := t.TempDir()
	err := ac.WriteEscalation(context.Background(), dir, "first escalation")
	if err != nil {
		t.Fatalf("WriteEscalation: %v", err)
	}

	// Append a second one.
	err = ac.WriteEscalation(context.Background(), dir, "second escalation")
	if err != nil {
		t.Fatalf("WriteEscalation(2): %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".agentbox", "escalations.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "first escalation") {
		t.Error("expected first escalation message")
	}
	if !strings.Contains(content, "second escalation") {
		t.Error("expected second escalation message")
	}
}

func TestApply_ReorderTasks_WithTaskDB(t *testing.T) {
	tests := []struct {
		name          string
		taskID        string
		setupTasks    []taskdb.Task
		wantSubstring string
		wantPriority  int    // expected priority of taskID after apply; -1 to skip check
		wantNextID    string // expected NextTask ID; empty to skip check
	}{
		{
			name:   "deprioritizes failing task",
			taskID: "t-1",
			setupTasks: []taskdb.Task{
				{ID: "t-1", Title: "Failing task", Status: taskdb.StatusPending, Priority: 3},
				{ID: "t-2", Title: "Other task", Status: taskdb.StatusPending, Priority: 5},
			},
			wantSubstring: "Deprioritized",
			wantPriority:  13,
			wantNextID:    "t-2",
		},
		{
			name:          "empty task ID falls back",
			taskID:        "",
			wantSubstring: "Recommendation: reorder tasks",
			wantPriority:  -1,
		},
		{
			name:          "nonexistent task falls back",
			taskID:        "nonexistent",
			wantSubstring: "Recommendation: reorder tasks",
			wantPriority:  -1,
		},
		{
			name:   "caps priority at max",
			taskID: "t-1",
			setupTasks: []taskdb.Task{
				{ID: "t-1", Title: "High priority task", Status: taskdb.StatusPending, Priority: 95},
			},
			wantSubstring: "Deprioritized",
			wantPriority:  taskdb.MaxPriority,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openTestStore(t)
			sessionID, _ := s.CreateSession("", "main", "")
			logger := testLogger()

			tdb := taskdb.New()
			for i := range tt.setupTasks {
				_ = tdb.Add(&tt.setupTasks[i])
			}

			ac := NewAdaptiveController(s, sessionID, tdb, logger)

			recs := []retro.Recommendation{
				{Action: retro.RecReorderTasks, TaskID: tt.taskID, Description: "test reorder"},
			}
			actions := ac.Apply(recs)
			if len(actions) != 1 {
				t.Fatalf("expected 1 action, got %d: %v", len(actions), actions)
			}
			if !strings.Contains(actions[0], tt.wantSubstring) {
				t.Errorf("expected action containing %q, got %q", tt.wantSubstring, actions[0])
			}

			if tt.wantPriority >= 0 {
				task, _ := tdb.Get(tt.taskID)
				if task.Priority != tt.wantPriority {
					t.Errorf("expected priority %d, got %d", tt.wantPriority, task.Priority)
				}
			}

			if tt.wantNextID != "" {
				next := tdb.NextTask()
				if next == nil || next.ID != tt.wantNextID {
					nextID := "<nil>"
					if next != nil {
						nextID = next.ID
					}
					t.Errorf("expected next task %s, got %s", tt.wantNextID, nextID)
				}
			}
		})
	}
}

func TestApply_SplitTask_WithTaskDB(t *testing.T) {
	tests := []struct {
		name          string
		taskID        string
		setupTask     *taskdb.Task // nil means no setup
		wantSubstring string
		wantDeferred  bool
		wantSubtasks  bool
	}{
		{
			name:   "creates subtasks from parent",
			taskID: "t-1",
			setupTask: &taskdb.Task{
				ID: "t-1", Title: "Big complex task", Description: "Do everything",
				Status: taskdb.StatusPending, Priority: 1, MaxAttempts: 3,
			},
			wantSubstring: "Split task t-1",
			wantDeferred:  true,
			wantSubtasks:  true,
		},
		{
			name:          "empty task ID falls back",
			taskID:        "",
			wantSubstring: "Recommendation: split task",
		},
		{
			name:          "nonexistent task falls back",
			taskID:        "nonexistent",
			wantSubstring: "Recommendation: split task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openTestStore(t)
			sessionID, _ := s.CreateSession("", "main", "")
			logger := testLogger()

			tdb := taskdb.New()
			if tt.setupTask != nil {
				_ = tdb.Add(tt.setupTask)
			}

			ac := NewAdaptiveController(s, sessionID, tdb, logger)

			recs := []retro.Recommendation{
				{Action: retro.RecSplitTask, TaskID: tt.taskID, Description: "test split"},
			}
			actions := ac.Apply(recs)
			if len(actions) != 1 {
				t.Fatalf("expected 1 action, got %d: %v", len(actions), actions)
			}
			if !strings.Contains(actions[0], tt.wantSubstring) {
				t.Errorf("expected action containing %q, got %q", tt.wantSubstring, actions[0])
			}

			if tt.wantDeferred {
				parent, _ := tdb.Get(tt.taskID)
				if parent.Status != taskdb.StatusDeferred {
					t.Errorf("expected parent deferred, got %s", parent.Status)
				}
			}

			if tt.wantSubtasks {
				part1, ok1 := tdb.Get(tt.taskID + "-part1")
				part2, ok2 := tdb.Get(tt.taskID + "-part2")
				if !ok1 || !ok2 {
					t.Fatal("expected both subtasks to exist")
				}
				if part1.ParentID != tt.taskID || part2.ParentID != tt.taskID {
					t.Error("expected subtasks to reference parent")
				}
			}
		})
	}
}

func TestApply_NilTaskDB_FallsBack(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()

	// nil taskDB — should produce recommendation-only actions.
	ac := NewAdaptiveController(s, sessionID, nil, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecReorderTasks, TaskID: "t-1", Description: "reorder"},
		{Action: retro.RecSplitTask, TaskID: "t-1", Description: "split"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	if !strings.Contains(actions[0], "Recommendation: reorder tasks") {
		t.Errorf("expected reorder recommendation, got %q", actions[0])
	}
	if !strings.Contains(actions[1], "Recommendation: split task") {
		t.Errorf("expected split recommendation, got %q", actions[1])
	}
}

func TestWriteEscalation_FileMethod(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)
	ac.SetEscalationMethod("file")

	dir := t.TempDir()
	if err := ac.WriteEscalation(context.Background(), dir, "file escalation test"); err != nil {
		t.Fatalf("WriteEscalation: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".agentbox", "escalations.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "file escalation test") {
		t.Error("expected escalation message in file")
	}
}

func TestWriteEscalation_NoneMethod(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)
	ac.SetEscalationMethod("none")

	dir := t.TempDir()
	if err := ac.WriteEscalation(context.Background(), dir, "none escalation test"); err != nil {
		t.Fatalf("WriteEscalation: %v", err)
	}

	// No file should be created.
	_, err := os.Stat(filepath.Join(dir, ".agentbox", "escalations.md"))
	if err == nil {
		t.Error("expected no escalation file for 'none' method, but file exists")
	}
}

func TestWriteEscalation_GitHubIssueMethod(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)
	ac.SetEscalationMethod("github_issue")

	mock := &mockCommandExecutor{output: "https://github.com/org/repo/issues/42\n"}
	ac.SetCommandExecutor(mock)

	dir := t.TempDir()
	if err := ac.WriteEscalation(context.Background(), dir, "task T-1 failed 3 times"); err != nil {
		t.Fatalf("WriteEscalation: %v", err)
	}

	// Verify gh was called correctly.
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(mock.calls))
	}
	call := mock.calls[0]
	if call.Name != "gh" {
		t.Errorf("expected command 'gh', got %q", call.Name)
	}
	if call.Dir != dir {
		t.Errorf("expected dir %q, got %q", dir, call.Dir)
	}

	// Check args contain expected values.
	args := strings.Join(call.Args, " ")
	if !strings.Contains(args, "issue") || !strings.Contains(args, "create") {
		t.Errorf("expected 'issue create' in args, got %q", args)
	}
	if !strings.Contains(args, "task T-1 failed 3 times") {
		t.Errorf("expected escalation message in title args, got %q", args)
	}

	// No file should be created.
	_, err := os.Stat(filepath.Join(dir, ".agentbox", "escalations.md"))
	if err == nil {
		t.Error("expected no local file for github_issue method")
	}
}

func TestWriteEscalation_GitHubIssueMethod_Error(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)
	ac.SetEscalationMethod("github_issue")

	mock := &mockCommandExecutor{err: fmt.Errorf("gh: not authenticated")}
	ac.SetCommandExecutor(mock)

	err := ac.WriteEscalation(context.Background(), t.TempDir(), "failing escalation")
	if err == nil {
		t.Fatal("expected error when gh fails")
	}
	if !strings.Contains(err.Error(), "creating GitHub issue") {
		t.Errorf("expected wrapped error, got %q", err.Error())
	}
}

func TestWriteEscalation_DefaultMethodIsFile(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)
	// Don't set escalation method — should default to "file".

	dir := t.TempDir()
	if err := ac.WriteEscalation(context.Background(), dir, "default method test"); err != nil {
		t.Fatalf("WriteEscalation: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".agentbox", "escalations.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "default method test") {
		t.Error("expected escalation message in file for default method")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world this is long", 10, "hello w..."},
		{"utf8 multibyte", "こんにちは世界です", 6, "こんに..."},
		{"utf8 short enough", "日本語", 5, "日本語"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

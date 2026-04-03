package supervisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swamp-dev/agentbox/internal/retro"
	"github.com/swamp-dev/agentbox/internal/store"
	"github.com/swamp-dev/agentbox/internal/taskdb"
)

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
	err := ac.WriteEscalation(dir, "first escalation")
	if err != nil {
		t.Fatalf("WriteEscalation: %v", err)
	}

	// Append a second one.
	err = ac.WriteEscalation(dir, "second escalation")
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

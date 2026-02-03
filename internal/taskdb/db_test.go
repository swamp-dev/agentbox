package taskdb

import (
	"path/filepath"
	"testing"
)

func TestAddAndGet(t *testing.T) {
	db := New()
	task := &Task{ID: "t-1", Title: "First task", Status: StatusPending}
	if err := db.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, ok := db.Get("t-1")
	if !ok {
		t.Fatal("expected task to exist")
	}
	if got.Title != "First task" {
		t.Errorf("expected 'First task', got %q", got.Title)
	}
	if got.MaxAttempts != 3 {
		t.Errorf("expected default MaxAttempts 3, got %d", got.MaxAttempts)
	}
}

func TestAddDuplicate(t *testing.T) {
	db := New()
	db.Add(&Task{ID: "t-1", Title: "First", Status: StatusPending})
	err := db.Add(&Task{ID: "t-1", Title: "Dup", Status: StatusPending})
	if err == nil {
		t.Error("expected error for duplicate task ID")
	}
}

func TestNextTask_Priority(t *testing.T) {
	db := New()
	db.Add(&Task{ID: "t-low", Title: "Low priority", Status: StatusPending, Priority: 10})
	db.Add(&Task{ID: "t-high", Title: "High priority", Status: StatusPending, Priority: 1})
	db.Add(&Task{ID: "t-mid", Title: "Mid priority", Status: StatusPending, Priority: 5})

	next := db.NextTask()
	if next == nil || next.ID != "t-high" {
		t.Errorf("expected t-high, got %v", next)
	}
}

func TestNextTask_Dependencies(t *testing.T) {
	db := New()
	db.Add(&Task{ID: "t-1", Title: "First", Status: StatusPending, Priority: 1})
	db.Add(&Task{ID: "t-2", Title: "Second", Status: StatusPending, Priority: 1, DependsOn: []string{"t-1"}})
	db.Add(&Task{ID: "t-3", Title: "Third", Status: StatusPending, Priority: 1, DependsOn: []string{"t-2"}})

	// Only t-1 should be available.
	next := db.NextTask()
	if next == nil || next.ID != "t-1" {
		t.Errorf("expected t-1, got %v", next)
	}

	// Complete t-1, now t-2 should be next.
	db.Tasks["t-1"].Status = StatusCompleted
	next = db.NextTask()
	if next == nil || next.ID != "t-2" {
		t.Errorf("expected t-2, got %v", next)
	}

	// Complete t-2, now t-3.
	db.Tasks["t-2"].Status = StatusCompleted
	next = db.NextTask()
	if next == nil || next.ID != "t-3" {
		t.Errorf("expected t-3, got %v", next)
	}
}

func TestNextTask_SkipsExhausted(t *testing.T) {
	db := New()
	task := &Task{
		ID: "t-1", Title: "Flaky", Status: StatusPending, MaxAttempts: 2,
		Attempts: []Attempt{
			{Number: 1, Success: false},
			{Number: 2, Success: false},
		},
	}
	db.Add(task)

	next := db.NextTask()
	if next != nil {
		t.Errorf("expected nil (exhausted), got %v", next.ID)
	}
}

func TestDetectCycles_NoCycle(t *testing.T) {
	db := New()
	db.Add(&Task{ID: "a", Title: "A", Status: StatusPending})
	db.Add(&Task{ID: "b", Title: "B", Status: StatusPending, DependsOn: []string{"a"}})
	db.Add(&Task{ID: "c", Title: "C", Status: StatusPending, DependsOn: []string{"b"}})

	cycles := db.DetectCycles()
	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %v", cycles)
	}
}

func TestDetectCycles_WithCycle(t *testing.T) {
	db := New()
	db.Tasks["a"] = &Task{ID: "a", Title: "A", Status: StatusPending, DependsOn: []string{"c"}}
	db.Tasks["b"] = &Task{ID: "b", Title: "B", Status: StatusPending, DependsOn: []string{"a"}}
	db.Tasks["c"] = &Task{ID: "c", Title: "C", Status: StatusPending, DependsOn: []string{"b"}}

	cycles := db.DetectCycles()
	if len(cycles) == 0 {
		t.Error("expected cycle to be detected")
	}
}

func TestAddDependency_PreventsCycle(t *testing.T) {
	db := New()
	db.Add(&Task{ID: "a", Title: "A", Status: StatusPending})
	db.Add(&Task{ID: "b", Title: "B", Status: StatusPending, DependsOn: []string{"a"}})

	err := db.AddDependency("a", "b")
	if err == nil {
		t.Error("expected error for cycle-creating dependency")
	}
}

func TestSplitTask(t *testing.T) {
	db := New()
	db.Add(&Task{ID: "big", Title: "Big task", Status: StatusPending, Priority: 1})
	db.Add(&Task{ID: "after", Title: "After big", Status: StatusPending, DependsOn: []string{"big"}})

	subtasks := []*Task{
		{ID: "big-1", Title: "Part 1", Status: StatusPending, Priority: 1},
		{ID: "big-2", Title: "Part 2", Status: StatusPending, Priority: 2},
	}

	if err := db.SplitTask("big", subtasks); err != nil {
		t.Fatalf("SplitTask: %v", err)
	}

	// Parent should be deferred.
	parent, _ := db.Get("big")
	if parent.Status != StatusDeferred {
		t.Errorf("expected parent deferred, got %s", parent.Status)
	}

	// "after" should now depend on "big-2" (last subtask).
	after, _ := db.Get("after")
	found := false
	for _, dep := range after.DependsOn {
		if dep == "big-2" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'after' to depend on 'big-2'")
	}
}

func TestMergeTasks(t *testing.T) {
	db := New()
	db.Add(&Task{ID: "a", Title: "A", Status: StatusPending, DependsOn: []string{}})
	db.Add(&Task{ID: "b", Title: "B", Status: StatusPending, DependsOn: []string{"a"}})
	db.Add(&Task{ID: "c", Title: "C", Status: StatusPending, DependsOn: []string{"b"}})

	merged := &Task{ID: "ab", Title: "A+B", Status: StatusPending}
	if err := db.MergeTasks(merged, []string{"a", "b"}); err != nil {
		t.Fatalf("MergeTasks: %v", err)
	}

	if _, ok := db.Get("a"); ok {
		t.Error("expected 'a' to be removed")
	}
	if _, ok := db.Get("b"); ok {
		t.Error("expected 'b' to be removed")
	}

	// c should now depend on ab.
	c, _ := db.Get("c")
	found := false
	for _, dep := range c.DependsOn {
		if dep == "ab" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'c' to depend on 'ab'")
	}
}

func TestIsComplete(t *testing.T) {
	db := New()
	db.Add(&Task{ID: "t-1", Title: "First", Status: StatusCompleted})
	db.Add(&Task{ID: "t-2", Title: "Second", Status: StatusCompleted})

	if !db.IsComplete() {
		t.Error("expected complete")
	}

	db.Add(&Task{ID: "t-3", Title: "Third", Status: StatusPending})
	if db.IsComplete() {
		t.Error("expected not complete")
	}
}

func TestStats(t *testing.T) {
	db := New()
	db.Add(&Task{ID: "t-1", Title: "Done", Status: StatusCompleted})
	db.Add(&Task{ID: "t-2", Title: "Pending", Status: StatusPending})
	db.Add(&Task{ID: "t-3", Title: "Failed", Status: StatusFailed})

	total, completed, pending, failed, deferred := db.Stats()
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if completed != 1 {
		t.Errorf("expected 1 completed, got %d", completed)
	}
	if pending != 1 {
		t.Errorf("expected 1 pending, got %d", pending)
	}
	if failed != 1 {
		t.Errorf("expected 1 failed, got %d", failed)
	}
	if deferred != 0 {
		t.Errorf("expected 0 deferred, got %d", deferred)
	}
}

func TestSaveAndLoad(t *testing.T) {
	db := New()
	db.Add(&Task{ID: "t-1", Title: "First", Status: StatusPending, Priority: 1})
	db.Add(&Task{ID: "t-2", Title: "Second", Status: StatusCompleted, Priority: 2, DependsOn: []string{"t-1"}})

	path := filepath.Join(t.TempDir(), "tasks.json")
	if err := db.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded.Tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(loaded.Tasks))
	}
	if loaded.Tasks["t-2"].Status != StatusCompleted {
		t.Errorf("expected t-2 completed, got %s", loaded.Tasks["t-2"].Status)
	}
}

func TestTaskFailureHistory(t *testing.T) {
	task := &Task{
		ID: "t-1", Title: "Test", Status: StatusPending,
		Attempts: []Attempt{
			{Number: 1, Success: false, ErrorMsg: "compile error"},
			{Number: 2, Success: false, ErrorMsg: "test failure"},
			{Number: 3, Success: true},
		},
	}

	failures := task.FailureHistory()
	if len(failures) != 2 {
		t.Errorf("expected 2 failures, got %d", len(failures))
	}
}

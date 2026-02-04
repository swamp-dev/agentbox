package taskdb

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// DB is an in-memory task database with DAG dependency tracking.
// All mutating operations are protected by a mutex for concurrent safety.
type DB struct {
	mu    sync.Mutex
	Tasks map[string]*Task `json:"tasks"`
}

// New creates a new empty task database.
func New() *DB {
	return &DB{Tasks: make(map[string]*Task)}
}

// Add inserts a task into the database.
func (db *DB) Add(task *Task) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.addLocked(task)
}

// addLocked inserts a task without acquiring the mutex (caller must hold lock).
func (db *DB) addLocked(task *Task) error {
	if _, exists := db.Tasks[task.ID]; exists {
		return fmt.Errorf("task %s already exists", task.ID)
	}
	if task.MaxAttempts == 0 {
		task.MaxAttempts = 3
	}
	if task.Complexity == 0 {
		task.Complexity = 3
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	db.Tasks[task.ID] = task
	return nil
}

// Get returns a task by ID.
func (db *DB) Get(id string) (*Task, bool) {
	t, ok := db.Tasks[id]
	return t, ok
}

// AddDependency adds a dependency edge and checks for cycles.
func (db *DB) AddDependency(taskID, dependsOn string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	task, ok := db.Tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	if _, ok := db.Tasks[dependsOn]; !ok {
		return fmt.Errorf("dependency %s not found", dependsOn)
	}

	// Check if dependency already exists.
	for _, dep := range task.DependsOn {
		if dep == dependsOn {
			return nil
		}
	}

	// Check for cycles before mutation by simulating the addition.
	task.DependsOn = append(task.DependsOn, dependsOn)
	if db.wouldCreateCycle(taskID, dependsOn) {
		task.DependsOn = task.DependsOn[:len(task.DependsOn)-1]
		return fmt.Errorf("adding dependency %s -> %s would create a cycle", taskID, dependsOn)
	}

	return nil
}

// wouldCreateCycle checks if dependsOn can reach taskID via the dependency graph (cycle detection).
func (db *DB) wouldCreateCycle(taskID, from string) bool {
	visited := make(map[string]bool)
	var dfs func(id string) bool
	dfs = func(id string) bool {
		if id == taskID {
			return true
		}
		if visited[id] {
			return false
		}
		visited[id] = true
		t, ok := db.Tasks[id]
		if !ok {
			return false
		}
		for _, dep := range t.DependsOn {
			if dfs(dep) {
				return true
			}
		}
		return false
	}
	return dfs(from)
}

// DetectCycles returns all cycles in the dependency graph using DFS.
func (db *DB) DetectCycles() [][]string {
	var cycles [][]string
	visited := make(map[string]bool)
	inStack := make(map[string]bool)
	path := make([]string, 0)

	var dfs func(id string)
	dfs = func(id string) {
		visited[id] = true
		inStack[id] = true
		path = append(path, id)

		task, ok := db.Tasks[id]
		if !ok {
			path = path[:len(path)-1]
			inStack[id] = false
			return
		}

		for _, dep := range task.DependsOn {
			if !visited[dep] {
				dfs(dep)
			} else if inStack[dep] {
				// Found a cycle. Extract it.
				cycle := []string{dep}
				for i := len(path) - 1; i >= 0; i-- {
					cycle = append(cycle, path[i])
					if path[i] == dep {
						break
					}
				}
				cycles = append(cycles, cycle)
			}
		}

		path = path[:len(path)-1]
		inStack[id] = false
	}

	for id := range db.Tasks {
		if !visited[id] {
			dfs(id)
		}
	}

	return cycles
}

// NextTask returns the highest-priority unblocked task that hasn't exhausted attempts.
func (db *DB) NextTask() *Task {
	db.mu.Lock()
	defer db.mu.Unlock()

	completedIDs := make(map[string]bool)
	for _, t := range db.Tasks {
		if t.Status == StatusCompleted {
			completedIDs[t.ID] = true
		}
	}

	var candidates []*Task
	for _, t := range db.Tasks {
		if t.Status != StatusPending && t.Status != StatusInProgress {
			continue
		}
		if t.HasExhaustedAttempts() {
			continue
		}

		blocked := false
		for _, dep := range t.DependsOn {
			if !completedIDs[dep] {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}

		candidates = append(candidates, t)
	}

	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority < candidates[j].Priority
		}
		return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
	})

	return candidates[0]
}

// SplitTask decomposes a task into subtasks, marking the parent as deferred.
func (db *DB) SplitTask(parentID string, subtasks []*Task) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	parent, ok := db.Tasks[parentID]
	if !ok {
		return fmt.Errorf("task %s not found", parentID)
	}

	for _, sub := range subtasks {
		sub.ParentID = parentID
		sub.DependsOn = append(sub.DependsOn, parent.DependsOn...)
		if err := db.addLocked(sub); err != nil {
			return fmt.Errorf("adding subtask %s: %w", sub.ID, err)
		}
	}

	// Update dependent tasks to depend on the last subtask.
	lastSub := subtasks[len(subtasks)-1]
	for _, t := range db.Tasks {
		for i, dep := range t.DependsOn {
			if dep == parentID {
				t.DependsOn[i] = lastSub.ID
			}
		}
	}

	parent.Status = StatusDeferred
	return nil
}

// MergeTasks combines multiple tasks into one.
func (db *DB) MergeTasks(newTask *Task, oldIDs []string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Collect all dependencies from old tasks.
	depSet := make(map[string]bool)
	for _, id := range oldIDs {
		t, ok := db.Tasks[id]
		if !ok {
			return fmt.Errorf("task %s not found", id)
		}
		for _, dep := range t.DependsOn {
			if dep != newTask.ID {
				depSet[dep] = true
			}
		}
	}

	for dep := range depSet {
		found := false
		for _, d := range newTask.DependsOn {
			if d == dep {
				found = true
				break
			}
		}
		if !found {
			newTask.DependsOn = append(newTask.DependsOn, dep)
		}
	}

	// Update references from other tasks and deduplicate.
	oldIDSet := make(map[string]bool, len(oldIDs))
	for _, id := range oldIDs {
		oldIDSet[id] = true
	}
	for _, t := range db.Tasks {
		seen := make(map[string]bool)
		var deduped []string
		for _, dep := range t.DependsOn {
			if oldIDSet[dep] {
				dep = newTask.ID
			}
			if !seen[dep] {
				seen[dep] = true
				deduped = append(deduped, dep)
			}
		}
		t.DependsOn = deduped
	}

	// Remove old tasks.
	for _, id := range oldIDs {
		delete(db.Tasks, id)
	}

	return db.addLocked(newTask)
}

// IsComplete returns true if all non-deferred tasks are completed.
func (db *DB) IsComplete() bool {
	for _, t := range db.Tasks {
		if t.Status != StatusCompleted && t.Status != StatusDeferred {
			return false
		}
	}
	return len(db.Tasks) > 0
}

// Stats returns task count statistics.
func (db *DB) Stats() (total, completed, pending, failed, deferred int) {
	for _, t := range db.Tasks {
		total++
		switch t.Status {
		case StatusCompleted:
			completed++
		case StatusPending, StatusInProgress:
			pending++
		case StatusFailed:
			failed++
		case StatusDeferred:
			deferred++
		}
	}
	return
}

// Save persists the task DB to a JSON file.
func (db *DB) Save(path string) error {
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling task DB: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// Load reads a task DB from a JSON file.
func Load(path string) (*DB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading task DB: %w", err)
	}
	db := &DB{}
	if err := json.Unmarshal(data, db); err != nil {
		return nil, fmt.Errorf("parsing task DB: %w", err)
	}
	if db.Tasks == nil {
		db.Tasks = make(map[string]*Task)
	}
	return db, nil
}

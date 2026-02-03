// Package store provides SQLite-based persistence for agentbox runtime data.
package store

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

const currentSchemaVersion = 1

// Store is the SQLite-backed persistence layer for agentbox.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens or creates a SQLite database at path and runs migrations.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrent read/write performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate applies the schema if not already at the current version.
func (s *Store) migrate() error {
	// Check if schema_version table exists.
	var name string
	err := s.db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='schema_version'",
	).Scan(&name)

	if err == sql.ErrNoRows {
		// Fresh database — apply full schema.
		if _, err := s.db.Exec(schemaSQL); err != nil {
			return fmt.Errorf("applying schema: %w", err)
		}
		_, err = s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", currentSchemaVersion)
		return err
	}
	if err != nil {
		return fmt.Errorf("checking schema version: %w", err)
	}

	var version int
	if err := s.db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	if version < currentSchemaVersion {
		// Future: apply incremental migrations here.
		return fmt.Errorf("schema version %d is older than %d — migration not yet implemented", version, currentSchemaVersion)
	}

	return nil
}

// --- Session management ---

// Session represents a supervisor session.
type Session struct {
	ID         int64     `json:"id"`
	StartedAt  time.Time `json:"started_at"`
	RepoURL    string    `json:"repo_url"`
	BranchName string    `json:"branch_name"`
	Status     string    `json:"status"`
	ConfigJSON string    `json:"config_json,omitempty"`
}

// CreateSession starts a new session and returns its ID.
func (s *Store) CreateSession(repoURL, branchName, configJSON string) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO sessions (repo_url, branch_name, config_json) VALUES (?, ?, ?)",
		repoURL, branchName, configJSON,
	)
	if err != nil {
		return 0, fmt.Errorf("creating session: %w", err)
	}
	return result.LastInsertId()
}

// UpdateSessionStatus sets the session status.
func (s *Store) UpdateSessionStatus(id int64, status string) error {
	_, err := s.db.Exec("UPDATE sessions SET status = ? WHERE id = ?", status, id)
	return err
}

// GetSession returns a session by ID.
func (s *Store) GetSession(id int64) (*Session, error) {
	sess := &Session{}
	err := s.db.QueryRow(
		"SELECT id, started_at, repo_url, branch_name, status, COALESCE(config_json, '') FROM sessions WHERE id = ?", id,
	).Scan(&sess.ID, &sess.StartedAt, &sess.RepoURL, &sess.BranchName, &sess.Status, &sess.ConfigJSON)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session %d not found", id)
	}
	return sess, err
}

// LatestSession returns the most recent session.
func (s *Store) LatestSession() (*Session, error) {
	sess := &Session{}
	err := s.db.QueryRow(
		"SELECT id, started_at, repo_url, branch_name, status, COALESCE(config_json, '') FROM sessions ORDER BY id DESC LIMIT 1",
	).Scan(&sess.ID, &sess.StartedAt, &sess.RepoURL, &sess.BranchName, &sess.Status, &sess.ConfigJSON)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no sessions found")
	}
	return sess, err
}

// --- Task CRUD ---

// Task represents a task in the store.
type Task struct {
	ID                   string    `json:"id"`
	SessionID            int64     `json:"session_id"`
	Title                string    `json:"title"`
	Description          string    `json:"description"`
	Status               string    `json:"status"`
	Priority             int       `json:"priority"`
	Complexity           int       `json:"complexity"`
	ParentID             string    `json:"parent_id,omitempty"`
	MaxAttempts          int       `json:"max_attempts"`
	ContextNotes         string    `json:"context_notes,omitempty"`
	AcceptanceCriteriaJSON string  `json:"acceptance_criteria_json,omitempty"`
	TagsJSON             string    `json:"tags_json,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
}

// InsertTask adds a task to the store.
func (s *Store) InsertTask(t *Task) error {
	_, err := s.db.Exec(
		`INSERT INTO tasks (id, session_id, title, description, status, priority, complexity,
		 parent_id, max_attempts, context_notes, acceptance_criteria_json, tags_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?)`,
		t.ID, t.SessionID, t.Title, t.Description, t.Status, t.Priority, t.Complexity,
		t.ParentID, t.MaxAttempts, t.ContextNotes, t.AcceptanceCriteriaJSON, t.TagsJSON,
	)
	return err
}

// GetTask returns a task by ID.
func (s *Store) GetTask(id string) (*Task, error) {
	t := &Task{}
	var parentID sql.NullString
	var completedAt sql.NullTime
	err := s.db.QueryRow(
		`SELECT id, session_id, title, description, status, priority, complexity,
		 parent_id, max_attempts, COALESCE(context_notes, ''),
		 COALESCE(acceptance_criteria_json, ''), COALESCE(tags_json, ''),
		 created_at, completed_at
		 FROM tasks WHERE id = ?`, id,
	).Scan(&t.ID, &t.SessionID, &t.Title, &t.Description, &t.Status, &t.Priority,
		&t.Complexity, &parentID, &t.MaxAttempts, &t.ContextNotes,
		&t.AcceptanceCriteriaJSON, &t.TagsJSON, &t.CreatedAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	if parentID.Valid {
		t.ParentID = parentID.String
	}
	if completedAt.Valid {
		t.CompletedAt = &completedAt.Time
	}
	return t, nil
}

// UpdateTaskStatus updates a task's status and optionally sets completed_at.
func (s *Store) UpdateTaskStatus(taskID, status string) error {
	if status == "completed" {
		_, err := s.db.Exec(
			"UPDATE tasks SET status = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?",
			status, taskID,
		)
		return err
	}
	_, err := s.db.Exec("UPDATE tasks SET status = ? WHERE id = ?", status, taskID)
	return err
}

// UpdateTaskContextNotes updates the context notes for a task.
func (s *Store) UpdateTaskContextNotes(taskID, contextNotes string) error {
	_, err := s.db.Exec("UPDATE tasks SET context_notes = ? WHERE id = ?", contextNotes, taskID)
	return err
}

// ListTasks returns all tasks for a session.
func (s *Store) ListTasks(sessionID int64) ([]*Task, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, title, description, status, priority, complexity,
		 parent_id, max_attempts, COALESCE(context_notes, ''),
		 COALESCE(acceptance_criteria_json, ''), COALESCE(tags_json, ''),
		 created_at, completed_at
		 FROM tasks WHERE session_id = ? ORDER BY priority ASC, created_at ASC`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t := &Task{}
		var parentID sql.NullString
		var completedAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.SessionID, &t.Title, &t.Description, &t.Status,
			&t.Priority, &t.Complexity, &parentID, &t.MaxAttempts, &t.ContextNotes,
			&t.AcceptanceCriteriaJSON, &t.TagsJSON, &t.CreatedAt, &completedAt); err != nil {
			return nil, err
		}
		if parentID.Valid {
			t.ParentID = parentID.String
		}
		if completedAt.Valid {
			t.CompletedAt = &completedAt.Time
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// AddDependency adds a task dependency.
func (s *Store) AddDependency(taskID, dependsOn string) error {
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO task_dependencies (task_id, depends_on) VALUES (?, ?)",
		taskID, dependsOn,
	)
	return err
}

// GetDependencies returns the task IDs that a given task depends on.
func (s *Store) GetDependencies(taskID string) ([]string, error) {
	rows, err := s.db.Query(
		"SELECT depends_on FROM task_dependencies WHERE task_id = ?", taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []string
	for rows.Next() {
		var dep string
		if err := rows.Scan(&dep); err != nil {
			return nil, err
		}
		deps = append(deps, dep)
	}
	return deps, rows.Err()
}

// GetAllDependencies returns all dependency edges for a session.
func (s *Store) GetAllDependencies(sessionID int64) (map[string][]string, error) {
	rows, err := s.db.Query(
		`SELECT td.task_id, td.depends_on FROM task_dependencies td
		 JOIN tasks t ON td.task_id = t.id WHERE t.session_id = ?`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deps := make(map[string][]string)
	for rows.Next() {
		var taskID, dependsOn string
		if err := rows.Scan(&taskID, &dependsOn); err != nil {
			return nil, err
		}
		deps[taskID] = append(deps[taskID], dependsOn)
	}
	return deps, rows.Err()
}

// NextTask returns the next unblocked, pending task that hasn't exhausted its attempts.
func (s *Store) NextTask(sessionID int64) (*Task, error) {
	// Find pending tasks whose dependencies are all completed,
	// and which haven't hit max attempts.
	rows, err := s.db.Query(`
		SELECT t.id, t.session_id, t.title, t.description, t.status, t.priority,
		       t.complexity, t.parent_id, t.max_attempts,
		       COALESCE(t.context_notes, ''),
		       COALESCE(t.acceptance_criteria_json, ''),
		       COALESCE(t.tags_json, ''),
		       t.created_at, t.completed_at
		FROM tasks t
		WHERE t.session_id = ?
		  AND t.status IN ('pending', 'in_progress')
		  AND NOT EXISTS (
		      SELECT 1 FROM task_dependencies td
		      JOIN tasks dep ON td.depends_on = dep.id
		      WHERE td.task_id = t.id AND dep.status != 'completed'
		  )
		  AND (
		      SELECT COUNT(*) FROM attempts a WHERE a.task_id = t.id
		  ) < t.max_attempts
		ORDER BY t.priority ASC, t.created_at ASC
		LIMIT 1
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}

	t := &Task{}
	var parentID sql.NullString
	var completedAt sql.NullTime
	if err := rows.Scan(&t.ID, &t.SessionID, &t.Title, &t.Description, &t.Status,
		&t.Priority, &t.Complexity, &parentID, &t.MaxAttempts, &t.ContextNotes,
		&t.AcceptanceCriteriaJSON, &t.TagsJSON, &t.CreatedAt, &completedAt); err != nil {
		return nil, err
	}
	if parentID.Valid {
		t.ParentID = parentID.String
	}
	if completedAt.Valid {
		t.CompletedAt = &completedAt.Time
	}
	return t, rows.Err()
}

// TaskAttemptCount returns how many attempts have been made for a task.
func (s *Store) TaskAttemptCount(taskID string) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM attempts WHERE task_id = ?", taskID).Scan(&count)
	return count, err
}

// --- Attempts ---

// Attempt represents a single execution attempt for a task.
type Attempt struct {
	ID          int64      `json:"id"`
	TaskID      string     `json:"task_id"`
	SessionID   int64      `json:"session_id"`
	Number      int        `json:"number"`
	AgentName   string     `json:"agent_name"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Success     *bool      `json:"success,omitempty"`
	ErrorMsg    string     `json:"error_msg,omitempty"`
	GitCommit   string     `json:"git_commit,omitempty"`
	GitRollback string     `json:"git_rollback,omitempty"`
	TokensUsed  int        `json:"tokens_used"`
	DurationMs  int        `json:"duration_ms"`
	Transcript  string     `json:"transcript,omitempty"`
}

// RecordAttempt inserts an attempt and returns its ID.
func (s *Store) RecordAttempt(a *Attempt) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO attempts (task_id, session_id, number, agent_name, started_at,
		 completed_at, success, error_msg, git_commit, git_rollback, tokens_used, duration_ms, transcript)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.TaskID, a.SessionID, a.Number, a.AgentName, a.StartedAt,
		a.CompletedAt, a.Success, a.ErrorMsg, a.GitCommit, a.GitRollback,
		a.TokensUsed, a.DurationMs, a.Transcript,
	)
	if err != nil {
		return 0, fmt.Errorf("recording attempt: %w", err)
	}
	return result.LastInsertId()
}

// GetAttempts returns all attempts for a task.
func (s *Store) GetAttempts(taskID string) ([]*Attempt, error) {
	rows, err := s.db.Query(
		`SELECT id, task_id, session_id, number, agent_name, started_at,
		 completed_at, success, COALESCE(error_msg, ''), COALESCE(git_commit, ''),
		 COALESCE(git_rollback, ''), tokens_used, duration_ms
		 FROM attempts WHERE task_id = ? ORDER BY number ASC`, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []*Attempt
	for rows.Next() {
		a := &Attempt{}
		var completedAt sql.NullTime
		var success sql.NullBool
		if err := rows.Scan(&a.ID, &a.TaskID, &a.SessionID, &a.Number, &a.AgentName,
			&a.StartedAt, &completedAt, &success, &a.ErrorMsg, &a.GitCommit,
			&a.GitRollback, &a.TokensUsed, &a.DurationMs); err != nil {
			return nil, err
		}
		if completedAt.Valid {
			a.CompletedAt = &completedAt.Time
		}
		if success.Valid {
			a.Success = &success.Bool
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

// SaveTranscript stores the full agent transcript for an attempt.
func (s *Store) SaveTranscript(attemptID int64, transcript string) error {
	_, err := s.db.Exec("UPDATE attempts SET transcript = ? WHERE id = ?", transcript, attemptID)
	return err
}

// GetTranscript retrieves the transcript for an attempt.
func (s *Store) GetTranscript(attemptID int64) (string, error) {
	var transcript sql.NullString
	err := s.db.QueryRow("SELECT transcript FROM attempts WHERE id = ?", attemptID).Scan(&transcript)
	if err != nil {
		return "", err
	}
	if transcript.Valid {
		return transcript.String, nil
	}
	return "", nil
}

// --- Quality Snapshots ---

// QualitySnapshot represents a point-in-time quality measurement.
type QualitySnapshot struct {
	ID              int64     `json:"id"`
	SessionID       int64     `json:"session_id"`
	AttemptID       *int64    `json:"attempt_id,omitempty"`
	Iteration       int       `json:"iteration"`
	TaskID          string    `json:"task_id,omitempty"`
	OverallPass     bool      `json:"overall_pass"`
	ChecksJSON      string    `json:"checks_json,omitempty"`
	TestTotal       int       `json:"test_total"`
	TestPassed      int       `json:"test_passed"`
	TestFailed      int       `json:"test_failed"`
	TestSkipped     int       `json:"test_skipped"`
	FailedTestsJSON string    `json:"failed_tests_json,omitempty"`
	Timestamp       time.Time `json:"timestamp"`
}

// RecordQuality inserts a quality snapshot.
func (s *Store) RecordQuality(q *QualitySnapshot) error {
	_, err := s.db.Exec(
		`INSERT INTO quality_snapshots (session_id, attempt_id, iteration, task_id,
		 overall_pass, checks_json, test_total, test_passed, test_failed, test_skipped,
		 failed_tests_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		q.SessionID, q.AttemptID, q.Iteration, q.TaskID,
		q.OverallPass, q.ChecksJSON, q.TestTotal, q.TestPassed, q.TestFailed,
		q.TestSkipped, q.FailedTestsJSON,
	)
	return err
}

// TestPassRate computes the test pass rate over the last N snapshots.
func (s *Store) TestPassRate(sessionID int64, lastN int) (float64, error) {
	var totalTests, passedTests int
	err := s.db.QueryRow(`
		SELECT COALESCE(SUM(test_total), 0), COALESCE(SUM(test_passed), 0)
		FROM (
			SELECT test_total, test_passed FROM quality_snapshots
			WHERE session_id = ? ORDER BY id DESC LIMIT ?
		)`, sessionID, lastN,
	).Scan(&totalTests, &passedTests)
	if err != nil {
		return 0, err
	}
	if totalTests == 0 {
		return 0, nil
	}
	return float64(passedTests) / float64(totalTests), nil
}

// FailingTestTrend returns a map of test names to failure counts over last N snapshots.
func (s *Store) FailingTestTrend(sessionID int64, lastN int) (map[string]int, error) {
	rows, err := s.db.Query(`
		SELECT failed_tests_json FROM quality_snapshots
		WHERE session_id = ? AND failed_tests_json IS NOT NULL AND failed_tests_json != ''
		ORDER BY id DESC LIMIT ?`, sessionID, lastN,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var tests []string
		if err := json.Unmarshal([]byte(raw), &tests); err != nil {
			continue
		}
		for _, t := range tests {
			counts[t]++
		}
	}
	return counts, rows.Err()
}

// QualityTrend returns "improving", "stable", or "degrading" based on recent snapshots.
func (s *Store) QualityTrend(sessionID int64, lastN int) (string, error) {
	rows, err := s.db.Query(`
		SELECT overall_pass FROM quality_snapshots
		WHERE session_id = ? ORDER BY id DESC LIMIT ?`, sessionID, lastN,
	)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var results []bool
	for rows.Next() {
		var pass bool
		if err := rows.Scan(&pass); err != nil {
			return "", err
		}
		results = append(results, pass)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	if len(results) < 2 {
		return "stable", nil
	}

	// Results are newest-first. Split into equal halves for comparison.
	// For odd lengths, the middle element goes to the older half
	// to avoid bias toward the recent half.
	recentCount := len(results) / 2
	olderCount := len(results) - recentCount

	recentPasses := 0
	olderPasses := 0
	for i, r := range results {
		if r {
			if i < recentCount {
				recentPasses++
			} else {
				olderPasses++
			}
		}
	}

	recentRate := float64(recentPasses) / float64(recentCount)
	olderRate := float64(olderPasses) / float64(olderCount)

	if recentRate > olderRate+0.2 {
		return "improving", nil
	}
	if recentRate < olderRate-0.2 {
		return "degrading", nil
	}
	return "stable", nil
}

// --- Resource Usage ---

// ResourceUsage tracks resource consumption for an iteration.
type ResourceUsage struct {
	ID              int64     `json:"id"`
	SessionID       int64     `json:"session_id"`
	AttemptID       *int64    `json:"attempt_id,omitempty"`
	Iteration       int       `json:"iteration"`
	TaskID          string    `json:"task_id,omitempty"`
	AgentName       string    `json:"agent_name,omitempty"`
	ContainerTimeMs int       `json:"container_time_ms"`
	EstimatedTokens int       `json:"estimated_tokens"`
	Timestamp       time.Time `json:"timestamp"`
}

// RecordUsage inserts a resource usage record.
func (s *Store) RecordUsage(u *ResourceUsage) error {
	_, err := s.db.Exec(
		`INSERT INTO resource_usage (session_id, attempt_id, iteration, task_id,
		 agent_name, container_time_ms, estimated_tokens)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.SessionID, u.AttemptID, u.Iteration, u.TaskID,
		u.AgentName, u.ContainerTimeMs, u.EstimatedTokens,
	)
	return err
}

// TotalUsage returns aggregate resource usage for a session.
func (s *Store) TotalUsage(sessionID int64) (*ResourceUsage, error) {
	u := &ResourceUsage{SessionID: sessionID}
	err := s.db.QueryRow(`
		SELECT COALESCE(SUM(container_time_ms), 0), COALESCE(SUM(estimated_tokens), 0),
		       COUNT(*)
		FROM resource_usage WHERE session_id = ?`, sessionID,
	).Scan(&u.ContainerTimeMs, &u.EstimatedTokens, &u.Iteration)
	return u, err
}

// --- Journal Entries ---

// JournalEntry represents a dev diary entry.
type JournalEntry struct {
	ID         int64     `json:"id"`
	SessionID  int64     `json:"session_id"`
	Kind       string    `json:"kind"`
	TaskID     string    `json:"task_id,omitempty"`
	Sprint     int       `json:"sprint,omitempty"`
	Iteration  int       `json:"iteration"`
	Summary    string    `json:"summary"`
	Reflection string    `json:"reflection"`
	Confidence int       `json:"confidence,omitempty"`
	Difficulty int       `json:"difficulty,omitempty"`
	Momentum   int       `json:"momentum,omitempty"`
	DurationMs int       `json:"duration_ms,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// AddJournalEntry inserts a journal entry.
func (s *Store) AddJournalEntry(e *JournalEntry) error {
	_, err := s.db.Exec(
		`INSERT INTO journal_entries (session_id, kind, task_id, sprint, iteration,
		 summary, reflection, confidence, difficulty, momentum, duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.SessionID, e.Kind, e.TaskID, e.Sprint, e.Iteration,
		e.Summary, e.Reflection, e.Confidence, e.Difficulty, e.Momentum, e.DurationMs,
	)
	return err
}

// JournalQuery specifies filters for querying journal entries.
type JournalQuery struct {
	Kind   string
	Sprint int
	Limit  int
}

// JournalEntries returns journal entries for a session, optionally filtered.
func (s *Store) JournalEntries(sessionID int64, opts *JournalQuery) ([]*JournalEntry, error) {
	query := "SELECT id, session_id, kind, COALESCE(task_id, ''), sprint, iteration, summary, reflection, confidence, difficulty, momentum, duration_ms, timestamp FROM journal_entries WHERE session_id = ?"
	args := []interface{}{sessionID}

	if opts != nil {
		if opts.Kind != "" {
			query += " AND kind = ?"
			args = append(args, opts.Kind)
		}
		if opts.Sprint > 0 {
			query += " AND sprint = ?"
			args = append(args, opts.Sprint)
		}
	}

	query += " ORDER BY timestamp ASC"

	if opts != nil && opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*JournalEntry
	for rows.Next() {
		e := &JournalEntry{}
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Kind, &e.TaskID, &e.Sprint,
			&e.Iteration, &e.Summary, &e.Reflection, &e.Confidence, &e.Difficulty,
			&e.Momentum, &e.DurationMs, &e.Timestamp); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ExportJournalMarkdown generates a human-readable markdown diary.
func (s *Store) ExportJournalMarkdown(sessionID int64) (string, error) {
	entries, err := s.JournalEntries(sessionID, nil)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("# Agentbox Dev Diary\n\n")

	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("## Iteration %d — %s\n", e.Iteration, e.Summary))
		sb.WriteString(fmt.Sprintf("**%s | Sprint %d", e.Timestamp.Format("2006-01-02 15:04"), e.Sprint))
		if e.Confidence > 0 {
			sb.WriteString(fmt.Sprintf(" | Confidence: %d/5", e.Confidence))
		}
		if e.Difficulty > 0 {
			sb.WriteString(fmt.Sprintf(" | Difficulty: %d/5", e.Difficulty))
		}
		sb.WriteString("**\n\n")
		sb.WriteString(e.Reflection)
		sb.WriteString("\n\n---\n\n")
	}

	return sb.String(), nil
}

// --- Sprint Reports ---

// SprintReport represents a sprint retrospective.
type SprintReport struct {
	ID                  int64     `json:"id"`
	SessionID           int64     `json:"session_id"`
	SprintNumber        int       `json:"sprint_number"`
	StartIteration      int       `json:"start_iteration"`
	EndIteration        int       `json:"end_iteration"`
	TasksAttempted      int       `json:"tasks_attempted"`
	TasksCompleted      int       `json:"tasks_completed"`
	TasksFailed         int       `json:"tasks_failed"`
	Velocity            float64   `json:"velocity"`
	QualityTrend        string    `json:"quality_trend"`
	TestPassRate        float64   `json:"test_pass_rate"`
	PatternsJSON        string    `json:"patterns_json,omitempty"`
	RecommendationsJSON string    `json:"recommendations_json,omitempty"`
	TotalTokens         int       `json:"total_tokens"`
	DurationMs          int       `json:"duration_ms"`
	Timestamp           time.Time `json:"timestamp"`
}

// SaveSprintReport inserts a sprint report.
func (s *Store) SaveSprintReport(r *SprintReport) error {
	_, err := s.db.Exec(
		`INSERT INTO sprint_reports (session_id, sprint_number, start_iteration, end_iteration,
		 tasks_attempted, tasks_completed, tasks_failed, velocity, quality_trend, test_pass_rate,
		 patterns_json, recommendations_json, total_tokens, duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.SessionID, r.SprintNumber, r.StartIteration, r.EndIteration,
		r.TasksAttempted, r.TasksCompleted, r.TasksFailed, r.Velocity,
		r.QualityTrend, r.TestPassRate, r.PatternsJSON, r.RecommendationsJSON,
		r.TotalTokens, r.DurationMs,
	)
	return err
}

// SprintReports returns all sprint reports for a session.
func (s *Store) SprintReports(sessionID int64) ([]*SprintReport, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, sprint_number, start_iteration, end_iteration,
		 tasks_attempted, tasks_completed, tasks_failed, velocity, COALESCE(quality_trend, ''),
		 test_pass_rate, COALESCE(patterns_json, ''), COALESCE(recommendations_json, ''),
		 total_tokens, duration_ms, timestamp
		 FROM sprint_reports WHERE session_id = ? ORDER BY sprint_number ASC`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []*SprintReport
	for rows.Next() {
		r := &SprintReport{}
		if err := rows.Scan(&r.ID, &r.SessionID, &r.SprintNumber, &r.StartIteration,
			&r.EndIteration, &r.TasksAttempted, &r.TasksCompleted, &r.TasksFailed,
			&r.Velocity, &r.QualityTrend, &r.TestPassRate, &r.PatternsJSON,
			&r.RecommendationsJSON, &r.TotalTokens, &r.DurationMs, &r.Timestamp); err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

// --- Review Results ---

// ReviewResult represents a code review outcome.
type ReviewResult struct {
	ID           int64     `json:"id"`
	SessionID    int64     `json:"session_id"`
	Sprint       int       `json:"sprint"`
	ReviewAgent  string    `json:"review_agent"`
	FindingsJSON string    `json:"findings_json,omitempty"`
	Summary      string    `json:"summary"`
	Approved     bool      `json:"approved"`
	ReviewedAt   time.Time `json:"reviewed_at"`
}

// SaveReviewResult inserts a review result.
func (s *Store) SaveReviewResult(r *ReviewResult) error {
	_, err := s.db.Exec(
		`INSERT INTO review_results (session_id, sprint, review_agent, findings_json,
		 summary, approved) VALUES (?, ?, ?, ?, ?, ?)`,
		r.SessionID, r.Sprint, r.ReviewAgent, r.FindingsJSON, r.Summary, r.Approved,
	)
	return err
}

// --- Dashboard Export ---

// DashboardData holds aggregated stats for display.
type DashboardData struct {
	Session        *Session         `json:"session"`
	TaskStats      *TaskStats       `json:"task_stats"`
	TotalUsage     *ResourceUsage   `json:"total_usage"`
	QualityTrend   string           `json:"quality_trend"`
	TestPassRate   float64          `json:"test_pass_rate"`
	SprintReports  []*SprintReport  `json:"sprint_reports"`
	RecentJournal  []*JournalEntry  `json:"recent_journal"`
}

// TaskStats holds task count aggregations.
type TaskStats struct {
	Total     int `json:"total"`
	Pending   int `json:"pending"`
	InProgress int `json:"in_progress"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Deferred  int `json:"deferred"`
}

// ExportDashboardData gathers all dashboard data for a session.
func (s *Store) ExportDashboardData(sessionID int64) (*DashboardData, error) {
	sess, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// Task stats
	stats := &TaskStats{}
	rows, err := s.db.Query(
		"SELECT status, COUNT(*) FROM tasks WHERE session_id = ? GROUP BY status", sessionID,
	)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.Total += count
		switch status {
		case "pending":
			stats.Pending = count
		case "in_progress":
			stats.InProgress = count
		case "completed":
			stats.Completed = count
		case "failed":
			stats.Failed = count
		case "deferred":
			stats.Deferred = count
		}
	}
	rows.Close()

	usage, err := s.TotalUsage(sessionID)
	if err != nil {
		return nil, err
	}

	trend, err := s.QualityTrend(sessionID, 10)
	if err != nil {
		return nil, err
	}

	passRate, err := s.TestPassRate(sessionID, 10)
	if err != nil {
		return nil, err
	}

	reports, err := s.SprintReports(sessionID)
	if err != nil {
		return nil, err
	}

	journal, err := s.JournalEntries(sessionID, &JournalQuery{Limit: 5})
	if err != nil {
		return nil, err
	}

	return &DashboardData{
		Session:       sess,
		TaskStats:     stats,
		TotalUsage:    usage,
		QualityTrend:  trend,
		TestPassRate:  passRate,
		SprintReports: reports,
		RecentJournal: journal,
	}, nil
}

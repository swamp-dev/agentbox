-- Agentbox SQLite schema v1

CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    repo_url    TEXT,
    branch_name TEXT,
    status      TEXT NOT NULL DEFAULT 'running',
    config_json TEXT
);

CREATE TABLE IF NOT EXISTS tasks (
    id          TEXT PRIMARY KEY,
    session_id  INTEGER NOT NULL REFERENCES sessions(id),
    title       TEXT NOT NULL,
    description TEXT,
    status      TEXT NOT NULL DEFAULT 'pending',
    priority    INTEGER DEFAULT 0,
    complexity  INTEGER DEFAULT 3,
    parent_id   TEXT REFERENCES tasks(id),
    max_attempts INTEGER DEFAULT 3,
    context_notes TEXT,
    acceptance_criteria_json TEXT,
    tags_json   TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);

CREATE TABLE IF NOT EXISTS task_dependencies (
    task_id    TEXT NOT NULL REFERENCES tasks(id),
    depends_on TEXT NOT NULL REFERENCES tasks(id),
    PRIMARY KEY (task_id, depends_on)
);

CREATE TABLE IF NOT EXISTS attempts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     TEXT NOT NULL REFERENCES tasks(id),
    session_id  INTEGER NOT NULL REFERENCES sessions(id),
    number      INTEGER NOT NULL,
    agent_name  TEXT NOT NULL,
    started_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    success     BOOLEAN,
    error_msg   TEXT,
    git_commit  TEXT,
    git_rollback TEXT,
    tokens_used INTEGER DEFAULT 0,
    duration_ms INTEGER DEFAULT 0,
    transcript  TEXT
);

CREATE TABLE IF NOT EXISTS quality_snapshots (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  INTEGER NOT NULL REFERENCES sessions(id),
    attempt_id  INTEGER REFERENCES attempts(id),
    iteration   INTEGER NOT NULL,
    task_id     TEXT,
    overall_pass BOOLEAN,
    checks_json TEXT,
    test_total  INTEGER DEFAULT 0,
    test_passed INTEGER DEFAULT 0,
    test_failed INTEGER DEFAULT 0,
    test_skipped INTEGER DEFAULT 0,
    failed_tests_json TEXT,
    timestamp   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS resource_usage (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id      INTEGER NOT NULL REFERENCES sessions(id),
    attempt_id      INTEGER REFERENCES attempts(id),
    iteration       INTEGER NOT NULL,
    task_id         TEXT,
    agent_name      TEXT,
    container_time_ms INTEGER DEFAULT 0,
    estimated_tokens  INTEGER DEFAULT 0,
    timestamp       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS journal_entries (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  INTEGER NOT NULL REFERENCES sessions(id),
    kind        TEXT NOT NULL,
    task_id     TEXT,
    sprint      INTEGER,
    iteration   INTEGER NOT NULL,
    summary     TEXT NOT NULL,
    reflection  TEXT NOT NULL,
    confidence  INTEGER,
    difficulty  INTEGER,
    momentum    INTEGER,
    duration_ms INTEGER DEFAULT 0,
    timestamp   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sprint_reports (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id      INTEGER NOT NULL REFERENCES sessions(id),
    sprint_number   INTEGER NOT NULL,
    start_iteration INTEGER,
    end_iteration   INTEGER,
    tasks_attempted INTEGER DEFAULT 0,
    tasks_completed INTEGER DEFAULT 0,
    tasks_failed    INTEGER DEFAULT 0,
    velocity        REAL DEFAULT 0,
    quality_trend   TEXT,
    test_pass_rate  REAL DEFAULT 0,
    patterns_json   TEXT,
    recommendations_json TEXT,
    total_tokens    INTEGER DEFAULT 0,
    duration_ms     INTEGER DEFAULT 0,
    timestamp       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS review_results (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id   INTEGER NOT NULL REFERENCES sessions(id),
    sprint       INTEGER,
    review_agent TEXT NOT NULL,
    findings_json TEXT,
    summary      TEXT,
    approved     BOOLEAN,
    reviewed_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_tasks_session ON tasks(session_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_attempts_task ON attempts(task_id);
CREATE INDEX IF NOT EXISTS idx_attempts_session ON attempts(session_id);
CREATE INDEX IF NOT EXISTS idx_quality_session ON quality_snapshots(session_id);
CREATE INDEX IF NOT EXISTS idx_journal_session ON journal_entries(session_id);
CREATE INDEX IF NOT EXISTS idx_journal_kind ON journal_entries(kind);
CREATE INDEX IF NOT EXISTS idx_resource_session ON resource_usage(session_id);
CREATE INDEX IF NOT EXISTS idx_sprint_reports_session ON sprint_reports(session_id);
CREATE INDEX IF NOT EXISTS idx_review_results_session ON review_results(session_id);

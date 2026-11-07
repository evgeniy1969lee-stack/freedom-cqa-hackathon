CREATE TABLE IF NOT EXISTS conversations (
    id          TEXT PRIMARY KEY,
    channel     TEXT NOT NULL,
    language    TEXT NOT NULL,
    audio_url   TEXT,
    source_hash TEXT NOT NULL,
    imported_at TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS turns (
    conversation_id TEXT    NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    turn_id         INTEGER NOT NULL,
    timestamp       TEXT    NOT NULL,
    speaker         TEXT    NOT NULL,
    text            TEXT    NOT NULL,
    PRIMARY KEY (conversation_id, turn_id)
);

CREATE TABLE IF NOT EXISTS checklists (
    version       INTEGER PRIMARY KEY,
    created_at    TEXT NOT NULL,
    note          TEXT NOT NULL DEFAULT '',
    criteria_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS jobs (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id   TEXT    NOT NULL REFERENCES conversations(id),
    input_hash        TEXT    NOT NULL,
    checklist_version INTEGER NOT NULL,
    checklist_json    TEXT    NOT NULL,
    status            TEXT    NOT NULL,
    attempts          INTEGER NOT NULL DEFAULT 0,
    max_attempts      INTEGER NOT NULL,
    error             TEXT    NOT NULL DEFAULT '',
    model             TEXT    NOT NULL DEFAULT '',
    prompt_version    TEXT    NOT NULL DEFAULT '',
    created_at        TEXT    NOT NULL,
    started_at        TEXT,
    finished_at       TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS jobs_active_unique
    ON jobs (conversation_id, input_hash, checklist_version)
    WHERE status IN ('queued', 'running');

CREATE INDEX IF NOT EXISTS jobs_status ON jobs (status, id);

CREATE TABLE IF NOT EXISTS evaluations (
    id                        INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id                    INTEGER NOT NULL UNIQUE REFERENCES jobs(id),
    conversation_id           TEXT    NOT NULL REFERENCES conversations(id),
    input_hash                TEXT    NOT NULL,
    checklist_version         INTEGER NOT NULL,
    checklist_json            TEXT    NOT NULL,
    model                     TEXT    NOT NULL,
    prompt_version            TEXT    NOT NULL,
    total                     REAL,
    w                         REAL    NOT NULL,
    gross_error               INTEGER NOT NULL DEFAULT 0,
    gross_error_category      TEXT    NOT NULL DEFAULT '',
    gross_error_reason        TEXT    NOT NULL DEFAULT '',
    gross_error_evidence_json TEXT    NOT NULL DEFAULT '{}',
    raw_json                  TEXT    NOT NULL,
    created_at                TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS evaluation_items (
    evaluation_id INTEGER NOT NULL REFERENCES evaluations(id) ON DELETE CASCADE,
    position      INTEGER NOT NULL,
    criterion_id  TEXT    NOT NULL,
    answer        TEXT    NOT NULL,
    reason        TEXT    NOT NULL,
    evidence_json TEXT    NOT NULL,
    weight        REAL    NOT NULL,
    applicable    INTEGER NOT NULL,
    q             REAL    NOT NULL,
    points        REAL    NOT NULL,
    PRIMARY KEY (evaluation_id, criterion_id)
);

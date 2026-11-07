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

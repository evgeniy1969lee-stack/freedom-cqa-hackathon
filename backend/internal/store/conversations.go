package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/homepunks/datathong/internal/domain"
)

var ErrNotFound = errors.New("not found")

type UpsertStatus string

const (
	StatusCreated   UpsertStatus = "created"
	StatusUpdated   UpsertStatus = "updated"
	StatusUnchanged UpsertStatus = "unchanged"
)

func (s *Store) UpsertConversation(ctx context.Context, c domain.Conversation) (UpsertStatus, error) {
	status := StatusCreated
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var oldHash string
		var importedAt string
		err := tx.QueryRowContext(ctx, `SELECT source_hash, imported_at FROM conversations WHERE id = ?`, c.ID).Scan(&oldHash, &importedAt)
		switch {
		case err == sql.ErrNoRows:
		case err != nil:
			return err
		case oldHash == c.SourceHash:
			status = StatusUnchanged
			return nil
		default:
			status = StatusUpdated
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if status == StatusCreated {
			importedAt = now
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO conversations (id, channel, language, audio_url, source_hash, imported_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				channel = excluded.channel, language = excluded.language, audio_url = excluded.audio_url,
				source_hash = excluded.source_hash, updated_at = excluded.updated_at`,
			c.ID, c.Channel, c.Language, c.AudioURL, c.SourceHash, importedAt, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM turns WHERE conversation_id = ?`, c.ID); err != nil {
			return err
		}
		for _, t := range c.Turns {
			if _, err := tx.ExecContext(ctx, `INSERT INTO turns (conversation_id, turn_id, timestamp, speaker, text) VALUES (?, ?, ?, ?, ?)`,
				c.ID, t.TurnID, t.Timestamp, t.Speaker, t.Text); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("upsert conversation %s: %w", c.ID, err)
	}
	return status, nil
}

func (s *Store) ListConversations(ctx context.Context) ([]domain.ConversationSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.channel, c.language, c.audio_url, c.source_hash, c.imported_at, c.updated_at,
		       (SELECT COUNT(*) FROM turns t WHERE t.conversation_id = c.id)
		FROM conversations c ORDER BY c.updated_at DESC, c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ConversationSummary{}
	for rows.Next() {
		var cs domain.ConversationSummary
		var imported, updated string
		if err := rows.Scan(&cs.ID, &cs.Channel, &cs.Language, &cs.AudioURL, &cs.SourceHash, &imported, &updated, &cs.TurnCount); err != nil {
			return nil, err
		}
		cs.ImportedAt, cs.UpdatedAt = parseTime(imported), parseTime(updated)
		out = append(out, cs)
	}
	return out, rows.Err()
}

func (s *Store) GetConversation(ctx context.Context, id string) (domain.Conversation, error) {
	var c domain.Conversation
	var imported, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id, channel, language, audio_url, source_hash, imported_at, updated_at FROM conversations WHERE id = ?`, id).
		Scan(&c.ID, &c.Channel, &c.Language, &c.AudioURL, &c.SourceHash, &imported, &updated)
	if err == sql.ErrNoRows {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	c.ImportedAt, c.UpdatedAt = parseTime(imported), parseTime(updated)
	rows, err := s.db.QueryContext(ctx, `SELECT turn_id, timestamp, speaker, text FROM turns WHERE conversation_id = ? ORDER BY turn_id`, id)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	c.Turns = []domain.Turn{}
	for rows.Next() {
		var t domain.Turn
		if err := rows.Scan(&t.TurnID, &t.Timestamp, &t.Speaker, &t.Text); err != nil {
			return c, err
		}
		c.Turns = append(c.Turns, t)
	}
	return c, rows.Err()
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

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
		       (SELECT COUNT(*) FROM turns t WHERE t.conversation_id = c.id),
		       j.id, j.status, e.id, CASE WHEN e.revised_at IS NULL THEN e.total ELSE e.final_total END, e.gross_error
		FROM conversations c
		LEFT JOIN jobs j ON j.id = (
		    SELECT j2.id FROM jobs j2 WHERE j2.conversation_id = c.id AND j2.input_hash = c.source_hash
		    ORDER BY j2.id DESC LIMIT 1)
		LEFT JOIN evaluations e ON e.id = (
		    SELECT e2.id FROM evaluations e2 WHERE e2.conversation_id = c.id AND e2.input_hash = c.source_hash
		    ORDER BY e2.id DESC LIMIT 1)
		ORDER BY c.updated_at DESC, c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ConversationSummary{}
	for rows.Next() {
		var cs domain.ConversationSummary
		var imported, updated string
		var jobID, evalID sql.NullInt64
		var status sql.NullString
		var total sql.NullFloat64
		var gross sql.NullBool
		if err := rows.Scan(&cs.ID, &cs.Channel, &cs.Language, &cs.AudioURL, &cs.SourceHash, &imported, &updated, &cs.TurnCount,
			&jobID, &status, &evalID, &total, &gross); err != nil {
			return nil, err
		}
		if jobID.Valid {
			cs.LatestJobID = &jobID.Int64
			st := domain.JobStatus(status.String)
			cs.LatestJobStatus = &st
		}
		if evalID.Valid {
			cs.LatestEvaluationID = &evalID.Int64
			cs.LatestGrossError = &gross.Bool
		}
		if total.Valid {
			cs.LatestTotal = &total.Float64
		}
		cs.ImportedAt, cs.UpdatedAt = parseTime(imported), parseTime(updated)
		out = append(out, cs)
	}
	return out, rows.Err()
}

func (s *Store) GetConversation(ctx context.Context, id string) (domain.Conversation, error) {
	var c domain.Conversation
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var imported, updated string
		err := tx.QueryRowContext(ctx, `SELECT id, channel, language, audio_url, source_hash, imported_at, updated_at FROM conversations WHERE id = ?`, id).
			Scan(&c.ID, &c.Channel, &c.Language, &c.AudioURL, &c.SourceHash, &imported, &updated)
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		c.ImportedAt, c.UpdatedAt = parseTime(imported), parseTime(updated)
		rows, err := tx.QueryContext(ctx, `SELECT turn_id, timestamp, speaker, text FROM turns WHERE conversation_id = ? ORDER BY turn_id`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		c.Turns = []domain.Turn{}
		for rows.Next() {
			var t domain.Turn
			if err := rows.Scan(&t.TurnID, &t.Timestamp, &t.Speaker, &t.Text); err != nil {
				return err
			}
			c.Turns = append(c.Turns, t)
		}
		return rows.Err()
	})
	return c, err
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

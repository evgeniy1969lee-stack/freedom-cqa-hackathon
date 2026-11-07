package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/homepunks/datathong/internal/domain"
)

type ChecklistSummary struct {
	Version       int       `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	Note          string    `json:"note"`
	CriteriaCount int       `json:"criteria_count"`
}

func (s *Store) SaveChecklist(ctx context.Context, cl domain.Checklist) (domain.Checklist, error) {
	if err := cl.Validate(); err != nil {
		return cl, err
	}
	cl.CreatedAt = time.Now().UTC()
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM checklists`).Scan(&cl.Version); err != nil {
			return err
		}
		body, err := json.Marshal(cl)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO checklists (version, created_at, note, criteria_json) VALUES (?, ?, ?, ?)`,
			cl.Version, cl.CreatedAt.Format(time.RFC3339Nano), cl.Note, string(body))
		return err
	})
	if err != nil {
		return cl, fmt.Errorf("save checklist: %w", err)
	}
	return cl, nil
}

func (s *Store) ListChecklists(ctx context.Context) ([]ChecklistSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version, created_at, note, criteria_json FROM checklists ORDER BY version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChecklistSummary{}
	for rows.Next() {
		var cs ChecklistSummary
		var created, body string
		if err := rows.Scan(&cs.Version, &created, &cs.Note, &body); err != nil {
			return nil, err
		}
		var cl domain.Checklist
		if err := json.Unmarshal([]byte(body), &cl); err != nil {
			return nil, err
		}
		cs.CreatedAt = parseTime(created)
		cs.CriteriaCount = len(cl.Criteria)
		out = append(out, cs)
	}
	return out, rows.Err()
}

func (s *Store) GetChecklist(ctx context.Context, version int) (domain.Checklist, error) {
	var body string
	var err error
	if version == 0 {
		err = s.db.QueryRowContext(ctx, `SELECT criteria_json FROM checklists ORDER BY version DESC LIMIT 1`).Scan(&body)
	} else {
		err = s.db.QueryRowContext(ctx, `SELECT criteria_json FROM checklists WHERE version = ?`, version).Scan(&body)
	}
	var cl domain.Checklist
	if err == sql.ErrNoRows {
		return cl, ErrNotFound
	}
	if err != nil {
		return cl, err
	}
	if err := json.Unmarshal([]byte(body), &cl); err != nil {
		return cl, fmt.Errorf("decode checklist: %w", err)
	}
	return cl, nil
}

func (s *Store) EnsureSeedChecklist(ctx context.Context, seed domain.Checklist) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM checklists`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := s.SaveChecklist(ctx, seed)
	return err
}

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/homepunks/datathong/internal/domain"
)

func (s *Store) CompleteJob(ctx context.Context, job domain.Job, ev domain.Evaluation) (domain.Evaluation, error) {
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id = ?`, job.ID).Scan(&status); err != nil {
			return err
		}
		if status != string(domain.JobRunning) {
			return fmt.Errorf("job %d is %s, not running", job.ID, status)
		}
		clJSON, err := json.Marshal(ev.Checklist)
		if err != nil {
			return err
		}
		geEv, _ := json.Marshal(ev.GrossError.Evidence)
		ts := now()
		res, err := tx.ExecContext(ctx, `INSERT INTO evaluations
			(job_id, conversation_id, input_hash, checklist_version, checklist_json, model, prompt_version, total, w,
			 gross_error, gross_error_category, gross_error_reason, gross_error_evidence_json, raw_json, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			job.ID, ev.ConversationID, ev.InputHash, ev.ChecklistVersion, string(clJSON), ev.Model, ev.PromptVersion,
			ev.Total, ev.W, ev.GrossError.Flag, ev.GrossError.Category, ev.GrossError.Reason, string(geEv), string(ev.Raw), ts)
		if err != nil {
			return err
		}
		ev.ID, err = res.LastInsertId()
		if err != nil {
			return err
		}
		ev.JobID = job.ID
		ev.CreatedAt = parseTime(ts)
		for i, it := range ev.Items {
			evJSON, _ := json.Marshal(it.Evidence)
			if _, err := tx.ExecContext(ctx, `INSERT INTO evaluation_items
				(evaluation_id, position, criterion_id, answer, reason, evidence_json, weight, applicable, q, points)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				ev.ID, i, it.CriterionID, it.Answer, it.Reason, string(evJSON), it.Weight, it.Applicable, it.Q, it.Points); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE jobs SET status = 'completed', model = ?, prompt_version = ?, error = '', finished_at = ? WHERE id = ?`,
			ev.Model, ev.PromptVersion, ts, job.ID)
		return err
	})
	if err != nil {
		return ev, fmt.Errorf("complete job %d: %w", job.ID, err)
	}
	return ev, nil
}

const evalColumns = `id, job_id, conversation_id, input_hash, checklist_version, checklist_json, model, prompt_version, total, w,
	gross_error, gross_error_category, gross_error_reason, gross_error_evidence_json, raw_json, created_at`

func (s *Store) scanEvaluation(ctx context.Context, row interface{ Scan(...any) error }) (domain.Evaluation, error) {
	var ev domain.Evaluation
	var clJSON, geEv, raw, created string
	var total sql.NullFloat64
	err := row.Scan(&ev.ID, &ev.JobID, &ev.ConversationID, &ev.InputHash, &ev.ChecklistVersion, &clJSON, &ev.Model, &ev.PromptVersion,
		&total, &ev.W, &ev.GrossError.Flag, &ev.GrossError.Category, &ev.GrossError.Reason, &geEv, &raw, &created)
	if err != nil {
		return ev, err
	}
	if total.Valid {
		ev.Total = &total.Float64
	}
	if err := json.Unmarshal([]byte(clJSON), &ev.Checklist); err != nil {
		return ev, fmt.Errorf("decode evaluation checklist: %w", err)
	}
	_ = json.Unmarshal([]byte(geEv), &ev.GrossError.Evidence)
	ev.Raw = []byte(raw)
	ev.CreatedAt = parseTime(created)
	rows, err := s.db.QueryContext(ctx, `SELECT criterion_id, answer, reason, evidence_json, weight, applicable, q, points
		FROM evaluation_items WHERE evaluation_id = ? ORDER BY position`, ev.ID)
	if err != nil {
		return ev, err
	}
	defer rows.Close()
	ev.Items = []domain.EvaluationItem{}
	for rows.Next() {
		var it domain.EvaluationItem
		var evJSON string
		if err := rows.Scan(&it.CriterionID, &it.Answer, &it.Reason, &evJSON, &it.Weight, &it.Applicable, &it.Q, &it.Points); err != nil {
			return ev, err
		}
		_ = json.Unmarshal([]byte(evJSON), &it.Evidence)
		ev.Items = append(ev.Items, it)
	}
	return ev, rows.Err()
}

func (s *Store) GetEvaluation(ctx context.Context, id int64) (domain.Evaluation, error) {
	ev, err := s.scanEvaluation(ctx, s.db.QueryRowContext(ctx, `SELECT `+evalColumns+` FROM evaluations WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return ev, ErrNotFound
	}
	return ev, err
}

func (s *Store) ListEvaluations(ctx context.Context, conversationID string) ([]domain.Evaluation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM evaluations WHERE conversation_id = ? ORDER BY id DESC`, conversationID)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := []domain.Evaluation{}
	for _, id := range ids {
		ev, err := s.GetEvaluation(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}

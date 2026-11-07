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
	gross_error, gross_error_category, gross_error_reason, gross_error_evidence_json, raw_json, created_at, final_total, final_w, revised_at`

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *Store) scanEvaluation(ctx context.Context, q querier, row interface{ Scan(...any) error }) (domain.Evaluation, error) {
	var ev domain.Evaluation
	var clJSON, geEv, raw, created string
	var total, finalTotal, finalW sql.NullFloat64
	var revised sql.NullString
	err := row.Scan(&ev.ID, &ev.JobID, &ev.ConversationID, &ev.InputHash, &ev.ChecklistVersion, &clJSON, &ev.Model, &ev.PromptVersion,
		&total, &ev.W, &ev.GrossError.Flag, &ev.GrossError.Category, &ev.GrossError.Reason, &geEv, &raw, &created, &finalTotal, &finalW, &revised)
	if err != nil {
		return ev, err
	}
	if total.Valid {
		ev.Total = &total.Float64
	}
	if finalTotal.Valid {
		ev.FinalTotal = &finalTotal.Float64
	}
	if finalW.Valid {
		ev.FinalW = &finalW.Float64
	}
	ev.RevisedAt = nullTime(revised)
	if err := json.Unmarshal([]byte(clJSON), &ev.Checklist); err != nil {
		return ev, fmt.Errorf("decode evaluation checklist: %w", err)
	}
	_ = json.Unmarshal([]byte(geEv), &ev.GrossError.Evidence)
	ev.Raw = []byte(raw)
	ev.CreatedAt = parseTime(created)
	rows, err := q.QueryContext(ctx, `SELECT criterion_id, answer, reason, evidence_json, weight, applicable, q, points, final_answer
		FROM evaluation_items WHERE evaluation_id = ? ORDER BY position`, ev.ID)
	if err != nil {
		return ev, err
	}
	ev.Items = []domain.EvaluationItem{}
	for rows.Next() {
		var it domain.EvaluationItem
		var evJSON string
		var final sql.NullString
		if err := rows.Scan(&it.CriterionID, &it.Answer, &it.Reason, &evJSON, &it.Weight, &it.Applicable, &it.Q, &it.Points, &final); err != nil {
			rows.Close()
			return ev, err
		}
		_ = json.Unmarshal([]byte(evJSON), &it.Evidence)
		if final.Valid {
			it.FinalAnswer = &final.String
		}
		ev.Items = append(ev.Items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ev, err
	}
	ev.Appeals, err = listAppeals(ctx, q, ev.ID, "")
	return ev, err
}

func (s *Store) GetEvaluation(ctx context.Context, id int64) (domain.Evaluation, error) {
	ev, err := s.scanEvaluation(ctx, s.db, s.db.QueryRowContext(ctx, `SELECT `+evalColumns+` FROM evaluations WHERE id = ?`, id))
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

func (s *Store) ExportEvaluationIDs(ctx context.Context, conversationID string, checklistVersion int, all bool) ([]int64, error) {
	q := `SELECT e.id FROM evaluations e`
	if !all {
		q += ` JOIN conversations c ON c.id = e.conversation_id AND c.source_hash = e.input_hash`
	}
	q += ` WHERE 1=1`
	var args []any
	if conversationID != "" {
		q += ` AND e.conversation_id = ?`
		args = append(args, conversationID)
	}
	if checklistVersion != 0 {
		q += ` AND e.checklist_version = ?`
		args = append(args, checklistVersion)
	}
	if !all {
		q += ` AND e.id = (SELECT MAX(e2.id) FROM evaluations e2
		                    WHERE e2.conversation_id = e.conversation_id AND e2.input_hash = e.input_hash`
		if checklistVersion != 0 {
			q += ` AND e2.checklist_version = ?`
			args = append(args, checklistVersion)
		}
		q += `)`
	}
	q += ` ORDER BY e.conversation_id, e.id`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

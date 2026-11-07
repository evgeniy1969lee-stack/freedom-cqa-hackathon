package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/homepunks/datathong/internal/domain"
)

var ErrJobNotFailed = errors.New("job is not in failed state")

const jobColumns = `j.id, j.conversation_id, j.input_hash, j.checklist_version, j.checklist_json, j.status,
	j.attempts, j.max_attempts, j.error, j.model, j.prompt_version, j.created_at, j.started_at, j.finished_at,
	(SELECT e.id FROM evaluations e WHERE e.job_id = j.id)`

func scanJob(row interface{ Scan(...any) error }) (domain.Job, error) {
	var j domain.Job
	var clJSON, created string
	var started, finished sql.NullString
	var evalID sql.NullInt64
	err := row.Scan(&j.ID, &j.ConversationID, &j.InputHash, &j.ChecklistVersion, &clJSON, &j.Status,
		&j.Attempts, &j.MaxAttempts, &j.Error, &j.Model, &j.PromptVersion, &created, &started, &finished, &evalID)
	if err != nil {
		return j, err
	}
	if err := json.Unmarshal([]byte(clJSON), &j.Checklist); err != nil {
		return j, fmt.Errorf("decode job checklist: %w", err)
	}
	j.CreatedAt = parseTime(created)
	j.StartedAt = nullTime(started)
	j.FinishedAt = nullTime(finished)
	if evalID.Valid {
		j.EvaluationID = &evalID.Int64
	}
	return j, nil
}

func nullTime(s sql.NullString) *time.Time {
	if !s.Valid {
		return nil
	}
	t := parseTime(s.String)
	return &t
}

func (s *Store) CreateJob(ctx context.Context, conv domain.Conversation, cl domain.Checklist, maxAttempts int, forceNew bool) (domain.Job, bool, error) {
	var job domain.Job
	created := false
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if !forceNew {
			row := tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs j
				WHERE j.conversation_id = ? AND j.input_hash = ? AND j.checklist_version = ? AND j.status IN ('queued','running')
				ORDER BY j.id DESC LIMIT 1`, conv.ID, conv.SourceHash, cl.Version)
			j, err := scanJob(row)
			if err == nil {
				job = j
				return nil
			}
			if err != sql.ErrNoRows {
				return err
			}
		}
		clJSON, err := json.Marshal(cl)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO jobs (conversation_id, input_hash, checklist_version, checklist_json, status, max_attempts, created_at)
			VALUES (?, ?, ?, ?, 'queued', ?, ?)`,
			conv.ID, conv.SourceHash, cl.Version, string(clJSON), maxAttempts, now())
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		job, err = scanJob(tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs j WHERE j.id = ?`, id))
		created = err == nil
		return err
	})
	if err != nil {
		if forceNew && strings.Contains(err.Error(), "UNIQUE") {
			return job, false, fmt.Errorf("an active job for this input already exists; wait for it or let it finish")
		}
		return job, false, fmt.Errorf("create job: %w", err)
	}
	return job, created, nil
}

func (s *Store) ClaimJob(ctx context.Context) (domain.Job, bool, error) {
	row := s.db.QueryRowContext(ctx, `UPDATE jobs SET status = 'running', attempts = attempts + 1, started_at = ?, error = ''
		WHERE id = (SELECT id FROM jobs WHERE status = 'queued' ORDER BY id LIMIT 1)
		RETURNING `+strings.ReplaceAll(jobColumns, "j.", "jobs."), now())
	j, err := scanJob(row)
	if err == sql.ErrNoRows {
		return j, false, nil
	}
	if err != nil {
		return j, false, fmt.Errorf("claim job: %w", err)
	}
	return j, true, nil
}

func (s *Store) FailJob(ctx context.Context, id int64, msg string, retry bool) error {
	status, finished := domain.JobFailed, sql.NullString{String: now(), Valid: true}
	if retry {
		status, finished = domain.JobQueued, sql.NullString{}
	}
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET status = ?, error = ?, finished_at = ? WHERE id = ?`, status, msg, finished, id)
	return err
}

func (s *Store) RecoverRunningJobs(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE jobs SET status = 'queued' WHERE status = 'running'`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) RetryJob(ctx context.Context, id int64) (domain.Job, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE jobs SET status = 'queued', attempts = 0, error = '', finished_at = NULL WHERE id = ? AND status = 'failed'`, id)
	if err != nil {
		return domain.Job{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		if _, err := s.GetJob(ctx, id); err != nil {
			return domain.Job{}, err
		}
		return domain.Job{}, ErrJobNotFailed
	}
	return s.GetJob(ctx, id)
}

func (s *Store) GetJob(ctx context.Context, id int64) (domain.Job, error) {
	j, err := scanJob(s.db.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs j WHERE j.id = ?`, id))
	if err == sql.ErrNoRows {
		return j, ErrNotFound
	}
	return j, err
}

func (s *Store) ListJobs(ctx context.Context, conversationID string, status string) ([]domain.Job, error) {
	q := `SELECT ` + jobColumns + ` FROM jobs j WHERE 1=1`
	var args []any
	if conversationID != "" {
		q += ` AND j.conversation_id = ?`
		args = append(args, conversationID)
	}
	if status != "" {
		q += ` AND j.status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY j.id DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) JobCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM jobs GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

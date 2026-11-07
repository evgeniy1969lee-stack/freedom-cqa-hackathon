package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/homepunks/datathong/internal/domain"
	"github.com/homepunks/datathong/internal/scoring"
)

var (
	ErrAppealOpenExists = errors.New("an open appeal for this criterion already exists")
	ErrAppealResolved   = errors.New("appeal is already resolved")
	ErrUnknownCriterion = errors.New("criterion is not part of this evaluation")
)

const appealColumns = `id, evaluation_id, criterion_id, comment, original_answer, status, decision, final_answer, reason, created_at, resolved_at`

func scanAppeal(row interface{ Scan(...any) error }) (domain.Appeal, error) {
	var a domain.Appeal
	var created string
	var final, resolved sql.NullString
	err := row.Scan(&a.ID, &a.EvaluationID, &a.CriterionID, &a.Comment, &a.OriginalAnswer, &a.Status, &a.Decision, &final, &a.Reason, &created, &resolved)
	if err != nil {
		return a, err
	}
	a.CreatedAt = parseTime(created)
	a.ResolvedAt = nullTime(resolved)
	if final.Valid {
		a.FinalAnswer = &final.String
	}
	return a, nil
}

func (s *Store) CreateAppeal(ctx context.Context, evaluationID int64, criterionID, comment string) (domain.Appeal, error) {
	var a domain.Appeal
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var original string
		err := tx.QueryRowContext(ctx, `SELECT answer FROM evaluation_items WHERE evaluation_id = ? AND criterion_id = ?`, evaluationID, criterionID).Scan(&original)
		if err == sql.ErrNoRows {
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM evaluations WHERE id = ?`, evaluationID).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return ErrNotFound
			}
			return ErrUnknownCriterion
		}
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO appeals (evaluation_id, criterion_id, comment, original_answer, status, created_at)
			VALUES (?, ?, ?, ?, 'open', ?)`, evaluationID, criterionID, comment, original, now())
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return ErrAppealOpenExists
			}
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		a, err = scanAppeal(tx.QueryRowContext(ctx, `SELECT `+appealColumns+` FROM appeals WHERE id = ?`, id))
		return err
	})
	return a, err
}

func (s *Store) GetAppeal(ctx context.Context, id int64) (domain.Appeal, error) {
	a, err := scanAppeal(s.db.QueryRowContext(ctx, `SELECT `+appealColumns+` FROM appeals WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return a, ErrNotFound
	}
	return a, err
}

func (s *Store) ListAppeals(ctx context.Context, evaluationID int64, status string) ([]domain.Appeal, error) {
	return listAppeals(ctx, s.db, evaluationID, status)
}

func listAppeals(ctx context.Context, db querier, evaluationID int64, status string) ([]domain.Appeal, error) {
	q := `SELECT ` + appealColumns + ` FROM appeals WHERE 1=1`
	var args []any
	if evaluationID != 0 {
		q += ` AND evaluation_id = ?`
		args = append(args, evaluationID)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY id DESC`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Appeal{}
	for rows.Next() {
		a, err := scanAppeal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) DecideAppeal(ctx context.Context, id int64, decision domain.Decision, newAnswer *domain.AnswerValue, reason string) (domain.Appeal, domain.Evaluation, error) {
	var a domain.Appeal
	var ev domain.Evaluation
	err := s.tx(ctx, func(tx *sql.Tx) error {
		cur, err := scanAppeal(tx.QueryRowContext(ctx, `SELECT `+appealColumns+` FROM appeals WHERE id = ?`, id))
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if cur.Status != domain.AppealOpen {
			return ErrAppealResolved
		}
		ev, err = s.scanEvaluation(ctx, tx, tx.QueryRowContext(ctx, `SELECT `+evalColumns+` FROM evaluations WHERE id = ?`, cur.EvaluationID))
		if err != nil {
			return err
		}
		var finalStr *string
		if decision == domain.DecisionChanged {
			str := newAnswer.String()
			finalStr = &str
		}
		answers := make([]domain.Answer, 0, len(ev.Items))
		for _, it := range ev.Items {
			raw := it.Answer
			if it.FinalAnswer != nil {
				raw = *it.FinalAnswer
			}
			if it.CriterionID == cur.CriterionID {
				raw = it.Answer
				if finalStr != nil {
					raw = *finalStr
				}
			}
			var v domain.AnswerValue
			if err := v.UnmarshalJSON([]byte(fmt.Sprintf("%q", raw))); err != nil {
				return fmt.Errorf("stored answer %q: %w", raw, err)
			}
			answers = append(answers, domain.Answer{CriterionID: it.CriterionID, Value: v})
		}
		res, err := scoring.Score(ev.Checklist, answers)
		if err != nil {
			return err
		}
		upd, err := tx.ExecContext(ctx, `UPDATE appeals SET status = 'resolved', decision = ?, final_answer = ?, reason = ?, resolved_at = ?
			WHERE id = ? AND status = 'open'`, decision, finalStr, reason, now(), id)
		if err != nil {
			return err
		}
		if n, _ := upd.RowsAffected(); n != 1 {
			return ErrAppealResolved
		}
		if _, err := tx.ExecContext(ctx, `UPDATE evaluation_items SET final_answer = ? WHERE evaluation_id = ? AND criterion_id = ?`,
			finalStr, ev.ID, cur.CriterionID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE evaluations SET final_total = ?, final_w = ?, revised_at = ? WHERE id = ?`,
			res.Total, res.W, now(), ev.ID); err != nil {
			return err
		}
		a, err = scanAppeal(tx.QueryRowContext(ctx, `SELECT `+appealColumns+` FROM appeals WHERE id = ?`, id))
		if err != nil {
			return err
		}
		ev, err = s.scanEvaluation(ctx, tx, tx.QueryRowContext(ctx, `SELECT `+evalColumns+` FROM evaluations WHERE id = ?`, ev.ID))
		return err
	})
	return a, ev, err
}

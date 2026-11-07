package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/homepunks/datathong/internal/domain"
	"github.com/homepunks/datathong/internal/evaluator"
	"github.com/homepunks/datathong/internal/store"
)

type Worker struct {
	store   *store.Store
	eval    evaluator.Evaluator
	timeout time.Duration
	workers int
	poll    time.Duration
	log     *slog.Logger
	kick    chan struct{}
}

func New(st *store.Store, ev evaluator.Evaluator, workers int, timeout time.Duration, log *slog.Logger) *Worker {
	if workers < 1 {
		workers = 1
	}
	return &Worker{store: st, eval: ev, timeout: timeout, workers: workers, poll: 2 * time.Second, log: log, kick: make(chan struct{}, 1)}
}

func (w *Worker) Kick() {
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

func (w *Worker) Run(ctx context.Context) {
	if n, err := w.store.RecoverRunningJobs(ctx); err != nil {
		w.log.Error("recover running jobs", "err", err)
	} else if n > 0 {
		w.log.Info("requeued jobs interrupted by restart", "count", n)
	}
	var wg sync.WaitGroup
	for i := 0; i < w.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.loop(ctx)
		}()
	}
	wg.Wait()
}

func (w *Worker) loop(ctx context.Context) {
	for {
		job, ok, err := w.store.ClaimJob(ctx)
		if err != nil {
			w.log.Error("claim job", "err", err)
		}
		if ok {
			w.process(ctx, job)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-w.kick:
		case <-time.After(w.poll):
		}
	}
}

func (w *Worker) process(ctx context.Context, job domain.Job) {
	ev, err := w.run(ctx, job)
	if err == nil {
		w.log.Info("job completed", "job", job.ID, "conversation", job.ConversationID, "total", ev.Total)
		return
	}
	if ctx.Err() != nil {
		w.log.Warn("job interrupted by shutdown; left running for recovery", "job", job.ID)
		return
	}
	retry := job.Attempts < job.MaxAttempts && !isPermanent(err)
	w.log.Warn("job failed", "job", job.ID, "attempt", job.Attempts, "retry", retry, "err", err)
	if ferr := w.store.FailJob(ctx, job.ID, err.Error(), retry); ferr != nil {
		w.log.Error("record job failure", "job", job.ID, "err", ferr)
	}
}

type permanentError struct{ error }

func isPermanent(err error) bool {
	_, ok := err.(permanentError)
	return ok
}

func (w *Worker) run(ctx context.Context, job domain.Job) (domain.Evaluation, error) {
	conv, err := w.store.GetConversation(ctx, job.ConversationID)
	if err != nil {
		return domain.Evaluation{}, permanentError{fmt.Errorf("load conversation: %w", err)}
	}
	if conv.SourceHash != job.InputHash {
		return domain.Evaluation{}, permanentError{fmt.Errorf("conversation changed since the job was created; start a new evaluation")}
	}
	cctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	resp, raw, err := w.eval.Evaluate(cctx, evaluator.Request{Conversation: conv, Checklist: job.Checklist})
	if err != nil {
		if cctx.Err() == context.DeadlineExceeded {
			return domain.Evaluation{}, fmt.Errorf("ml call exceeded timeout of %s", w.timeout)
		}
		return domain.Evaluation{}, err
	}
	res, err := evaluator.Validate(conv, job.Checklist, resp)
	if err != nil {
		return domain.Evaluation{}, err
	}
	byID := make(map[string]domain.MLItem, len(resp.Results))
	for _, it := range resp.Results {
		byID[it.CriterionID] = it
	}
	ev := domain.Evaluation{
		ConversationID:   conv.ID,
		InputHash:        conv.SourceHash,
		ChecklistVersion: job.ChecklistVersion,
		Checklist:        job.Checklist,
		Model:            resp.Model,
		PromptVersion:    resp.PromptVersion,
		Total:            res.Total,
		W:                res.W,
		Raw:              raw,
	}
	if resp.GrossError != nil {
		ev.GrossError = *resp.GrossError
	}
	for _, s := range res.Items {
		ml := byID[s.CriterionID]
		ev.Items = append(ev.Items, domain.EvaluationItem{
			CriterionID: s.CriterionID, Answer: s.Answer, Reason: ml.Reason, Evidence: ml.Evidence,
			Weight: s.Weight, Applicable: s.Applicable, Q: s.Q, Points: s.Points,
		})
	}
	return w.store.CompleteJob(ctx, job, ev)
}

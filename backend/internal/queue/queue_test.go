package queue

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/homepunks/datathong/internal/checklist"
	"github.com/homepunks/datathong/internal/domain"
	"github.com/homepunks/datathong/internal/evaluator"
	"github.com/homepunks/datathong/internal/importer"
	"github.com/homepunks/datathong/internal/store"
)

type scripted struct {
	calls atomic.Int32
	fn    func(call int, req evaluator.Request) (domain.MLResponse, error)
}

func (s *scripted) Evaluate(ctx context.Context, req evaluator.Request) (domain.MLResponse, []byte, error) {
	n := int(s.calls.Add(1))
	r, err := s.fn(n, req)
	return r, []byte("{}"), err
}

func good(req evaluator.Request) domain.MLResponse {
	out := domain.MLResponse{Model: "test", PromptVersion: "t1", GrossError: &domain.GrossError{}}
	for _, c := range req.Checklist.Criteria {
		it := domain.MLItem{CriterionID: c.ID, Reason: "ok", Evidence: domain.Evidence{Kind: "turns", TurnIDs: []int{0}}}
		switch {
		case c.NAAllowed:
			it.Answer = domain.AnswerValue{Kind: domain.AnswerNA}
		case c.Type == domain.TypeScale:
			it.Answer = domain.AnswerValue{Kind: domain.AnswerStep, Step: c.MaxStep()}
		default:
			it.Answer = domain.AnswerValue{Kind: domain.AnswerYes}
		}
		out.Results = append(out.Results, it)
	}
	return out
}

func setup(t *testing.T) (*store.Store, domain.Conversation, domain.Checklist) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "q.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	seed, _ := checklist.Seed()
	for i := range seed.Criteria {
		seed.Criteria[i].Weight = 10
	}
	cl, err := st.SaveChecklist(ctx, seed)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile("../../testdata/example_conversation.json")
	convs, _ := importer.Parse(body)
	if _, err := st.UpsertConversation(ctx, convs[0]); err != nil {
		t.Fatal(err)
	}
	conv, _ := st.GetConversation(ctx, convs[0].ID)
	return st, conv, cl
}

func runUntil(t *testing.T, st *store.Store, ev evaluator.Evaluator, id int64, want domain.JobStatus) domain.Job {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := New(st, ev, 2, 2*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	w.poll = 20 * time.Millisecond
	go w.Run(ctx)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j, err := st.GetJob(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if j.Status == want {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	j, _ := st.GetJob(ctx, id)
	t.Fatalf("job %d did not reach %s: %+v", id, want, j)
	return j
}

func TestSuccessPath(t *testing.T) {
	st, conv, cl := setup(t)
	ctx := context.Background()
	job, created, err := st.CreateJob(ctx, conv, cl, 3, false)
	if err != nil || !created || job.Status != domain.JobQueued {
		t.Fatalf("create: %+v %v %v", job, created, err)
	}
	ev := &scripted{fn: func(_ int, req evaluator.Request) (domain.MLResponse, error) { return good(req), nil }}
	j := runUntil(t, st, ev, job.ID, domain.JobCompleted)
	if j.Attempts != 1 || j.Model != "test" || j.EvaluationID == nil {
		t.Fatalf("job: %+v", j)
	}
	e, err := st.GetEvaluation(ctx, *j.EvaluationID)
	if err != nil {
		t.Fatal(err)
	}
	if e.Total == nil || *e.Total != 100 || len(e.Items) != 7 || e.W != 30 {
		t.Fatalf("evaluation: total=%v w=%v items=%d", e.Total, e.W, len(e.Items))
	}
	if e.Items[1].Answer != "N/A" || e.Items[1].Applicable {
		t.Fatalf("item 02: %+v", e.Items[1])
	}
	list, _ := st.ListEvaluations(ctx, conv.ID)
	if len(list) != 1 {
		t.Fatalf("list: %d", len(list))
	}
}

func TestRetryThenSuccess(t *testing.T) {
	st, conv, cl := setup(t)
	job, _, _ := st.CreateJob(context.Background(), conv, cl, 3, false)
	ev := &scripted{fn: func(call int, req evaluator.Request) (domain.MLResponse, error) {
		if call < 3 {
			return domain.MLResponse{}, errors.New("connection refused")
		}
		return good(req), nil
	}}
	j := runUntil(t, st, ev, job.ID, domain.JobCompleted)
	if j.Attempts != 3 || j.Error != "" {
		t.Fatalf("job: %+v", j)
	}
}

func TestFailsAfterMaxAttempts(t *testing.T) {
	st, conv, cl := setup(t)
	job, _, _ := st.CreateJob(context.Background(), conv, cl, 2, false)
	ev := &scripted{fn: func(_ int, req evaluator.Request) (domain.MLResponse, error) {
		r := good(req)
		r.Results[0].Evidence.TurnIDs = []int{99}
		return r, nil
	}}
	j := runUntil(t, st, ev, job.ID, domain.JobFailed)
	if j.Attempts != 2 || j.Error == "" || j.EvaluationID != nil {
		t.Fatalf("job: %+v", j)
	}
	retried, err := st.RetryJob(context.Background(), job.ID)
	if err != nil || retried.Status != domain.JobQueued || retried.Attempts != 0 {
		t.Fatalf("retry: %+v %v", retried, err)
	}
	if _, err := st.RetryJob(context.Background(), job.ID); !errors.Is(err, store.ErrJobNotFailed) {
		t.Fatalf("retry of queued job: %v", err)
	}
}

func TestDedupeAndForceNew(t *testing.T) {
	st, conv, cl := setup(t)
	ctx := context.Background()
	a, created, _ := st.CreateJob(ctx, conv, cl, 3, false)
	b, created2, _ := st.CreateJob(ctx, conv, cl, 3, false)
	if !created || created2 || a.ID != b.ID {
		t.Fatalf("dedupe: %d %d %v %v", a.ID, b.ID, created, created2)
	}
	if _, _, err := st.CreateJob(ctx, conv, cl, 3, true); err == nil {
		t.Fatal("force_new while active must be refused")
	}
	ev := &scripted{fn: func(_ int, req evaluator.Request) (domain.MLResponse, error) { return good(req), nil }}
	runUntil(t, st, ev, a.ID, domain.JobCompleted)
	c, created3, err := st.CreateJob(ctx, conv, cl, 3, true)
	if err != nil || !created3 || c.ID == a.ID {
		t.Fatalf("force_new after completion: %+v %v %v", c, created3, err)
	}
}

func TestRestartRecovery(t *testing.T) {
	st, conv, cl := setup(t)
	ctx := context.Background()
	job, _, _ := st.CreateJob(ctx, conv, cl, 3, false)
	claimed, ok, err := st.ClaimJob(ctx)
	if err != nil || !ok || claimed.ID != job.ID || claimed.Status != domain.JobRunning {
		t.Fatalf("claim: %+v %v %v", claimed, ok, err)
	}
	n, err := st.RecoverRunningJobs(ctx)
	if err != nil || n != 1 {
		t.Fatalf("recover: %d %v", n, err)
	}
	j, _ := st.GetJob(ctx, job.ID)
	if j.Status != domain.JobQueued {
		t.Fatalf("after recovery: %s", j.Status)
	}
}

func TestChangedConversationIsPermanentFailure(t *testing.T) {
	st, conv, cl := setup(t)
	ctx := context.Background()
	job, _, _ := st.CreateJob(ctx, conv, cl, 3, false)
	conv.Turns[0].Text += "!"
	changed, _ := importer.Parse(mustJSON(conv))
	if _, err := st.UpsertConversation(ctx, changed[0]); err != nil {
		t.Fatal(err)
	}
	ev := &scripted{fn: func(_ int, req evaluator.Request) (domain.MLResponse, error) { return good(req), nil }}
	j := runUntil(t, st, ev, job.ID, domain.JobFailed)
	if j.Attempts != 1 {
		t.Fatalf("permanent failure must not retry: %+v", j)
	}
}

func mustJSON(c domain.Conversation) []byte {
	b, _ := json.Marshal(c)
	return b
}

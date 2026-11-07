package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/homepunks/datathong/internal/checklist"
	"github.com/homepunks/datathong/internal/domain"
	"github.com/homepunks/datathong/internal/evaluator"
	"github.com/homepunks/datathong/internal/queue"
	"github.com/homepunks/datathong/internal/store"
)

func newServerWithWorker(t *testing.T) http.Handler {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	seed, _ := checklist.Seed()
	for i := range seed.Criteria {
		seed.Criteria[i].Weight = 1
	}
	if err := st.EnsureSeedChecklist(ctx, seed); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := queue.New(st, evaluator.Stub{}, 1, time.Second, log)
	go w.Run(ctx)
	return New(st, Options{Worker: w, MaxAttempts: 2}, log)
}

func TestEvaluateFlow(t *testing.T) {
	h := newServerWithWorker(t)
	records, _ := os.ReadFile("../../testdata/example_records.json")
	do(t, h, "POST", "/api/import", records, nil)

	var res struct {
		Job     domain.Job `json:"job"`
		Created bool       `json:"created"`
	}
	if code := do(t, h, "POST", "/api/conversations/example-001/evaluate", []byte(`{"checklist_version":1}`), &res); code != 202 || !res.Created {
		t.Fatalf("evaluate: %d %+v", code, res)
	}
	if code := do(t, h, "POST", "/api/conversations/nope/evaluate", []byte(`{}`), nil); code != 404 {
		t.Fatalf("missing conversation: %d", code)
	}
	if code := do(t, h, "POST", "/api/conversations/example-001/evaluate", []byte(`{"checklist_version":42}`), nil); code != 400 {
		t.Fatalf("bad version: %d", code)
	}

	var job domain.Job
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		do(t, h, "GET", "/api/jobs/"+itoa(res.Job.ID), nil, &job)
		if job.Status == domain.JobCompleted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if job.Status != domain.JobCompleted || job.EvaluationID == nil || job.Model != "stub" {
		t.Fatalf("job: %+v", job)
	}

	var ev domain.Evaluation
	if code := do(t, h, "GET", "/api/evaluations/"+itoa(*job.EvaluationID), nil, &ev); code != 200 {
		t.Fatalf("evaluation: %d", code)
	}
	if len(ev.Items) != 7 || ev.Total == nil || ev.ChecklistVersion != 1 || ev.Items[0].Evidence.Kind != "range" {
		t.Fatalf("evaluation: %+v", ev)
	}

	var list []domain.Evaluation
	if code := do(t, h, "GET", "/api/conversations/example-001/evaluations", nil, &list); code != 200 || len(list) != 1 {
		t.Fatalf("list evaluations: %d %d", code, len(list))
	}

	var summaries []domain.ConversationSummary
	do(t, h, "GET", "/api/conversations", nil, &summaries)
	var s001 domain.ConversationSummary
	for _, s := range summaries {
		if s.ID == "example-001" {
			s001 = s
		}
	}
	if s001.LatestJobStatus == nil || *s001.LatestJobStatus != domain.JobCompleted || s001.LatestTotal == nil {
		t.Fatalf("summary: %+v", s001)
	}

	var batch struct {
		Created int `json:"created"`
		Jobs    []struct {
			ConversationID string `json:"conversation_id"`
			Created        bool   `json:"created"`
		} `json:"jobs"`
	}
	if code := do(t, h, "POST", "/api/evaluate/batch", []byte(`{}`), &batch); code != 202 || batch.Created != 2 || len(batch.Jobs) != 2 {
		t.Fatalf("batch: %d %+v", code, batch)
	}

	var jobs struct {
		Counts map[string]int `json:"counts"`
		Jobs   []domain.Job   `json:"jobs"`
	}
	if code := do(t, h, "GET", "/api/jobs?conversation_id=example-002", nil, &jobs); code != 200 || len(jobs.Jobs) != 1 {
		t.Fatalf("jobs: %d %+v", code, jobs)
	}
	if code := do(t, h, "POST", "/api/jobs/999/retry", nil, nil); code != 404 {
		t.Fatalf("retry missing: %d", code)
	}
	if code := do(t, h, "POST", "/api/jobs/"+itoa(res.Job.ID)+"/retry", nil, nil); code != 409 {
		t.Fatalf("retry completed: %d", code)
	}
}

func itoa(n int64) string {
	b := []byte{}
	if n == 0 {
		return "0"
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

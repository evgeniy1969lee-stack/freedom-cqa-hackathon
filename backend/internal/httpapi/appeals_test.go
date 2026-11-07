package httpapi

import (
	"context"
	"encoding/json"
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

type env struct {
	t      *testing.T
	path   string
	cancel context.CancelFunc
	st     *store.Store
	h      http.Handler
}

func newEnv(t *testing.T, path string) *env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := checklist.Seed()
	for i := range seed.Criteria {
		seed.Criteria[i].Weight = 10
	}
	if err := st.EnsureSeedChecklist(ctx, seed); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := queue.New(st, evaluator.Stub{}, 1, time.Second, log)
	go w.Run(ctx)
	e := &env{t: t, path: path, cancel: cancel, st: st, h: New(st, Options{Worker: w, MaxAttempts: 1}, log)}
	t.Cleanup(e.close)
	return e
}

func (e *env) close() {
	if e.st == nil {
		return
	}
	e.cancel()
	e.st.Close()
	e.st = nil
}

func (e *env) waitCompleted(jobID int64) domain.Evaluation {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var job domain.Job
		do(e.t, e.h, "GET", "/api/jobs/"+itoa(jobID), nil, &job)
		if job.Status == domain.JobCompleted {
			var ev domain.Evaluation
			do(e.t, e.h, "GET", "/api/evaluations/"+itoa(*job.EvaluationID), nil, &ev)
			return ev
		}
		if job.Status == domain.JobFailed {
			e.t.Fatalf("job failed: %s", job.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	e.t.Fatal("job did not complete")
	return domain.Evaluation{}
}

func TestChecklistEditAndAppealCycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.db")
	e := newEnv(t, path)
	h := e.h
	records, _ := os.ReadFile("../../testdata/example_records.json")
	do(t, h, "POST", "/api/import", records, nil)

	var v1 domain.Checklist
	do(t, h, "GET", "/api/checklists/latest", nil, &v1)
	v2 := v1
	v2.Note = "replace 04 with 04b, add 08"
	v2.Draft = false
	v2.Criteria = append([]domain.Criterion{}, v1.Criteria...)
	for i, c := range v2.Criteria {
		if c.ID == "04" {
			v2.Criteria[i] = domain.Criterion{ID: "04b", Name: "Консультация (новая шкала)", Weight: 30, Type: domain.TypeScale,
				Steps: []domain.Step{{Value: 0, Label: "нет"}, {Value: 1, Label: "частично"}, {Value: 2, Label: "полностью"}}, NAAllowed: true, NARule: "нет вопроса"}
		}
	}
	v2.Criteria = append(v2.Criteria, domain.Criterion{ID: "08", Name: "Новый критерий", Weight: 5, Type: domain.TypeYesNo})
	body, _ := json.Marshal(v2)
	var saved domain.Checklist
	if code := do(t, h, "POST", "/api/checklists", body, &saved); code != 201 || saved.Version != 2 || len(saved.Criteria) != 8 {
		t.Fatalf("save checklist: %d %+v", code, saved)
	}
	bad := v2
	bad.Criteria = append([]domain.Criterion{}, v2.Criteria...)
	bad.Criteria[0].Type = "maybe"
	badBody, _ := json.Marshal(bad)
	if code := do(t, h, "POST", "/api/checklists", badBody, nil); code != 400 {
		t.Fatalf("invalid checklist accepted: %d", code)
	}
	var old domain.Checklist
	do(t, h, "GET", "/api/checklists/1", nil, &old)
	if len(old.Criteria) != 7 {
		t.Fatal("version 1 must be untouched")
	}

	var res struct {
		Job domain.Job `json:"job"`
	}
	do(t, h, "POST", "/api/conversations/example-001/evaluate", []byte(`{}`), &res)
	ev := e.waitCompleted(res.Job.ID)
	if ev.ChecklistVersion != 2 || len(ev.Items) != 8 || ev.Items[3].CriterionID != "04b" || ev.Items[3].Answer != "1" {
		t.Fatalf("evaluation on v2: %+v", ev)
	}
	if ev.Total == nil || *ev.Total != 81.05 {
		t.Fatalf("total %v", ev.Total)
	}

	evPath := "/api/evaluations/" + itoa(ev.ID)
	var ap domain.Appeal
	if code := do(t, h, "POST", evPath+"/appeals", []byte(`{"criterion_id":"04b","comment":"консультация была полной"}`), &ap); code != 201 || ap.OriginalAnswer != "1" || ap.Status != domain.AppealOpen {
		t.Fatalf("appeal: %d %+v", code, ap)
	}
	if code := do(t, h, "POST", evPath+"/appeals", []byte(`{"criterion_id":"04b","comment":"dup"}`), nil); code != 409 {
		t.Fatalf("duplicate open appeal: %d", code)
	}
	if code := do(t, h, "POST", evPath+"/appeals", []byte(`{"criterion_id":"99","comment":"x"}`), nil); code != 400 {
		t.Fatalf("unknown criterion: %d", code)
	}
	if code := do(t, h, "POST", evPath+"/appeals", []byte(`{"criterion_id":"01"}`), nil); code != 400 {
		t.Fatalf("missing comment: %d", code)
	}
	if code := do(t, h, "POST", "/api/evaluations/999/appeals", []byte(`{"criterion_id":"01","comment":"x"}`), nil); code != 404 {
		t.Fatalf("missing evaluation: %d", code)
	}

	apPath := "/api/appeals/" + itoa(ap.ID) + "/decide"
	if code := do(t, h, "POST", apPath, []byte(`{"decision":"changed","new_answer":2}`), nil); code != 400 {
		t.Fatalf("missing reason accepted: %d", code)
	}
	if code := do(t, h, "POST", apPath, []byte(`{"decision":"changed","new_answer":7,"reason":"r"}`), nil); code != 400 {
		t.Fatalf("forbidden step accepted: %d", code)
	}
	var decided struct {
		Appeal     domain.Appeal     `json:"appeal"`
		Evaluation domain.Evaluation `json:"evaluation"`
	}
	if code := do(t, h, "POST", apPath, []byte(`{"decision":"changed","new_answer":"N/A","reason":"клиент не задавал вопрос"}`), &decided); code != 200 {
		t.Fatalf("decide: %d", code)
	}
	if decided.Appeal.Status != domain.AppealResolved || decided.Appeal.FinalAnswer == nil || *decided.Appeal.FinalAnswer != "N/A" {
		t.Fatalf("decided appeal: %+v", decided.Appeal)
	}
	if decided.Evaluation.FinalTotal == nil || *decided.Evaluation.FinalTotal != 95.38 || *decided.Evaluation.FinalW != 65 || *decided.Evaluation.Total != 81.05 {
		t.Fatalf("recalculated: final=%v finalW=%v original=%v", decided.Evaluation.FinalTotal, decided.Evaluation.FinalW, *decided.Evaluation.Total)
	}
	if code := do(t, h, "POST", apPath, []byte(`{"decision":"confirmed","reason":"again"}`), nil); code != 409 {
		t.Fatalf("second decision accepted: %d", code)
	}

	do(t, h, "POST", evPath+"/appeals", []byte(`{"criterion_id":"04b","comment":"вернуть"}`), &ap)
	do(t, h, "POST", "/api/appeals/"+itoa(ap.ID)+"/decide", []byte(`{"decision":"changed","new_answer":2,"reason":"полный ответ"}`), &decided)
	if *decided.Evaluation.FinalTotal != 96.84 || *decided.Evaluation.FinalW != 95 {
		t.Fatalf("back from N/A: %v %v", *decided.Evaluation.FinalTotal, *decided.Evaluation.FinalW)
	}
	do(t, h, "POST", evPath+"/appeals", []byte(`{"criterion_id":"01","comment":"не представился"}`), &ap)
	do(t, h, "POST", "/api/appeals/"+itoa(ap.ID)+"/decide", []byte(`{"decision":"changed","new_answer":"NO","reason":"имени нет"}`), &decided)
	if *decided.Evaluation.FinalTotal != 86.32 {
		t.Fatalf("two changes: %v", *decided.Evaluation.FinalTotal)
	}
	do(t, h, "POST", evPath+"/appeals", []byte(`{"criterion_id":"05","comment":"x"}`), &ap)
	do(t, h, "POST", "/api/appeals/"+itoa(ap.ID)+"/decide", []byte(`{"decision":"confirmed","reason":"всё верно"}`), &decided)
	if *decided.Evaluation.FinalTotal != 86.32 || decided.Appeal.FinalAnswer != nil {
		t.Fatalf("confirm must not change total: %+v", decided.Appeal)
	}
	var summaries []domain.ConversationSummary
	do(t, h, "GET", "/api/conversations", nil, &summaries)
	for _, c := range summaries {
		if c.ID == ev.ConversationID && (c.LatestTotal == nil || *c.LatestTotal != 86.32) {
			t.Fatalf("registry total after appeal: %+v", c)
		}
	}

	var open []domain.Appeal
	do(t, h, "GET", "/api/appeals?status=open", nil, &open)
	if len(open) != 0 {
		t.Fatalf("open appeals: %d", len(open))
	}

	e.close()
	e2 := newEnv(t, path)
	var reloaded domain.Evaluation
	do(t, e2.h, "GET", evPath, nil, &reloaded)
	if reloaded.FinalTotal == nil || *reloaded.FinalTotal != 86.32 || len(reloaded.Appeals) != 4 || reloaded.Items[0].FinalAnswer == nil || *reloaded.Items[0].FinalAnswer != "NO" {
		t.Fatalf("after restart: final=%v appeals=%d item0=%+v", reloaded.FinalTotal, len(reloaded.Appeals), reloaded.Items[0])
	}
	var versions []store.ChecklistSummary
	do(t, e2.h, "GET", "/api/checklists", nil, &versions)
	if len(versions) != 2 {
		t.Fatalf("versions after restart: %d", len(versions))
	}
}

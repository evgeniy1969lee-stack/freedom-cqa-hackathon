package scoring

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/homepunks/datathong/internal/checklist"
	"github.com/homepunks/datathong/internal/domain"
)

func synthetic() domain.Checklist {
	return domain.Checklist{Criteria: []domain.Criterion{
		{ID: "01", Weight: 10, Type: domain.TypeYesNo},
		{ID: "02", Weight: 10, Type: domain.TypeYesNo, NAAllowed: true},
		{ID: "03", Weight: 20, Type: domain.TypeYesNo, NAAllowed: true},
		{ID: "04", Weight: 20, Type: domain.TypeScale, Steps: steps(0, 5, 10, 15, 20)},
		{ID: "05", Weight: 10, Type: domain.TypeYesNo, NAAllowed: true},
		{ID: "06", Weight: 20, Type: domain.TypeScale, Steps: steps(0, 4, 7, 10)},
		{ID: "07", Weight: 10, Type: domain.TypeYesNo},
	}}
}

func steps(vs ...int) []domain.Step {
	out := make([]domain.Step, len(vs))
	for i, v := range vs {
		out[i] = domain.Step{Value: v}
	}
	return out
}

func answers(t *testing.T, js string) []domain.Answer {
	t.Helper()
	var out []domain.Answer
	if err := json.Unmarshal([]byte(js), &out); err != nil {
		t.Fatalf("parse answers: %v", err)
	}
	return out
}

const seven = `[
 {"criterion_id":"01","answer":"YES"},
 {"criterion_id":"02","answer":"N/A"},
 {"criterion_id":"03","answer":"N/A"},
 {"criterion_id":"04","answer":15},
 {"criterion_id":"05","answer":"YES"},
 {"criterion_id":"06","answer":7},
 {"criterion_id":"07","answer":"NO"}]`

func TestSyntheticTotal(t *testing.T) {

	res, err := Score(synthetic(), answers(t, seven))
	if err != nil {
		t.Fatal(err)
	}
	if res.W != 70 || res.Total == nil || *res.Total != 70 {
		t.Fatalf("got W=%v total=%v, want W=70 total=70", res.W, deref(res.Total))
	}
	if n := len(res.Items); n != 7 {
		t.Fatalf("items = %d, want 7", n)
	}
	if res.Items[1].Applicable || res.Items[2].Applicable {
		t.Fatal("N/A items must not be applicable")
	}
}

func TestRounding(t *testing.T) {
	cl := domain.Checklist{Criteria: []domain.Criterion{
		{ID: "a", Weight: 1, Type: domain.TypeYesNo},
		{ID: "b", Weight: 1, Type: domain.TypeYesNo},
		{ID: "c", Weight: 1, Type: domain.TypeYesNo},
	}}
	res, err := Score(cl, answers(t, `[{"criterion_id":"a","answer":"YES"},{"criterion_id":"b","answer":"NO"},{"criterion_id":"c","answer":"NO"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if *res.Total != 33.33 {
		t.Fatalf("total = %v, want 33.33", *res.Total)
	}
}

func TestAllNAHasNoTotal(t *testing.T) {
	cl := domain.Checklist{Criteria: []domain.Criterion{
		{ID: "a", Weight: 5, Type: domain.TypeYesNo, NAAllowed: true},
		{ID: "b", Weight: 5, Type: domain.TypeScale, Steps: steps(0, 10), NAAllowed: true},
	}}
	res, err := Score(cl, answers(t, `[{"criterion_id":"a","answer":"N/A"},{"criterion_id":"b","answer":"N/A"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if res.W != 0 || res.Total != nil {
		t.Fatalf("W=%v total=%v, want W=0 and no total", res.W, deref(res.Total))
	}
}

func TestZeroWeightCriterionAppliesButAddsNothing(t *testing.T) {
	cl := domain.Checklist{Criteria: []domain.Criterion{
		{ID: "a", Weight: 0, Type: domain.TypeYesNo},
	}}
	res, err := Score(cl, answers(t, `[{"criterion_id":"a","answer":"YES"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != nil {
		t.Fatalf("W=0 must give no total, got %v", *res.Total)
	}
}

func TestBoundaryScaleSteps(t *testing.T) {
	cl := domain.Checklist{Criteria: []domain.Criterion{
		{ID: "s", Weight: 1, Type: domain.TypeScale, Steps: steps(0, 4, 7, 10)},
	}}
	for step, want := range map[int]float64{0: 0, 4: 40, 7: 70, 10: 100} {
		res, err := Score(cl, []domain.Answer{{CriterionID: "s", Value: domain.AnswerValue{Kind: domain.AnswerStep, Step: step}}})
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		if *res.Total != want {
			t.Fatalf("step %d: total %v, want %v", step, *res.Total, want)
		}
	}
}

func TestInvalidAnswers(t *testing.T) {
	cases := map[string]string{
		"forbidden step":   `[{"criterion_id":"04","answer":12}]`,
		"step above max":   `[{"criterion_id":"04","answer":25}]`,
		"yes on scale":     `[{"criterion_id":"04","answer":"YES"}]`,
		"step on yes/no":   `[{"criterion_id":"01","answer":1}]`,
		"N/A not allowed":  `[{"criterion_id":"01","answer":"N/A"}]`,
		"unknown id":       `[{"criterion_id":"99","answer":"YES"}]`,
		"duplicate answer": `[{"criterion_id":"01","answer":"YES"},{"criterion_id":"01","answer":"NO"}]`,
	}
	for name, js := range cases {
		full := fill(t, js)
		_, err := Score(synthetic(), full)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}

	_, err := Score(synthetic(), answers(t, `[{"criterion_id":"01","answer":"YES"}]`))
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("missing: err = %v, want ErrInvalid", err)
	}
}

func fill(t *testing.T, override string) []domain.Answer {
	base := answers(t, seven)
	over := answers(t, override)
	out := []domain.Answer{}
	replaced := map[string]bool{}
	for _, o := range over {
		if replaced[o.CriterionID] {
			out = append(out, o)
			continue
		}
		replaced[o.CriterionID] = true
		out = append(out, o)
	}
	for _, b := range base {
		if !replaced[b.CriterionID] {
			out = append(out, b)
		}
	}
	return out
}

func TestAnswerValueJSON(t *testing.T) {
	var a domain.Answer
	for _, js := range []string{`{"criterion_id":"x","answer":"yes"}`, `{"criterion_id":"x","answer":"Да"}`} {
		if err := json.Unmarshal([]byte(js), &a); err != nil || a.Value.Kind != domain.AnswerYes {
			t.Fatalf("%s: kind=%v err=%v", js, a.Value.Kind, err)
		}
	}
	if err := json.Unmarshal([]byte(`{"criterion_id":"x","answer":"15"}`), &a); err != nil || a.Value.Step != 15 {
		t.Fatalf("numeric string: %v %v", a.Value, err)
	}
	if err := json.Unmarshal([]byte(`{"criterion_id":"x","answer":"maybe"}`), &a); err == nil {
		t.Fatal("expected error for 'maybe'")
	}
	b, _ := json.Marshal(domain.Answer{CriterionID: "x", Value: domain.AnswerValue{Kind: domain.AnswerStep, Step: 7}})
	if string(b) != `{"criterion_id":"x","answer":7}` {
		t.Fatalf("marshal = %s", b)
	}
}

func TestOfficialExample(t *testing.T) {
	cl, err := checklist.Seed()
	if err != nil {
		t.Fatal(err)
	}
	if cl.Draft {
		t.Skip("seed checklist is a draft: fill weights from the Checklist PDF and set draft=false")
	}
	res, err := Score(cl, answers(t, seven))
	if err != nil {
		t.Fatal(err)
	}
	if res.Total == nil || *res.Total != 77.69 {
		t.Fatalf("total = %v, want 77.69", deref(res.Total))
	}
	flipped := answers(t, seven)
	flipped[6].Value = domain.AnswerValue{Kind: domain.AnswerYes}
	res, err = Score(cl, flipped)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total == nil || *res.Total != 85.38 {
		t.Fatalf("flipped total = %v, want 85.38", deref(res.Total))
	}
}

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

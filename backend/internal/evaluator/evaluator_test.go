package evaluator

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/homepunks/datathong/internal/domain"
)

func conv(n int) domain.Conversation {
	c := domain.Conversation{ID: "c"}
	for i := 0; i < n; i++ {
		c.Turns = append(c.Turns, domain.Turn{TurnID: i, Speaker: "operator", Text: "x"})
	}
	return c
}

func cl() domain.Checklist {
	return domain.Checklist{Criteria: []domain.Criterion{
		{ID: "01", Weight: 1, Type: domain.TypeYesNo},
		{ID: "04", Weight: 1, Type: domain.TypeScale, Steps: []domain.Step{{Value: 0}, {Value: 10}}},
	}}
}

func resp(js string, t *testing.T) domain.MLResponse {
	t.Helper()
	var r domain.MLResponse
	if err := json.Unmarshal([]byte(js), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestValidateOK(t *testing.T) {
	r := resp(`{"model":"m","prompt_version":"p","results":[
	 {"criterion_id":"01","answer":"YES","reason":"r","evidence":{"kind":"turns","turn_ids":[0,3]}},
	 {"criterion_id":"04","answer":10,"reason":"r","evidence":{"kind":"range","from":0,"to":3}}],
	 "gross_error":{"flag":false,"evidence":{"kind":"none"}}}`, t)
	res, err := Validate(conv(4), cl(), r)
	if err != nil {
		t.Fatal(err)
	}
	if *res.Total != 100 {
		t.Fatalf("total %v", *res.Total)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]string{
		"turn out of range": `{"results":[{"criterion_id":"01","answer":"YES","evidence":{"kind":"turns","turn_ids":[4]}},{"criterion_id":"04","answer":10,"evidence":{}}]}`,
		"negative turn":     `{"results":[{"criterion_id":"01","answer":"YES","evidence":{"kind":"turns","turn_ids":[-1]}},{"criterion_id":"04","answer":10,"evidence":{}}]}`,
		"reversed range":    `{"results":[{"criterion_id":"01","answer":"YES","evidence":{"kind":"range","from":3,"to":1}},{"criterion_id":"04","answer":10,"evidence":{}}]}`,
		"range past end":    `{"results":[{"criterion_id":"01","answer":"YES","evidence":{"kind":"range","from":0,"to":4}},{"criterion_id":"04","answer":10,"evidence":{}}]}`,
		"unknown kind":      `{"results":[{"criterion_id":"01","answer":"YES","evidence":{"kind":"quote"}},{"criterion_id":"04","answer":10,"evidence":{}}]}`,
		"missing criterion": `{"results":[{"criterion_id":"01","answer":"YES","evidence":{}}]}`,
		"bad step":          `{"results":[{"criterion_id":"01","answer":"YES","evidence":{}},{"criterion_id":"04","answer":7,"evidence":{}}]}`,
		"gross evidence":    `{"results":[{"criterion_id":"01","answer":"YES","evidence":{}},{"criterion_id":"04","answer":10,"evidence":{}}],"gross_error":{"flag":true,"evidence":{"kind":"turns","turn_ids":[9]}}}`,
	}
	for name, js := range cases {
		_, err := Validate(conv(4), cl(), resp(js, t))
		if !errors.Is(err, ErrInvalidResponse) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestHTTPEvaluator(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Conversation.ID != "c" || len(req.Checklist.Criteria) != 2 {
			http.Error(w, "bad request", 400)
			return
		}
		w.Write([]byte(`{"model":"m","prompt_version":"p","results":[]}`))
	}))
	defer srv.Close()
	out, raw, err := HTTP{URL: srv.URL, Client: srv.Client()}.Evaluate(context.Background(), Request{Conversation: conv(2), Checklist: cl()})
	if err != nil || out.Model != "m" || len(raw) == 0 {
		t.Fatalf("out=%+v raw=%s err=%v", out, raw, err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model crashed", 500)
	}))
	defer bad.Close()
	_, _, err = HTTP{URL: bad.URL, Client: bad.Client()}.Evaluate(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error on 500")
	}
	garbage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer garbage.Close()
	_, _, err = HTTP{URL: garbage.URL, Client: garbage.Client()}.Evaluate(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error on garbage")
	}
}

func TestStubIsValid(t *testing.T) {
	c := conv(3)
	out, _, err := Stub{}.Evaluate(context.Background(), Request{Conversation: c, Checklist: cl()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(c, cl(), out); err != nil {
		t.Fatal(err)
	}
}

package httpapi

import (
	"encoding/csv"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/homepunks/datathong/internal/export"
)

func TestExport(t *testing.T) {
	e := newEnv(t, filepath.Join(t.TempDir(), "x.db"))
	h := e.h
	records, _ := os.ReadFile("../../testdata/example_records.json")
	do(t, h, "POST", "/api/import", records, nil)
	var batch struct {
		Jobs []struct {
			Job struct{ ID int64 } `json:"job"`
		} `json:"jobs"`
	}
	do(t, h, "POST", "/api/evaluate/batch", []byte(`{}`), &batch)
	for _, j := range batch.Jobs {
		e.waitCompleted(j.Job.ID)
	}
	var res struct {
		Job struct{ ID int64 } `json:"job"`
	}
	do(t, h, "POST", "/api/conversations/example-001/evaluate", []byte(`{"force_new":true}`), &res)
	e.waitCompleted(res.Job.ID)

	var doc export.Document
	if code := do(t, h, "GET", "/api/export", nil, &doc); code != 200 || doc.Count != 2 {
		t.Fatalf("latest export: %d %+v", code, doc)
	}
	for _, r := range doc.Records {
		if r.ConversationID == "example-001" && r.EvaluationID != 3 {
			t.Fatalf("latest must pick the newest evaluation, got %d", r.EvaluationID)
		}
		if len(r.Items) != 7 || r.Items[0].Name == "" {
			t.Fatalf("items: %+v", r.Items)
		}
	}
	if code := do(t, h, "GET", "/api/export?all=true", nil, &doc); code != 200 || doc.Count != 3 {
		t.Fatalf("all export: %d %d", code, doc.Count)
	}
	if code := do(t, h, "GET", "/api/export?conversation_id=example-002", nil, &doc); code != 200 || doc.Count != 1 {
		t.Fatalf("filtered export: %d %d", code, doc.Count)
	}
	if code := do(t, h, "GET", "/api/export?format=xml", nil, nil); code != 400 {
		t.Fatalf("bad format: %d", code)
	}

	req := httptest.NewRequest("GET", "/api/export?format=csv", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("csv: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	rows, err := csv.NewReader(rec.Body).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1+2*7 || rows[0][0] != "conversation_id" || rows[1][11] != "01" {
		t.Fatalf("csv rows: %d first=%v", len(rows), rows[1])
	}
}

func TestDevTarget(t *testing.T) {
	got, _ := devTarget("https://h/x?k=v", "")
	if got != "https://h/x?k=v" {
		t.Fatal(got)
	}
	got, _ = devTarget("https://h/groups/{group_id}/data?k=v", "g 1")
	if got != "https://h/groups/g%201/data?k=v" {
		t.Fatal(got)
	}
	got, _ = devTarget("https://h/x?k=v", "g1")
	if got != "https://h/x?group_id=g1&k=v" {
		t.Fatal(got)
	}
}

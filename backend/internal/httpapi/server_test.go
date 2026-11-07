package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/homepunks/datathong/internal/checklist"
	"github.com/homepunks/datathong/internal/domain"
	"github.com/homepunks/datathong/internal/store"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	seed, err := checklist.Seed()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureSeedChecklist(ctx, seed); err != nil {
		t.Fatal(err)
	}
	return New(st, "", "", slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func do(t *testing.T, h http.Handler, method, path string, body []byte, out any) int {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: decode %q: %v", method, path, rec.Body.String(), err)
		}
	}
	return rec.Code
}

func TestImportListGet(t *testing.T) {
	h := newTestServer(t)
	records, _ := os.ReadFile("../../testdata/example_records.json")

	var imp struct {
		Imported []struct {
			ConversationID string `json:"conversation_id"`
			Status         string `json:"status"`
			TurnCount      int    `json:"turn_count"`
		} `json:"imported"`
	}
	if code := do(t, h, "POST", "/api/import", records, &imp); code != 200 {
		t.Fatalf("import: status %d", code)
	}
	if len(imp.Imported) != 2 || imp.Imported[0].Status != "created" || imp.Imported[0].TurnCount != 8 {
		t.Fatalf("import result: %+v", imp.Imported)
	}

	do(t, h, "POST", "/api/import", records, &imp)
	if imp.Imported[0].Status != "unchanged" || imp.Imported[1].Status != "unchanged" {
		t.Fatalf("re-import: %+v", imp.Imported)
	}

	single, _ := os.ReadFile("../../testdata/example_conversation.json")
	changed := bytes.Replace(single, []byte("Хорошего дня."), []byte("До свидания."), 1)
	do(t, h, "POST", "/api/import", changed, &imp)
	if imp.Imported[0].Status != "updated" {
		t.Fatalf("changed import: %+v", imp.Imported)
	}

	var list []domain.ConversationSummary
	if code := do(t, h, "GET", "/api/conversations", nil, &list); code != 200 || len(list) != 2 {
		t.Fatalf("list: code %d len %d", code, len(list))
	}
	if list[0].ID != "example-001" {
		t.Fatalf("list order: %s first", list[0].ID)
	}

	var c domain.Conversation
	if code := do(t, h, "GET", "/api/conversations/example-001", nil, &c); code != 200 {
		t.Fatalf("get: %d", code)
	}
	if len(c.Turns) != 8 || c.Turns[7].Text != "Пожалуйста! До свидания." || c.Turns[3].TurnID != 3 {
		t.Fatalf("get turns: %+v", c.Turns)
	}
	if code := do(t, h, "GET", "/api/conversations/nope", nil, nil); code != 404 {
		t.Fatalf("missing: %d", code)
	}
}

func TestImportErrors(t *testing.T) {
	h := newTestServer(t)
	if code := do(t, h, "POST", "/api/import", []byte(`{"conversation_id":"x"}`), nil); code != 400 {
		t.Fatalf("bad input: %d", code)
	}
	var e map[string]string
	if code := do(t, h, "POST", "/api/import/dev", nil, &e); code != 503 || e["error"] == "" {
		t.Fatalf("dev without url: %d %v", code, e)
	}
}

func TestChecklists(t *testing.T) {
	h := newTestServer(t)
	var list []store.ChecklistSummary
	if code := do(t, h, "GET", "/api/checklists", nil, &list); code != 200 || len(list) != 1 || list[0].Version != 1 {
		t.Fatalf("list: %d %+v", code, list)
	}
	var cl domain.Checklist
	if code := do(t, h, "GET", "/api/checklists/latest", nil, &cl); code != 200 || cl.Version != 1 || len(cl.Criteria) != 7 {
		t.Fatalf("latest: %d %+v", code, cl)
	}
	if code := do(t, h, "GET", "/api/checklists/1", nil, &cl); code != 200 {
		t.Fatalf("v1: %d", code)
	}
	if code := do(t, h, "GET", "/api/checklists/9", nil, nil); code != 404 {
		t.Fatalf("v9: %d", code)
	}
	if code := do(t, h, "GET", "/api/checklists/zero", nil, nil); code != 400 {
		t.Fatalf("bad version: %d", code)
	}
}

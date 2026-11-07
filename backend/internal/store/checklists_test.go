package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/homepunks/datathong/internal/checklist"
)

func TestEnsureSeedAppendsOfficialVersionAfterLegacyDraft(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "checklists.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	seed, err := checklist.Seed()
	if err != nil {
		t.Fatal(err)
	}
	draft := seed
	draft.Draft = true
	draft.Note = "Freedom CQA checklist, starting version. DRAFT: structure follows the team plan; weights and N/A rules must be confirmed against CQA_Chat_Quality_Checklist PDF, then set draft=false."
	if _, err := st.SaveChecklist(ctx, draft); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := st.EnsureSeedChecklist(ctx, seed); err != nil {
			t.Fatal(err)
		}
	}
	current, err := st.GetChecklist(ctx, 0)
	if err != nil || current.Version != 2 || current.Draft || current.Note != seed.Note {
		t.Fatalf("latest should be the official version 2: %+v, %v", current, err)
	}
	old, err := st.GetChecklist(ctx, 1)
	if err != nil || !old.Draft || old.Note != draft.Note {
		t.Fatalf("the original draft must remain available: %+v, %v", old, err)
	}
}

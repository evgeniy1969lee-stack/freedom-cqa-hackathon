package store

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/homepunks/datathong/internal/checklist"
)

func TestEnsureSeedAddsSectionsOnlyToStockChecklist(t *testing.T) {
	for _, custom := range []bool{false, true} {
		name := "stock"
		if custom {
			name = "custom"
		}
		t.Run(name, func(t *testing.T) {
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
			old := seed
			old.Criteria = slices.Clone(seed.Criteria)
			for i := range old.Criteria {
				old.Criteria[i].Section = ""
			}
			if custom {
				old.Criteria[0].Weight++
			}
			if _, err := st.SaveChecklist(ctx, old); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := st.EnsureSeedChecklist(ctx, seed); err != nil {
					t.Fatal(err)
				}
			}
			current, err := st.GetChecklist(ctx, 0)
			if err != nil {
				t.Fatal(err)
			}
			if custom {
				if current.Version != 1 || current.Criteria[0].Weight != old.Criteria[0].Weight {
					t.Fatal("custom checklist must remain active")
				}
			} else if current.Version != 2 || current.Criteria[0].Section == "" {
				t.Fatal("stock checklist must get exactly one new version with sections")
			}
			historical, err := st.GetChecklist(ctx, 1)
			if err != nil || historical.Criteria[0].Section != "" {
				t.Fatalf("the historical version must be preserved: %v", err)
			}
		})
	}
}

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

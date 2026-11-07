package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/homepunks/datathong/internal/export"
)

func (s *Server) exportEvaluations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := q.Get("format")
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" {
		writeError(w, http.StatusBadRequest, "format must be json or csv")
		return
	}
	var version int
	if v := q.Get("checklist_version"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "checklist_version must be a positive integer")
			return
		}
		version = n
	}
	all := q.Get("all") == "true"
	ids, err := s.store.ExportEvaluationIDs(r.Context(), q.Get("conversation_id"), version, all)
	if err != nil {
		s.fail(w, err)
		return
	}
	records := make([]export.Record, 0, len(ids))
	for _, id := range ids {
		ev, err := s.store.GetEvaluation(r.Context(), id)
		if err != nil {
			s.fail(w, err)
			return
		}
		conv, err := s.store.GetConversation(r.Context(), ev.ConversationID)
		if err != nil {
			s.fail(w, err)
			return
		}
		records = append(records, export.Build(conv, ev))
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="cqa-export-`+stamp+`.csv"`)
		if err := export.WriteCSV(w, records); err != nil {
			s.log.Error("export csv", "err", err)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="cqa-export-`+stamp+`.json"`)
	if err := export.WriteJSON(w, records); err != nil {
		s.log.Error("export json", "err", err)
	}
}

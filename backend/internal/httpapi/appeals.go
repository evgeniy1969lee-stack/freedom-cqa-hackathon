package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/homepunks/datathong/internal/domain"
	"github.com/homepunks/datathong/internal/scoring"
	"github.com/homepunks/datathong/internal/store"
)

func (s *Server) createChecklist(w http.ResponseWriter, r *http.Request) {
	var cl domain.Checklist
	if err := decodeBody(r, &cl); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := cl.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, c := range cl.Criteria {
		if strings.TrimSpace(c.Name) == "" {
			writeError(w, http.StatusBadRequest, "criterion "+c.ID+": name is required")
			return
		}
	}
	saved, err := s.store.SaveChecklist(r.Context(), cl)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

type appealRequest struct {
	CriterionID string `json:"criterion_id"`
	Comment     string `json:"comment"`
}

func (s *Server) createAppeal(w http.ResponseWriter, r *http.Request) {
	evID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "evaluation id must be an integer")
		return
	}
	var req appealRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.CriterionID = strings.TrimSpace(req.CriterionID)
	req.Comment = strings.TrimSpace(req.Comment)
	if req.CriterionID == "" || req.Comment == "" {
		writeError(w, http.StatusBadRequest, "criterion_id and comment are required")
		return
	}
	a, err := s.store.CreateAppeal(r.Context(), evID, req.CriterionID, req.Comment)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "evaluation not found")
	case errors.Is(err, store.ErrUnknownCriterion):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrAppealOpenExists):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		s.fail(w, err)
	default:
		writeJSON(w, http.StatusCreated, a)
	}
}

type decideRequest struct {
	Decision  domain.Decision     `json:"decision"`
	NewAnswer *domain.AnswerValue `json:"new_answer"`
	Reason    string              `json:"reason"`
}

func (s *Server) decideAppeal(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "appeal id must be an integer")
		return
	}
	var req decideRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		writeError(w, http.StatusBadRequest, "reason is required")
		return
	}
	switch req.Decision {
	case domain.DecisionConfirmed:
		req.NewAnswer = nil
	case domain.DecisionChanged:
		if req.NewAnswer == nil {
			writeError(w, http.StatusBadRequest, "new_answer is required when decision is changed")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "decision must be confirmed or changed")
		return
	}
	a, ev, err := s.store.DecideAppeal(r.Context(), id, req.Decision, req.NewAnswer, req.Reason)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "appeal not found")
	case errors.Is(err, store.ErrAppealResolved):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, scoring.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		s.fail(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"appeal": a, "evaluation": ev})
	}
}

func (s *Server) getAppeal(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "appeal id must be an integer")
		return
	}
	a, err := s.store.GetAppeal(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "appeal not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) listAppeals(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var evID int64
	if v := q.Get("evaluation_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "evaluation_id must be an integer")
			return
		}
		evID = n
	}
	list, err := s.store.ListAppeals(r.Context(), evID, q.Get("status"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

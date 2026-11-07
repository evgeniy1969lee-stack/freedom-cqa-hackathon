package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/homepunks/datathong/internal/domain"
	"github.com/homepunks/datathong/internal/store"
)

type evaluateRequest struct {
	ChecklistVersion int      `json:"checklist_version"`
	ForceNew         bool     `json:"force_new"`
	ConversationIDs  []string `json:"conversation_ids"`
}

type jobResult struct {
	ConversationID string      `json:"conversation_id"`
	Job            *domain.Job `json:"job"`
	Created        bool        `json:"created"`
	Error          string      `json:"error,omitempty"`
}

func (s *Server) evaluateOne(w http.ResponseWriter, r *http.Request) {
	var req evaluateRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cl, err := s.store.GetChecklist(r.Context(), req.ChecklistVersion)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "checklist version not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	res := s.enqueue(r, r.PathValue("id"), cl, req.ForceNew)
	switch {
	case res.Error != "" && res.Job == nil && errors.Is(res.err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, res.Error)
	case res.Error != "" && res.Job == nil:
		writeError(w, http.StatusConflict, res.Error)
	case res.Created:
		writeJSON(w, http.StatusAccepted, res)
	default:
		writeJSON(w, http.StatusOK, res)
	}
}

func (s *Server) evaluateBatch(w http.ResponseWriter, r *http.Request) {
	var req evaluateRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cl, err := s.store.GetChecklist(r.Context(), req.ChecklistVersion)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "checklist version not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	ids := req.ConversationIDs
	if len(ids) == 0 {
		list, err := s.store.ListConversations(r.Context())
		if err != nil {
			s.fail(w, err)
			return
		}
		for _, c := range list {
			ids = append(ids, c.ID)
		}
	}
	out := make([]jobResult, 0, len(ids))
	created := 0
	for _, id := range ids {
		res := s.enqueue(r, id, cl, req.ForceNew)
		if res.Created {
			created++
		}
		out = append(out, res.jobResult)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"checklist_version": cl.Version, "created": created, "jobs": out})
}

type enqueueResult struct {
	jobResult
	err error
}

func (s *Server) enqueue(r *http.Request, convID string, cl domain.Checklist, forceNew bool) enqueueResult {
	res := enqueueResult{jobResult: jobResult{ConversationID: convID}}
	conv, err := s.store.GetConversation(r.Context(), convID)
	if errors.Is(err, store.ErrNotFound) {
		res.Error, res.err = "conversation not found", err
		return res
	}
	if err != nil {
		s.log.Error("enqueue", "conversation", convID, "err", err)
		res.Error, res.err = "internal error", err
		return res
	}
	job, created, err := s.store.CreateJob(r.Context(), conv, cl, s.maxAttempts, forceNew)
	if err != nil {
		res.Error, res.err = err.Error(), err
		return res
	}
	res.Job, res.Created = &job, created
	if created && s.worker != nil {
		s.worker.Kick()
	}
	return res
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	jobs, err := s.store.ListJobs(r.Context(), q.Get("conversation_id"), q.Get("status"))
	if err != nil {
		s.fail(w, err)
		return
	}
	counts, err := s.store.JobCounts(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"counts": counts, "jobs": jobs})
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "job id must be an integer")
		return
	}
	job, err := s.store.GetJob(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) retryJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "job id must be an integer")
		return
	}
	job, err := s.store.RetryJob(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "job not found")
		return
	case errors.Is(err, store.ErrJobNotFailed):
		writeError(w, http.StatusConflict, "only failed jobs can be retried")
		return
	case err != nil:
		s.fail(w, err)
		return
	}
	if s.worker != nil {
		s.worker.Kick()
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) getEvaluation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "evaluation id must be an integer")
		return
	}
	ev, err := s.store.GetEvaluation(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "evaluation not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

func (s *Server) listEvaluations(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.store.GetConversation(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	} else if err != nil {
		s.fail(w, err)
		return
	}
	list, err := s.store.ListEvaluations(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func decodeBody(r *http.Request, v any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errors.New("invalid JSON body: " + err.Error())
	}
	return nil
}

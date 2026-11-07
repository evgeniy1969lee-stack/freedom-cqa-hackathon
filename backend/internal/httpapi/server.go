package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/homepunks/datathong/internal/importer"
	"github.com/homepunks/datathong/internal/queue"
	"github.com/homepunks/datathong/internal/store"
)

const maxBody = 64 << 20

type Server struct {
	store       *store.Store
	mlName      string
	mlReadiness func(context.Context) (bool, string)
	worker      *queue.Worker
	maxAttempts int
	devURL      string
	client      *http.Client
	log         *slog.Logger
}

type Options struct {
	MLName      string
	MLReadiness func(context.Context) (bool, string)
	DevURL      string
	StaticDir   string
	Worker      *queue.Worker
	MaxAttempts int
}

func New(st *store.Store, opt Options, log *slog.Logger) http.Handler {
	if opt.MaxAttempts < 1 {
		opt.MaxAttempts = 1
	}
	s := &Server{store: st, mlName: opt.MLName, mlReadiness: opt.MLReadiness, worker: opt.Worker, maxAttempts: opt.MaxAttempts, devURL: opt.DevURL, client: &http.Client{Timeout: 60 * time.Second}, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("POST /api/import", s.importBody)
	mux.HandleFunc("POST /api/import/dev", s.importDev)
	mux.HandleFunc("GET /api/conversations", s.listConversations)
	mux.HandleFunc("GET /api/conversations/{id}", s.getConversation)
	mux.HandleFunc("GET /api/checklists", s.listChecklists)
	mux.HandleFunc("GET /api/checklists/{version}", s.getChecklist)
	mux.HandleFunc("POST /api/conversations/{id}/evaluate", s.evaluateOne)
	mux.HandleFunc("GET /api/conversations/{id}/evaluations", s.listEvaluations)
	mux.HandleFunc("POST /api/evaluate/batch", s.evaluateBatch)
	mux.HandleFunc("GET /api/jobs", s.listJobs)
	mux.HandleFunc("GET /api/jobs/{id}", s.getJob)
	mux.HandleFunc("POST /api/jobs/{id}/retry", s.retryJob)
	mux.HandleFunc("GET /api/evaluations/{id}", s.getEvaluation)
	mux.HandleFunc("POST /api/checklists", s.createChecklist)
	mux.HandleFunc("POST /api/evaluations/{id}/appeals", s.createAppeal)
	mux.HandleFunc("GET /api/appeals", s.listAppeals)
	mux.HandleFunc("GET /api/appeals/{id}", s.getAppeal)
	mux.HandleFunc("POST /api/appeals/{id}/decide", s.decideAppeal)
	mux.HandleFunc("GET /api/export", s.exportEvaluations)
	if opt.StaticDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(opt.StaticDir)))
	}
	return logRequests(log, sameOriginWrites(mux))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	counts, err := s.store.JobCounts(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	ready, model := false, ""
	if s.mlReadiness != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		ready, model = s.mlReadiness(ctx)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "time": time.Now().UTC(), "ml": s.mlName, "ml_ready": ready, "ml_model": model,
		"dev_url_configured": s.devURL != "", "jobs": counts,
	})
}

type importResult struct {
	ConversationID string             `json:"conversation_id"`
	Status         store.UpsertStatus `json:"status"`
	SourceHash     string             `json:"source_hash"`
	TurnCount      int                `json:"turn_count"`
}

func (s *Server) importBody(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	s.runImport(w, r.Context(), body)
}

func (s *Server) importDev(w http.ResponseWriter, r *http.Request) {
	if s.devURL == "" {
		writeError(w, http.StatusServiceUnavailable, "DEV URL is not configured (set CQA_DEV_URL)")
		return
	}
	target, err := devTarget(s.devURL, r.URL.Query().Get("group_id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DEV URL is malformed")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DEV URL is malformed")
		return
	}
	resp, err := s.client.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "DEV fetch failed: "+stripURL(err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("DEV fetch failed: status %d", resp.StatusCode))
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		writeError(w, http.StatusBadGateway, "DEV fetch failed: "+stripURL(err))
		return
	}
	s.runImport(w, r.Context(), body)
}

func (s *Server) runImport(w http.ResponseWriter, ctx context.Context, body []byte) {
	convs, err := importer.Parse(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	results := make([]importResult, 0, len(convs))
	for _, c := range convs {
		status, err := s.store.UpsertConversation(ctx, c)
		if err != nil {
			s.log.Error("import", "conversation_id", c.ID, "err", err)
			writeError(w, http.StatusInternalServerError, "store failed for "+c.ID)
			return
		}
		results = append(results, importResult{ConversationID: c.ID, Status: status, SourceHash: c.SourceHash, TurnCount: len(c.Turns)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": results})
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListConversations(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request) {
	c, err := s.store.GetConversation(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) listChecklists(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListChecklists(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getChecklist(w http.ResponseWriter, r *http.Request) {
	var version int
	if v := r.PathValue("version"); v != "latest" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "version must be a positive integer or 'latest'")
			return
		}
		version = n
	}
	cl, err := s.store.GetChecklist(r.Context(), version)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "checklist version not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cl)
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	s.log.Error("request failed", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func devTarget(base, groupID string) (string, error) {
	if groupID == "" {
		return base, nil
	}
	if strings.Contains(base, "{group_id}") {
		return strings.ReplaceAll(base, "{group_id}", url.PathEscape(groupID)), nil
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("group_id", groupID)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func stripURL(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

func sameOriginWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			raw := r.Header.Get("Origin")
			origin, err := url.Parse(raw)
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" || (raw != "" &&
				(err != nil || !strings.EqualFold(origin.Host, r.Host) || (origin.Scheme != "https" && origin.Scheme != "http"))) {
				writeError(w, http.StatusForbidden, "cross-origin write forbidden")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Info("http", "method", r.Method, "path", r.URL.Path, "ms", time.Since(start).Milliseconds())
	})
}

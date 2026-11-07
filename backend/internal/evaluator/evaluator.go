package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/homepunks/datathong/internal/domain"
	"github.com/homepunks/datathong/internal/scoring"
)

type Request struct {
	Conversation domain.Conversation `json:"conversation"`
	Checklist    domain.Checklist    `json:"checklist"`
}

type Evaluator interface {
	Evaluate(ctx context.Context, req Request) (domain.MLResponse, []byte, error)
}

type HTTP struct {
	URL    string
	Client *http.Client
}

// Readiness checks the bridge without submitting an evaluation.
func (h HTTP) Readiness(ctx context.Context) (ready bool, model string) {
	endpoint, err := url.Parse(h.URL)
	if err != nil {
		return false, ""
	}
	endpoint = endpoint.ResolveReference(&url.URL{Path: "health"})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return false, ""
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()
	var status struct {
		Ready bool   `json:"ready"`
		Model string `json:"model"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&status) != nil {
		return false, ""
	}
	return resp.StatusCode == http.StatusOK && status.Ready, status.Model
}

func (h HTTP) Evaluate(ctx context.Context, req Request) (domain.MLResponse, []byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return domain.MLResponse{}, nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return domain.MLResponse{}, nil, err
	}
	hr.Header.Set("Content-Type", "application/json")
	resp, err := h.Client.Do(hr)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return domain.MLResponse{}, nil, fmt.Errorf("ml request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return domain.MLResponse{}, nil, fmt.Errorf("ml response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return domain.MLResponse{}, raw, fmt.Errorf("ml service returned %d: %s", resp.StatusCode, trim(raw, 300))
	}
	var out domain.MLResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return domain.MLResponse{}, raw, fmt.Errorf("ml response is not valid JSON: %v: %s", err, trim(raw, 300))
	}
	return out, raw, nil
}

type Stub struct{}

func (Stub) Evaluate(_ context.Context, req Request) (domain.MLResponse, []byte, error) {
	n := len(req.Conversation.Turns)
	from, to := 0, n-1
	ev := domain.Evidence{Kind: "range", From: &from, To: &to}
	out := domain.MLResponse{Model: "stub", PromptVersion: "stub", GrossError: &domain.GrossError{Evidence: ev}}
	for _, c := range req.Checklist.Criteria {
		item := domain.MLItem{CriterionID: c.ID, Reason: "STUB RESULT: ML service is not configured", Evidence: ev}
		if c.Type == domain.TypeScale {
			item.Answer = domain.AnswerValue{Kind: domain.AnswerStep, Step: c.Steps[len(c.Steps)/2].Value}
		} else {
			item.Answer = domain.AnswerValue{Kind: domain.AnswerYes}
		}
		out.Results = append(out.Results, item)
	}
	raw, _ := json.Marshal(out)
	return out, raw, nil
}

var ErrInvalidResponse = errors.New("invalid ml response")

func Validate(conv domain.Conversation, cl domain.Checklist, resp domain.MLResponse) (scoring.Result, error) {
	n := len(conv.Turns)
	answers := make([]domain.Answer, 0, len(resp.Results))
	for _, it := range resp.Results {
		if err := checkEvidence(it.Evidence, n); err != nil {
			return scoring.Result{}, fmt.Errorf("%w: criterion %q: %v", ErrInvalidResponse, it.CriterionID, err)
		}
		answers = append(answers, domain.Answer{CriterionID: it.CriterionID, Value: it.Answer})
	}
	if resp.GrossError != nil {
		if err := checkEvidence(resp.GrossError.Evidence, n); err != nil {
			return scoring.Result{}, fmt.Errorf("%w: gross_error: %v", ErrInvalidResponse, err)
		}
	}
	res, err := scoring.Score(cl, answers)
	if err != nil {
		return scoring.Result{}, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}
	return res, nil
}

func checkEvidence(e domain.Evidence, n int) error {
	switch e.Kind {
	case "", "none":
		return nil
	case "turns":
		if len(e.TurnIDs) == 0 {
			return fmt.Errorf("evidence kind turns has no turn_ids")
		}
		for _, id := range e.TurnIDs {
			if id < 0 || id >= n {
				return fmt.Errorf("turn_id %d does not exist (transcript has %d turns)", id, n)
			}
		}
	case "range":
		if e.From == nil || e.To == nil {
			return fmt.Errorf("evidence kind range needs from and to")
		}
		if *e.From < 0 || *e.To >= n {
			return fmt.Errorf("range %d..%d is outside the transcript (0..%d)", *e.From, *e.To, n-1)
		}
		if *e.From > *e.To {
			return fmt.Errorf("range %d..%d is reversed", *e.From, *e.To)
		}
	default:
		return fmt.Errorf("unknown evidence kind %q", e.Kind)
	}
	return nil
}

func trim(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "…"
	}
	return string(b)
}

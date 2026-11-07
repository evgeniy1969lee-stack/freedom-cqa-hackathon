package scoring

import (
	"errors"
	"fmt"
	"math"

	"github.com/homepunks/datathong/internal/domain"
)

type ItemScore struct {
	CriterionID string  `json:"criterion_id"`
	Answer      string  `json:"answer"`
	Weight      float64 `json:"weight"`
	Applicable  bool    `json:"applicable"`
	Q           float64 `json:"q"`
	Points      float64 `json:"points"`
}

type Result struct {
	Total *float64    `json:"total"`
	W     float64     `json:"w"`
	Items []ItemScore `json:"items"`
}

var ErrInvalid = errors.New("invalid answers")

func Score(cl domain.Checklist, answers []domain.Answer) (Result, error) {
	if err := cl.Validate(); err != nil {
		return Result{}, fmt.Errorf("checklist: %w", err)
	}
	byID := make(map[string]domain.Criterion, len(cl.Criteria))
	for _, c := range cl.Criteria {
		byID[c.ID] = c
	}
	got := make(map[string]domain.AnswerValue, len(answers))
	for _, a := range answers {
		if _, ok := byID[a.CriterionID]; !ok {
			return Result{}, fmt.Errorf("%w: unknown criterion %q", ErrInvalid, a.CriterionID)
		}
		if _, dup := got[a.CriterionID]; dup {
			return Result{}, fmt.Errorf("%w: duplicate answer for criterion %q", ErrInvalid, a.CriterionID)
		}
		got[a.CriterionID] = a.Value
	}

	res := Result{Items: make([]ItemScore, 0, len(cl.Criteria))}
	var sum float64
	for _, c := range cl.Criteria {
		v, ok := got[c.ID]
		if !ok {
			return Result{}, fmt.Errorf("%w: missing answer for criterion %q", ErrInvalid, c.ID)
		}
		item := ItemScore{CriterionID: c.ID, Answer: v.String(), Weight: c.Weight}
		q, applicable, err := quotient(c, v)
		if err != nil {
			return Result{}, err
		}
		if applicable {
			item.Applicable = true
			item.Q = q
			item.Points = c.Weight * q
			res.W += c.Weight
			sum += item.Points
		}
		res.Items = append(res.Items, item)
	}
	if res.W > 0 {
		t := round2(100 * sum / res.W)
		res.Total = &t
	}
	return res, nil
}

func quotient(c domain.Criterion, v domain.AnswerValue) (q float64, applicable bool, err error) {
	if v.Kind == domain.AnswerNA {
		if !c.NAAllowed {
			return 0, false, fmt.Errorf("%w: criterion %q does not allow N/A", ErrInvalid, c.ID)
		}
		return 0, false, nil
	}
	switch c.Type {
	case domain.TypeYesNo:
		switch v.Kind {
		case domain.AnswerYes:
			return 1, true, nil
		case domain.AnswerNo:
			return 0, true, nil
		}
		return 0, false, fmt.Errorf("%w: criterion %q expects YES/NO, got %s", ErrInvalid, c.ID, v)
	case domain.TypeScale:
		if v.Kind != domain.AnswerStep {
			return 0, false, fmt.Errorf("%w: criterion %q expects a scale step, got %s", ErrInvalid, c.ID, v)
		}
		for _, s := range c.Steps {
			if s.Value == v.Step {
				return float64(v.Step) / float64(c.MaxStep()), true, nil
			}
		}
		return 0, false, fmt.Errorf("%w: criterion %q: step %d is not allowed", ErrInvalid, c.ID, v.Step)
	}
	return 0, false, fmt.Errorf("checklist: criterion %q has unknown type %q", c.ID, c.Type)
}

func round2(x float64) float64 {
	return math.Round(x*100) / 100
}

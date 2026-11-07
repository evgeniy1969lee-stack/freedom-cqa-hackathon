package domain

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Turn struct {
	TurnID    int    `json:"turn_id"`
	Timestamp string `json:"timestamp"`
	Speaker   string `json:"speaker"`
	Text      string `json:"text"`
}

type Conversation struct {
	ID         string    `json:"conversation_id"`
	Channel    string    `json:"channel"`
	Language   string    `json:"language"`
	AudioURL   *string   `json:"audio_url"`
	Turns      []Turn    `json:"transcript"`
	SourceHash string    `json:"source_hash"`
	ImportedAt time.Time `json:"imported_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ConversationSummary struct {
	ID         string    `json:"conversation_id"`
	Channel    string    `json:"channel"`
	Language   string    `json:"language"`
	AudioURL   *string   `json:"audio_url"`
	TurnCount  int       `json:"turn_count"`
	SourceHash string    `json:"source_hash"`
	ImportedAt time.Time `json:"imported_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

const (
	TypeYesNo = "yes_no"
	TypeScale = "scale"
)

type Step struct {
	Value int    `json:"value"`
	Label string `json:"label"`
}

type Criterion struct {
	ID          string  `json:"criterion_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Weight      float64 `json:"weight"`
	Type        string  `json:"type"`
	Steps       []Step  `json:"steps,omitempty"`
	NAAllowed   bool    `json:"na_allowed"`
	NARule      string  `json:"na_rule,omitempty"`
	Section     string  `json:"section,omitempty"`
}

func (c Criterion) MaxStep() int {
	m := 0
	for _, s := range c.Steps {
		if s.Value > m {
			m = s.Value
		}
	}
	return m
}

type Checklist struct {
	Version              int         `json:"version"`
	CreatedAt            time.Time   `json:"created_at"`
	Note                 string      `json:"note"`
	Draft                bool        `json:"draft,omitempty"`
	GrossErrorCategories []string    `json:"gross_error_categories"`
	Criteria             []Criterion `json:"criteria"`
}

func (cl Checklist) Validate() error {
	if len(cl.Criteria) == 0 {
		return fmt.Errorf("checklist has no criteria")
	}
	seen := map[string]bool{}
	for _, c := range cl.Criteria {
		if strings.TrimSpace(c.ID) == "" {
			return fmt.Errorf("criterion with empty id")
		}
		if seen[c.ID] {
			return fmt.Errorf("duplicate criterion id %q", c.ID)
		}
		seen[c.ID] = true
		if c.Weight < 0 {
			return fmt.Errorf("criterion %s: negative weight", c.ID)
		}
		switch c.Type {
		case TypeYesNo:
			if len(c.Steps) != 0 {
				return fmt.Errorf("criterion %s: yes_no criterion must not have steps", c.ID)
			}
		case TypeScale:
			if len(c.Steps) < 2 {
				return fmt.Errorf("criterion %s: scale needs at least two steps", c.ID)
			}
			vals := map[int]bool{}
			for _, s := range c.Steps {
				if s.Value < 0 {
					return fmt.Errorf("criterion %s: negative step %d", c.ID, s.Value)
				}
				if vals[s.Value] {
					return fmt.Errorf("criterion %s: duplicate step %d", c.ID, s.Value)
				}
				vals[s.Value] = true
			}
			if c.MaxStep() == 0 {
				return fmt.Errorf("criterion %s: max step must be positive", c.ID)
			}
		default:
			return fmt.Errorf("criterion %s: unknown type %q", c.ID, c.Type)
		}
	}
	return nil
}

type AnswerKind int

const (
	AnswerYes AnswerKind = iota + 1
	AnswerNo
	AnswerNA
	AnswerStep
)

type AnswerValue struct {
	Kind AnswerKind
	Step int
}

func (v AnswerValue) String() string {
	switch v.Kind {
	case AnswerYes:
		return "YES"
	case AnswerNo:
		return "NO"
	case AnswerNA:
		return "N/A"
	case AnswerStep:
		return strconv.Itoa(v.Step)
	}
	return "?"
}

func (v AnswerValue) MarshalJSON() ([]byte, error) {
	if v.Kind == AnswerStep {
		return []byte(strconv.Itoa(v.Step)), nil
	}
	if v.Kind == 0 {
		return nil, fmt.Errorf("empty answer value")
	}
	return json.Marshal(v.String())
}

func (v *AnswerValue) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		return v.parse(s)
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("answer must be a string or a number, got %s", string(b))
	}
	return v.parse(n.String())
}

func (v *AnswerValue) parse(s string) error {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "YES", "ДА":
		*v = AnswerValue{Kind: AnswerYes}
	case "NO", "НЕТ":
		*v = AnswerValue{Kind: AnswerNo}
	case "N/A", "NA":
		*v = AnswerValue{Kind: AnswerNA}
	default:
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return fmt.Errorf("invalid answer %q", s)
		}
		*v = AnswerValue{Kind: AnswerStep, Step: n}
	}
	return nil
}

type Answer struct {
	CriterionID string      `json:"criterion_id"`
	Value       AnswerValue `json:"answer"`
}

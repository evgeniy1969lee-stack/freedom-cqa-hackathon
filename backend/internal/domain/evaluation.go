package domain

import "time"

type Evidence struct {
	Kind    string `json:"kind"`
	TurnIDs []int  `json:"turn_ids,omitempty"`
	From    *int   `json:"from,omitempty"`
	To      *int   `json:"to,omitempty"`
}

type MLItem struct {
	CriterionID string      `json:"criterion_id"`
	Answer      AnswerValue `json:"answer"`
	Reason      string      `json:"reason"`
	Evidence    Evidence    `json:"evidence"`
}

type GrossError struct {
	Flag     bool     `json:"flag"`
	Category string   `json:"category,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Evidence Evidence `json:"evidence"`
}

type MLResponse struct {
	Model         string      `json:"model"`
	PromptVersion string      `json:"prompt_version"`
	Results       []MLItem    `json:"results"`
	GrossError    *GrossError `json:"gross_error"`
}

type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
)

type Job struct {
	ID               int64      `json:"id"`
	ConversationID   string     `json:"conversation_id"`
	InputHash        string     `json:"input_hash"`
	ChecklistVersion int        `json:"checklist_version"`
	Status           JobStatus  `json:"status"`
	Attempts         int        `json:"attempts"`
	MaxAttempts      int        `json:"max_attempts"`
	Error            string     `json:"error"`
	Model            string     `json:"model"`
	PromptVersion    string     `json:"prompt_version"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at"`
	EvaluationID     *int64     `json:"evaluation_id"`
	Checklist        Checklist  `json:"-"`
}

type EvaluationItem struct {
	CriterionID string   `json:"criterion_id"`
	Answer      string   `json:"answer"`
	Reason      string   `json:"reason"`
	Evidence    Evidence `json:"evidence"`
	Weight      float64  `json:"weight"`
	Applicable  bool     `json:"applicable"`
	Q           float64  `json:"q"`
	Points      float64  `json:"points"`
}

type Evaluation struct {
	ID               int64            `json:"id"`
	JobID            int64            `json:"job_id"`
	ConversationID   string           `json:"conversation_id"`
	InputHash        string           `json:"input_hash"`
	ChecklistVersion int              `json:"checklist_version"`
	Model            string           `json:"model"`
	PromptVersion    string           `json:"prompt_version"`
	Total            *float64         `json:"total"`
	W                float64          `json:"w"`
	GrossError       GrossError       `json:"gross_error"`
	Items            []EvaluationItem `json:"items"`
	CreatedAt        time.Time        `json:"created_at"`
	Checklist        Checklist        `json:"-"`
	Raw              []byte           `json:"-"`
}

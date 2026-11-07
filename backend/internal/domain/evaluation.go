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
	FinalAnswer *string  `json:"final_answer"`
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
	FinalTotal       *float64         `json:"final_total"`
	FinalW           *float64         `json:"final_w"`
	RevisedAt        *time.Time       `json:"revised_at"`
	Appeals          []Appeal         `json:"appeals"`
	CreatedAt        time.Time        `json:"created_at"`
	Checklist        Checklist        `json:"-"`
	Raw              []byte           `json:"-"`
}

type AppealStatus string

const (
	AppealOpen     AppealStatus = "open"
	AppealResolved AppealStatus = "resolved"
)

type Decision string

const (
	DecisionConfirmed Decision = "confirmed"
	DecisionChanged   Decision = "changed"
)

type Appeal struct {
	ID             int64        `json:"id"`
	EvaluationID   int64        `json:"evaluation_id"`
	CriterionID    string       `json:"criterion_id"`
	Comment        string       `json:"comment"`
	OriginalAnswer string       `json:"original_answer"`
	Status         AppealStatus `json:"status"`
	Decision       Decision     `json:"decision"`
	FinalAnswer    *string      `json:"final_answer"`
	Reason         string       `json:"reason"`
	CreatedAt      time.Time    `json:"created_at"`
	ResolvedAt     *time.Time   `json:"resolved_at"`
}

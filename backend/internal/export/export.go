package export

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/homepunks/datathong/internal/domain"
)

type Item struct {
	CriterionID string          `json:"criterion_id"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Answer      string          `json:"answer"`
	FinalAnswer *string         `json:"final_answer"`
	Weight      float64         `json:"weight"`
	Applicable  bool            `json:"applicable"`
	Q           float64         `json:"q"`
	Points      float64         `json:"points"`
	Reason      string          `json:"reason"`
	Evidence    domain.Evidence `json:"evidence"`
}

type Record struct {
	ConversationID   string            `json:"conversation_id"`
	Channel          string            `json:"channel"`
	Language         string            `json:"language"`
	AudioURL         *string           `json:"audio_url"`
	EvaluationID     int64             `json:"evaluation_id"`
	JobID            int64             `json:"job_id"`
	ChecklistVersion int               `json:"checklist_version"`
	Model            string            `json:"model"`
	PromptVersion    string            `json:"prompt_version"`
	Total            *float64          `json:"total"`
	W                float64           `json:"w"`
	FinalTotal       *float64          `json:"final_total"`
	FinalW           *float64          `json:"final_w"`
	GrossError       domain.GrossError `json:"gross_error"`
	Items            []Item            `json:"items"`
	Appeals          []domain.Appeal   `json:"appeals"`
	CreatedAt        time.Time         `json:"created_at"`
	RevisedAt        *time.Time        `json:"revised_at"`
	InputHash        string            `json:"input_hash"`
	Historical       bool              `json:"historical"`
}

type Document struct {
	ExportedAt time.Time `json:"exported_at"`
	Count      int       `json:"count"`
	Records    []Record  `json:"records"`
}

func Build(conv domain.Conversation, ev domain.Evaluation) Record {
	names := make(map[string]domain.Criterion, len(ev.Checklist.Criteria))
	for _, c := range ev.Checklist.Criteria {
		names[c.ID] = c
	}
	r := Record{
		ConversationID: conv.ID, Channel: conv.Channel, Language: conv.Language, AudioURL: conv.AudioURL,
		EvaluationID: ev.ID, JobID: ev.JobID, ChecklistVersion: ev.ChecklistVersion,
		Model: ev.Model, PromptVersion: ev.PromptVersion, Total: ev.Total, W: ev.W,
		FinalTotal: ev.FinalTotal, FinalW: ev.FinalW, GrossError: ev.GrossError,
		Appeals: ev.Appeals, CreatedAt: ev.CreatedAt, RevisedAt: ev.RevisedAt,
		InputHash: ev.InputHash, Historical: ev.InputHash != conv.SourceHash,
	}
	if r.Historical {
		r.Channel, r.Language, r.AudioURL = "", "", nil
	}
	for _, it := range ev.Items {
		c := names[it.CriterionID]
		r.Items = append(r.Items, Item{
			CriterionID: it.CriterionID, Name: c.Name, Type: c.Type, Answer: it.Answer, FinalAnswer: it.FinalAnswer,
			Weight: it.Weight, Applicable: it.Applicable, Q: it.Q, Points: it.Points, Reason: it.Reason, Evidence: it.Evidence,
		})
	}
	return r
}

func WriteJSON(w io.Writer, records []Record) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(Document{ExportedAt: time.Now().UTC(), Count: len(records), Records: records})
}

var csvHeader = []string{
	"conversation_id", "channel", "language", "evaluation_id", "checklist_version", "model", "prompt_version",
	"total", "final_total", "gross_error", "gross_error_category",
	"criterion_id", "criterion_name", "answer", "final_answer", "weight", "applicable", "q", "points", "evidence", "reason",
	"input_hash", "historical",
}

func WriteCSV(w io.Writer, records []Record) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		return err
	}
	for _, r := range records {
		for _, it := range r.Items {
			ev, _ := json.Marshal(it.Evidence)
			row := []string{
				csvText(r.ConversationID), csvText(r.Channel), csvText(r.Language), strconv.FormatInt(r.EvaluationID, 10), strconv.Itoa(r.ChecklistVersion), csvText(r.Model), csvText(r.PromptVersion),
				num(r.Total), num(r.FinalTotal), strconv.FormatBool(r.GrossError.Flag), csvText(r.GrossError.Category),
				csvText(it.CriterionID), csvText(it.Name), it.Answer, str(it.FinalAnswer), fmtF(it.Weight), strconv.FormatBool(it.Applicable), fmtF(it.Q), fmtF(it.Points), string(ev), csvText(it.Reason),
				r.InputHash, strconv.FormatBool(r.Historical),
			}
			if err := cw.Write(row); err != nil {
				return err
			}
		}
	}
	cw.Flush()
	return cw.Error()
}

func csvText(value string) string {
	first, _ := utf8.DecodeRuneInString(value)
	meaningful, _ := utf8.DecodeRuneInString(strings.TrimSpace(value))
	if strings.ContainsRune("\t\r\n", first) || strings.ContainsRune("=+-@＝＋－＠", meaningful) {
		return "'" + value
	}
	return value
}

func num(p *float64) string {
	if p == nil {
		return ""
	}
	return fmtF(*p)
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func fmtF(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

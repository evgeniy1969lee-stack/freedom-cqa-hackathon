package importer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/homepunks/datathong/internal/domain"
)

var ErrBadInput = errors.New("bad input")

type rawTurn struct {
	Timestamp json.RawMessage `json:"timestamp"`
	Speaker   string          `json:"speaker"`
	Text      string          `json:"text"`
}

type rawConversation struct {
	ID         string    `json:"conversation_id"`
	Channel    string    `json:"channel"`
	Language   string    `json:"language"`
	Transcript []rawTurn `json:"transcript"`
	AudioURL   *string   `json:"audio_url"`
}

type wrapper struct {
	Data *struct {
		Records []json.RawMessage `json:"records"`
	} `json:"data"`
	Records []json.RawMessage `json:"records"`
}

func Parse(body []byte) ([]domain.Conversation, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, fmt.Errorf("%w: empty body", ErrBadInput)
	}
	var w wrapper
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadInput, err)
	}
	records := w.Records
	if w.Data != nil {
		records = w.Data.Records
	}
	if records == nil {
		records = []json.RawMessage{body}
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%w: no records", ErrBadInput)
	}
	out := make([]domain.Conversation, 0, len(records))
	seen := map[string]bool{}
	for i, r := range records {
		c, err := parseOne(r)
		if err != nil {
			return nil, fmt.Errorf("record %d: %w", i, err)
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("%w: duplicate conversation_id %q in one import", ErrBadInput, c.ID)
		}
		seen[c.ID] = true
		out = append(out, c)
	}
	return out, nil
}

func parseOne(r json.RawMessage) (domain.Conversation, error) {
	var rc rawConversation
	if err := json.Unmarshal(r, &rc); err != nil {
		return domain.Conversation{}, fmt.Errorf("%w: %v", ErrBadInput, err)
	}
	c := domain.Conversation{
		ID:       strings.TrimSpace(rc.ID),
		Channel:  strings.TrimSpace(rc.Channel),
		Language: strings.ToUpper(strings.TrimSpace(rc.Language)),
		AudioURL: rc.AudioURL,
	}
	if c.ID == "" {
		return c, fmt.Errorf("%w: conversation_id is required", ErrBadInput)
	}
	if len(rc.Transcript) == 0 {
		return c, fmt.Errorf("%w: transcript is empty", ErrBadInput)
	}
	if c.AudioURL != nil && strings.TrimSpace(*c.AudioURL) == "" {
		c.AudioURL = nil
	}
	c.Turns = make([]domain.Turn, len(rc.Transcript))
	for i, rt := range rc.Transcript {
		sp := strings.TrimSpace(rt.Speaker)
		if sp == "" {
			return c, fmt.Errorf("%w: turn %d has no speaker", ErrBadInput, i)
		}
		c.Turns[i] = domain.Turn{
			TurnID:    i,
			Timestamp: timestampString(rt.Timestamp),
			Speaker:   sp,
			Text:      rt.Text,
		}
	}
	c.SourceHash = hash(c)
	return c, nil
}

func timestampString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

func hash(c domain.Conversation) string {
	type turn struct {
		T, S, X string
	}
	payload := struct {
		Channel  string
		Language string
		AudioURL *string
		Turns    []turn
	}{Channel: c.Channel, Language: c.Language, AudioURL: c.AudioURL}
	for _, t := range c.Turns {
		payload.Turns = append(payload.Turns, turn{t.Timestamp, t.Speaker, t.Text})
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

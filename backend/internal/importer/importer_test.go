package importer

import (
	"errors"
	"os"
	"testing"
)

func TestParseWrapperAndSingle(t *testing.T) {
	records, err := os.ReadFile("../../testdata/example_records.json")
	if err != nil {
		t.Fatal(err)
	}
	cs, err := Parse(records)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("records: got %d conversations, want 2", len(cs))
	}
	single, err := os.ReadFile("../../testdata/example_conversation.json")
	if err != nil {
		t.Fatal(err)
	}
	one, err := Parse(single)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || one[0].ID != "example-001" {
		t.Fatalf("single: %+v", one)
	}
	for i, tr := range one[0].Turns {
		if tr.TurnID != i {
			t.Fatalf("turn %d has id %d", i, tr.TurnID)
		}
	}
	if one[0].Turns[2].Timestamp != one[0].Turns[3].Timestamp {
		t.Fatal("fixture should contain two adjacent turns with the same timestamp")
	}
	if one[0].Language != "RU" || one[0].AudioURL != nil {
		t.Fatalf("language=%q audio=%v", one[0].Language, one[0].AudioURL)
	}
	if cs[0].SourceHash != one[0].SourceHash {
		t.Fatal("same dialog in wrapper and single form must hash identically")
	}
	if cs[1].AudioURL == nil || *cs[1].AudioURL == "" {
		t.Fatal("call record should keep audio_url")
	}
}

func TestHashChangesWithContent(t *testing.T) {
	a, _ := Parse([]byte(`{"conversation_id":"x","channel":"chat","language":"ru","transcript":[{"timestamp":"00:01","speaker":"client","text":"hi"}]}`))
	b, _ := Parse([]byte(`{"conversation_id":"x","channel":"chat","language":"ru","transcript":[{"timestamp":"00:01","speaker":"client","text":"hi!"}]}`))
	c, _ := Parse([]byte(`{"conversation_id":"y","channel":"chat","language":"RU","transcript":[{"timestamp":"00:01","speaker":"client","text":"hi"}]}`))
	if a[0].SourceHash == b[0].SourceHash {
		t.Fatal("changed text must change hash")
	}
	if a[0].SourceHash != c[0].SourceHash {
		t.Fatal("hash must not depend on conversation_id or language case")
	}
}

func TestNumericTimestampKept(t *testing.T) {
	cs, err := Parse([]byte(`{"conversation_id":"x","channel":"call","language":"KZ","transcript":[{"timestamp":12.5,"speaker":"operator","text":"..."}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if cs[0].Turns[0].Timestamp != "12.5" {
		t.Fatalf("timestamp = %q", cs[0].Turns[0].Timestamp)
	}
}

func TestBadInput(t *testing.T) {
	for name, body := range map[string]string{
		"empty":         ``,
		"not json":      `{`,
		"no id":         `{"channel":"chat","language":"RU","transcript":[{"speaker":"client","text":"hi"}]}`,
		"no transcript": `{"conversation_id":"x","channel":"chat","language":"RU","transcript":[]}`,
		"no speaker":    `{"conversation_id":"x","channel":"chat","language":"RU","transcript":[{"text":"hi"}]}`,
		"empty records": `{"data":{"records":[]}}`,
		"dup ids":       `{"data":{"records":[{"conversation_id":"x","transcript":[{"speaker":"a","text":""}]},{"conversation_id":"x","transcript":[{"speaker":"a","text":""}]}]}}`,
	} {
		if _, err := Parse([]byte(body)); !errors.Is(err, ErrBadInput) {
			t.Errorf("%s: err = %v, want ErrBadInput", name, err)
		}
	}
}

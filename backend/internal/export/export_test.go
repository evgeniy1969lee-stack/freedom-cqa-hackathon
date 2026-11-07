package export

import (
	"bytes"
	"encoding/csv"
	"testing"
)

func TestCSVFormulaLikeText(t *testing.T) {
	var data bytes.Buffer
	records := []Record{{ConversationID: " =1+1", Items: []Item{{Reason: "@SUM(1,2)", Answer: "0"}}}}
	if err := WriteCSV(&data, records); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&data).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	cells := make(map[string]string)
	for i, name := range rows[0] {
		cells[name] = rows[1][i]
	}
	if cells["conversation_id"] != "' =1+1" || cells["reason"] != "'@SUM(1,2)" || cells["answer"] != "0" {
		t.Fatalf("unexpected CSV cells: %v", cells)
	}
}

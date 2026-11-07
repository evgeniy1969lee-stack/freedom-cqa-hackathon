package checklist

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/homepunks/datathong/internal/domain"
)

//go:embed seed.json
var seedJSON []byte

func Seed() (domain.Checklist, error) {
	var cl domain.Checklist
	if err := json.Unmarshal(seedJSON, &cl); err != nil {
		return cl, fmt.Errorf("seed checklist: %w", err)
	}
	if err := cl.Validate(); err != nil {
		return cl, fmt.Errorf("seed checklist: %w", err)
	}
	return cl, nil
}

package service

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestResearchSectionsMatchTheContract compares the research measurements with
// contracts/research-measurements.json, which the dashboard (web/) reads too:
// a key, category, label or unit changed on one side fails here or there.
func TestResearchSectionsMatchTheContract(t *testing.T) {
	t.Parallel()
	type entry struct {
		Key      string `json:"key"`
		Category string `json:"category"`
		Label    string `json:"label"`
		Unit     string `json:"unit"`
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "research-measurements.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract []entry
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	var ours []entry
	for _, s := range researchSections {
		for _, f := range s.fields {
			ours = append(ours, entry{Key: f.key, Category: s.category, Label: f.label, Unit: f.unit.String()})
		}
	}
	if !reflect.DeepEqual(ours, contract) {
		t.Errorf("the research sections differ from the contract:\nours     %v\ncontract %v", ours, contract)
	}
}

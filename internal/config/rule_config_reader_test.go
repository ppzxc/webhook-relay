package config_test

import (
	"context"
	"testing"

	"relaybox/internal/config"
)

// TestInMemoryRuleConfigReader_GetRulesByInputID verifies that rules are indexed
// by input ID regardless of any legacy type value.
// RED: currently fails because Update() uses inp.Type ("BESZEL") as key,
// so GetRules("beszel") returns ErrInputNotFound.
func TestInMemoryRuleConfigReader_GetRulesByInputID(t *testing.T) {
	cfg := &config.Config{
		Inputs: []config.InputConfig{
			{
				ID:     "beszel",
				Engine: "CEL",
				Rules:  []config.RuleConfig{{OutputIDs: []string{"out1"}}},
			},
		},
		Outputs: []config.OutputConfig{
			{ID: "out1", Type: "WEBHOOK", Engine: "CEL", URL: "https://example.com"},
		},
	}

	r := config.NewInMemoryRuleConfigReader(cfg)

	// Must find rules by input ID "beszel"
	engine, entries, err := r.GetRules(context.Background(), "beszel")
	if err != nil {
		t.Fatalf("GetRules(%q) error: %v", "beszel", err)
	}
	if engine != "CEL" {
		t.Errorf("engine = %q, want CEL", engine)
	}
	if len(entries) != 1 {
		t.Errorf("len(entries) = %d, want 1", len(entries))
	}

	// Must NOT find rules by uppercase ID that looks like an old-style type string
	_, _, err = r.GetRules(context.Background(), "BESZEL")
	if err == nil {
		t.Error("GetRules(\"BESZEL\") should return error: routing must use exact input ID")
	}
}

package config_test

import (
	"testing"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/input"
)

func TestLoadInputChainConfig(t *testing.T) {
	loader := input.NewLoader()
	chainConfig, err := loader.LoadChainConfig("testdata/chain/good_chain_config.toml")
	if err != nil {
		t.Fatalf("failed to load chain config: %v", err)
	}

	// Verify the loaded config
	if chainConfig == nil {
		t.Fatal("chain config is nil")
	}
	if chainConfig.Chain.ID != "atomone-1" {
		t.Errorf("expected chain_id atomone-1, got %s", chainConfig.Chain.ID)
	}
	if chainConfig.Chain.Name != "Atom One" {
		t.Errorf("expected chain name 'Atom One', got %s", chainConfig.Chain.Name)
	}
	if len(chainConfig.Tokens) != 1 {
		t.Errorf("expected 1 token, got %d", len(chainConfig.Tokens))
	}
	if chainConfig.Tokens[0].Symbol != "ATONE" {
		t.Errorf("expected token symbol ATONE, got %s", chainConfig.Tokens[0].Symbol)
	}
}

func TestValidateInputConfig(t *testing.T) {
	loader := input.NewLoader()
	chainConfig, err := loader.LoadChainConfig("testdata/chain/good_chain_config.toml")
	if err != nil {
		t.Fatalf("failed to load chain config: %v", err)
	}

	validator := input.NewValidator(input.WithSkipNetworkCheck(true))
	result := validator.Validate(chainConfig)

	if !result.IsValid {
		t.Errorf("expected valid config, got errors: %v", result.Errors)
	}
}

func TestBadChainConfigValidation(t *testing.T) {
	// Load an incomplete config - this should succeed (just loads the file)
	loader := input.NewLoader()
	chainConfig, err := loader.LoadChainConfig("testdata/chain/b_chain_config_1.toml")
	if err != nil {
		t.Fatalf("failed to load config file: %v", err)
	}

	// But validation should fail due to missing required fields
	validator := input.NewValidator(input.WithSkipNetworkCheck(true))
	result := validator.Validate(chainConfig)

	if result.IsValid {
		t.Error("expected validation to fail for incomplete config")
	}

	// Should have multiple errors for missing fields
	if len(result.Errors) == 0 {
		t.Error("expected validation errors, got none")
	}

	t.Logf("Validation errors (expected): %v", result.Errors)
}

func TestNonExistentConfigFile(t *testing.T) {
	loader := input.NewLoader()
	_, err := loader.LoadChainConfig("testdata/chain/nonexistent.toml")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

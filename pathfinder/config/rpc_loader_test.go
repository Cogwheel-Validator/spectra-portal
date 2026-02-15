package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/Cogwheel-Validator/spectra-portal/pathfinder/config"
)

// helper to reset env vars with PATHFINDER_ prefix between tests
func unsetPathfinderEnv() {
	for _, e := range os.Environ() {
		if len(e) > 12 && e[:12] == "PATHFINDER_" {
			if idx := strings.Index(e, "="); idx != -1 {
				_ = os.Unsetenv(e[:idx])
			}
		}
	}
}

func TestLoadRPCPathfinderConfig_FromEnv_Success(t *testing.T) {
	unsetPathfinderEnv()
	// set minimal valid envs
	_ = os.Setenv("PATHFINDER_PORT", "8080")
	_ = os.Setenv("PATHFINDER_HOST", "0.0.0.0")
	_ = os.Setenv("PATHFINDER_ALLOWED_ORIGINS", "*")
	_ = os.Setenv("PATHFINDER_SQS_URLS", "https://sqs.example.com/q1,https://sqs.example.com/q2")

	cfg, err := LoadRPCPathfinderConfig(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected config, got nil")
	}
	if cfg.Port != 8080 || cfg.Host != "0.0.0.0" {
		t.Errorf("unexpected port/host: %v %v", cfg.Port, cfg.Host)
	}
	if len(cfg.AllowedOrigins) == 0 {
		t.Errorf("expected at least one allowed origin")
	}
	if len(cfg.SqsURLs) != 2 {
		t.Errorf("expected 2 sqs urls, got %d", len(cfg.SqsURLs))
	}
}

func TestLoadRPCPathfinderConfig_FromEnv_FailVerification(t *testing.T) {
	unsetPathfinderEnv()
	_ = os.Unsetenv("PATHFINDER_HOST")
	// Run in empty dir so godotenv.Load() inside the loader doesn't set PATHFINDER_* from a .env file
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	_ = os.Chdir(t.TempDir())

	// missing HOST
	_ = os.Setenv("PATHFINDER_PORT", "8080")
	_ = os.Setenv("PATHFINDER_ALLOWED_ORIGINS", "*")
	_ = os.Setenv("PATHFINDER_SQS_URLS", "https://sqs.example.com/q1")

	_, err := LoadRPCPathfinderConfig(nil)
	if err == nil {
		t.Fatalf("expected error due to missing host, got nil")
	}
}

func TestLoadRPCPathfinderConfig_FromFile_Success(t *testing.T) {
	unsetPathfinderEnv()

	dir := t.TempDir()
	path := filepath.Join(dir, "rpc_config.toml")
	content := `
port = 9090
host = "127.0.0.1"
allowed_origins = ["https://example.com"]
sqs_urls = ["https://sqs.example.com/q1"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed writing temp config: %v", err)
	}

	cfgPath := path
	cfg, err := LoadRPCPathfinderConfig(&cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Port != 9090 || cfg.Host != "127.0.0.1" {
		t.Errorf("unexpected values: %+v", cfg)
	}
	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != "https://example.com" {
		t.Errorf("unexpected allowed origins: %+v", cfg.AllowedOrigins)
	}
	if len(cfg.SqsURLs) != 1 {
		t.Errorf("unexpected sqs urls: %+v", cfg.SqsURLs)
	}
}

func TestLoadRPCPathfinderConfig_FromFile_WrongExtension(t *testing.T) {
	unsetPathfinderEnv()
	p := "config.yaml"
	_, err := LoadRPCPathfinderConfig(&p)
	if err == nil {
		t.Fatalf("expected error for non-toml file")
	}
}

func TestLoadRPCPathfinderConfig_FileOverridesEnv(t *testing.T) {
	unsetPathfinderEnv()
	// set env with different values
	_ = os.Setenv("PATHFINDER_PORT", "8000")
	_ = os.Setenv("PATHFINDER_HOST", "0.0.0.0")
	_ = os.Setenv("PATHFINDER_ALLOWED_ORIGINS", "*")
	_ = os.Setenv("PATHFINDER_SQS_URLS", "https://sqs.example.com/q1,https://sqs.example.com/q2")

	dir := t.TempDir()
	path := filepath.Join(dir, "rpc_config.toml")
	content := `
port = 7000
host = "1.2.3.4"
allowed_origins = ["https://a.com"]
sqs_urls = ["https://b.com/q"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed writing temp config: %v", err)
	}
	cfgPath := path
	cfg, err := LoadRPCPathfinderConfig(&cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 7000 || cfg.Host != "1.2.3.4" {
		t.Errorf("expected file values to be used, got: %+v", cfg)
	}
}

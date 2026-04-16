package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigWithDefaults(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "taskmaster.toml")

	content := `[[program]]
name = "sleep"
command = "sleep"
args = ["10"]
autostart = true
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Server.Address != defaultAddress {
		t.Fatalf("expected default address %q, got %q", defaultAddress, cfg.Server.Address)
	}
	if len(cfg.Programs) != 1 {
		t.Fatalf("expected 1 program, got %d", len(cfg.Programs))
	}
	if got := cfg.Programs[0].Name; got != "sleep" {
		t.Fatalf("unexpected program name: %q", got)
	}
}

func TestLoadConfigValidation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "taskmaster.toml")

	content := `[[program]]
name = ""
command = "sleep"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("expected validation error")
	}
}

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExampleConfigMatchesDefaults(t *testing.T) {
	cfg, err := loadConfig("../../config.example.toml")
	if err != nil {
		t.Fatal(err)
	}
	want := defaultConfig()
	want.Rofi.Args = []string{}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("config.example.toml differs from defaults:\n%+v\n%+v", cfg, want)
	}
}

func TestConfigChainAndSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte(`
[backend]
phrase = "azure"
word = ["dictcc", "llm"]

[azure]
api_key = "k1"
region = "westeurope"

[llm]
api_key_cmd = "printf 'k2\nmore'"
`), 0o644)
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Backend.Phrase, chain{"azure"}) || !reflect.DeepEqual(cfg.Backend.Word, chain{"dictcc", "llm"}) {
		t.Errorf("chains = %v / %v", cfg.Backend.Phrase, cfg.Backend.Word)
	}
	if k, err := cfg.Azure.resolve("UNSET_TEST_VAR"); k != "k1" || err != nil {
		t.Errorf("azure key = %q, %v", k, err)
	}
	if k, err := cfg.LLM.resolve("UNSET_TEST_VAR"); k != "k2" || err != nil {
		t.Errorf("llm key = %q, %v", k, err)
	}
	if cfg.Azure.Region != "westeurope" {
		t.Errorf("region = %q", cfg.Azure.Region)
	}

	os.WriteFile(path, []byte("[backend]\nword = [\"nope\"]\n"), 0o644)
	if _, err := loadConfig(path); err == nil {
		t.Error("unknown backend should fail")
	}
}

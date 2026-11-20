package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Home    string `toml:"home"`
	Foreign string `toml:"foreign"`

	Detect struct {
		// How much more "foreign" a text must score than "home" before it is
		// translated foreign=>home. Ambiguous input goes home=>foreign.
		Margin float64 `toml:"margin"`
		// Retry in the other direction when the translation equals the input.
		FlipOnIdentity bool `toml:"flip_on_identity"`
	} `toml:"detect"`

	Backend struct {
		Phrase    chain `toml:"phrase"`
		Word      chain `toml:"word"`
		TimeoutMS int   `toml:"timeout_ms"` // per backend attempt
	} `toml:"backend"`

	GoogleCloud struct {
		secret
	} `toml:"google_cloud"`

	Azure struct {
		secret
		Region   string `toml:"region"`
		Endpoint string `toml:"endpoint"`
	} `toml:"azure"`

	DeepL struct {
		secret
		TargetVariants map[string]string `toml:"target_variants"`
	} `toml:"deepl"`

	Bergamot struct {
		Command string   `toml:"command"`
		Args    []string `toml:"args"`
	} `toml:"bergamot"`

	DictCC struct {
		File       string   `toml:"file"`
		Langs      []string `toml:"langs"` // language of column 1 and 2
		MaxResults int      `toml:"max_results"`
	} `toml:"dictcc"`

	LLM struct {
		secret
		BaseURL     string   `toml:"base_url"`
		Model       string   `toml:"model"`
		JSONMode    bool     `toml:"json_mode"`
		Temperature *float64 `toml:"temperature"`
	} `toml:"llm"`

	Output struct {
		Selections  []string `toml:"selections"` // clipboard | primary | secondary
		TypeDelayMS int      `toml:"type_delay_ms"`
		TypeWaitMS  int      `toml:"type_wait_ms"`
	} `toml:"output"`

	Keys struct {
		Toggle string `toml:"toggle"`
		From   string `toml:"from"`
		To     string `toml:"to"`
		Type   string `toml:"type"`
		Edit   string `toml:"edit"`
	} `toml:"keys"`

	Rofi struct {
		Bin       string   `toml:"bin"`
		ShowHints bool     `toml:"show_hints"`
		Unbind    []string `toml:"unbind"`
		Args      []string `toml:"args"`
		MaxLines  int      `toml:"max_lines"`
	} `toml:"rofi"`
}

// chain is a list of backend names; a single string is accepted too.
type chain []string

func (c *chain) UnmarshalTOML(v any) error {
	switch v := v.(type) {
	case string:
		*c = chain{v}
	case []any:
		out := chain{}
		for _, x := range v {
			s, ok := x.(string)
			if !ok {
				return fmt.Errorf("backend chain: %v is not a string", x)
			}
			out = append(out, s)
		}
		*c = out
	default:
		return errors.New("backend chain must be a string or a list of strings")
	}
	return nil
}

// secret is an API key given directly, by a command (e.g. "pass show x") or
// by an environment variable.
type secret struct {
	APIKey    string `toml:"api_key"`
	APIKeyCmd string `toml:"api_key_cmd"`
}

// resolve runs api_key_cmd lazily, so a slow password manager only costs time
// when that backend is actually used. No key at all is errUnsupported, so an
// unconfigured backend in the chain is skipped silently.
func (s secret) resolve(env string) (string, error) {
	if s.APIKey != "" {
		return s.APIKey, nil
	}
	if k := os.Getenv(env); k != "" {
		return k, nil
	}
	if s.APIKeyCmd != "" {
		out, err := exec.Command("sh", "-c", s.APIKeyCmd).Output()
		if err != nil {
			return "", fmt.Errorf("api_key_cmd: %w", err)
		}
		// Password managers like pass put the secret on the first line.
		return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]), nil
	}
	return "", fmt.Errorf("no API key (api_key, api_key_cmd or $%s): %w", env, errUnsupported)
}

func defaultConfig() *Config {
	c := &Config{Home: "de", Foreign: "en"}
	c.Detect.Margin = 1.5
	c.Detect.FlipOnIdentity = true
	// Backends that are not set up are skipped, so the defaults can list all
	// free ones; paid or key-based ones only run once configured.
	c.Backend.Phrase = chain{"google", "azure", "google_cloud", "llm", "bergamot"}
	c.Backend.Word = chain{"dictcc", "google", "azure", "google_cloud", "llm", "bergamot"}
	c.Backend.TimeoutMS = 3000
	c.Azure.Endpoint = "https://api.cognitive.microsofttranslator.com"
	c.DeepL.TargetVariants = map[string]string{"en": "EN-US", "pt": "PT-BR"}
	c.Bergamot.Command = "translateLocally"
	c.Bergamot.Args = []string{"-m", "{from}-{to}-base"}
	c.DictCC.File = "~/.local/share/linguini/dictcc-de-en.txt"
	c.DictCC.Langs = []string{"de", "en"}
	c.DictCC.MaxResults = 30
	c.LLM.BaseURL = "https://api.openai.com/v1"
	c.LLM.JSONMode = true
	c.Output.Selections = []string{"clipboard"}
	c.Output.TypeDelayMS = 12
	c.Output.TypeWaitMS = 80
	c.Keys.Toggle = "Alt+t"
	c.Keys.From = "Alt+s"
	c.Keys.To = "Alt+o"
	c.Keys.Type = "Shift+Return"
	c.Keys.Edit = "Alt+e"
	c.Rofi.Bin = "rofi"
	c.Rofi.ShowHints = true
	// Shift+Return is rofi's default kb-accept-alt; free it for keys.type.
	c.Rofi.Unbind = []string{"kb-accept-alt"}
	c.Rofi.MaxLines = 12
	return c
}

func configPath() string {
	return filepath.Join(xdgDir("XDG_CONFIG_HOME", ".config"), "linguini", "config.toml")
}

func xdgDir(env, fallback string) string {
	if dir := os.Getenv(env); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, fallback)
}

func expandHome(path string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, rest)
	}
	return path
}

// loadConfig decodes path over the defaults. A missing file is not an error.
func loadConfig(path string) (*Config, error) {
	c := defaultConfig()
	if _, err := toml.DecodeFile(path, c); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	c.Home = strings.ToLower(c.Home)
	c.Foreign = strings.ToLower(c.Foreign)
	if c.Home == c.Foreign {
		return nil, fmt.Errorf("config: home and foreign are both %q", c.Home)
	}
	for _, name := range append(append(chain{}, c.Backend.Phrase...), c.Backend.Word...) {
		if _, ok := backends[name]; !ok {
			return nil, fmt.Errorf("config: unknown backend %q", name)
		}
	}
	if len(c.DictCC.Langs) != 2 {
		return nil, errors.New("config: dictcc.langs needs exactly two languages")
	}
	return c, nil
}

func (c *Config) timeout() time.Duration {
	return time.Duration(c.Backend.TimeoutMS) * time.Millisecond
}

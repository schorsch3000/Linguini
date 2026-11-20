package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeTool puts an executable named name on PATH that runs body.
func fakeTool(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestTypeText(t *testing.T) {
	dir := t.TempDir()
	fakeTool(t, dir, "xdotool", `printf '%s\n' "$@" > `+dir+`/typed`)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	cfg := defaultConfig()
	cfg.Output.TypeDelayMS = 40
	cfg.Output.TypeWaitMS = 0

	if err := typeText(cfg, "-starts with a dash"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "typed"))
	want := "type\n--clearmodifiers\n--delay\n40\n--\n-starts with a dash\n"
	if string(data) != want {
		t.Errorf("xdotool args:\n%s\nwant:\n%s", data, want)
	}
}

func TestCopyFallsBackToXsel(t *testing.T) {
	dir := t.TempDir()
	fakeTool(t, dir, "xsel", `cat > `+dir+`/xsel$1`)
	// Only the fake dir: no xclip on PATH. sh and cat are needed by the fake.
	shDir := filepath.Dir(mustLookPath(t, "sh"))
	catDir := filepath.Dir(mustLookPath(t, "cat"))
	t.Setenv("PATH", dir+":"+shDir+":"+catDir)
	cfg := defaultConfig()
	cfg.Output.Selections = []string{"primary", "clipboard"}

	if err := copyText(cfg, "Haus"); err != nil {
		t.Fatal(err)
	}
	for _, sel := range []string{"--primary", "--clipboard"} {
		if data, _ := os.ReadFile(filepath.Join(dir, "xsel"+sel)); string(data) != "Haus" {
			t.Errorf("%s = %q", sel, data)
		}
	}
}

func mustLookPath(t *testing.T, name string) string {
	t.Helper()
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return filepath.Join(dir, name)
		}
	}
	t.Fatalf("%s not found", name)
	return ""
}

func TestRofiArgs(t *testing.T) {
	cfg := defaultConfig()
	cfg.Rofi.Args = []string{"-theme", "mine.rasi"}
	log := fakeRofi(t, cfg, "1\t")

	if _, _, err := runRofi(cfg, menu{prompt: "p", mesg: "m", format: "f", filter: "x y"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(log)
	want := "-dmenu -p p -format f -mesg m -filter x y -kb-accept-alt  " +
		"-kb-custom-1 Alt+t -kb-custom-2 Alt+s -kb-custom-3 Alt+o -kb-custom-4 Shift+Return " +
		"-kb-custom-5 Alt+e -theme mine.rasi"
	if got := strings.TrimSpace(string(data)); got != want {
		t.Errorf("args:\n%s\nwant:\n%s", got, want)
	}
}

func TestRofiUnexpectedExit(t *testing.T) {
	cfg := defaultConfig()
	fakeRofi(t, cfg, "3\t")
	if _, _, err := runRofi(cfg, menu{format: "f"}); err == nil || !strings.Contains(err.Error(), "exited 3") {
		t.Errorf("err = %v", err)
	}
}

func TestHints(t *testing.T) {
	cfg := defaultConfig()
	got := hints(cfg, "Shift+Return", "type", "Alt+<", "odd")
	if got != "<b>Shift+Enter</b> type   <b>Alt+&lt;</b> odd" {
		t.Errorf("hints = %q", got)
	}
	cfg.Rofi.ShowHints = false
	if got := hints(cfg, "Return", "copy"); got != "" {
		t.Errorf("disabled hints = %q", got)
	}
}

func TestPickerLangsConfiguredFirst(t *testing.T) {
	langs := pickerLangs("fr", "de")
	if langs[0].Code != "de" || langs[1].Code != "fr" || len(langs) != len(languages) {
		t.Errorf("first = %v %v, len %d", langs[0], langs[1], len(langs))
	}
	if langName("de") != "German" || langName("xx") != "xx" {
		t.Error("langName")
	}
}

func TestPaths(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("XDG_CONFIG_HOME", "")
	if got := configPath(); got != "/home/test/.config/linguini/config.toml" {
		t.Errorf("configPath = %s", got)
	}
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := configPath(); got != "/xdg/linguini/config.toml" {
		t.Errorf("configPath = %s", got)
	}
	if got := expandHome("~/a/b"); got != "/home/test/a/b" {
		t.Errorf("expandHome = %s", got)
	}
	if got := expandHome("/abs"); got != "/abs" {
		t.Errorf("expandHome = %s", got)
	}
}

func TestConfigRejectsSameLanguages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.toml")
	os.WriteFile(path, []byte(`home = "DE"`+"\n"+`foreign = "de"`), 0o644)
	if _, err := loadConfig(path); err == nil {
		t.Error("home == foreign should fail")
	}
	os.WriteFile(path, []byte("[dictcc]\nlangs = [\"de\"]\n"), 0o644)
	if _, err := loadConfig(path); err == nil {
		t.Error("one dictcc language should fail")
	}
	os.WriteFile(path, []byte("home = \n"), 0o644)
	if _, err := loadConfig(path); err == nil {
		t.Error("broken TOML should fail")
	}
}

func TestParseLLMEdgeCases(t *testing.T) {
	cases := map[string][]Entry{
		"":                                  nil,
		`{"translations":[]}`:               {{Text: `{"translations":[]}`}},
		`{"translations":[{"text":" a "}]}`: {{Text: "a"}},
		"plain answer\n":                    {{Text: "plain answer"}},
	}
	for in, want := range cases {
		if got := parseLLM(in); !reflect.DeepEqual(got, want) {
			t.Errorf("parseLLM(%q) = %v, want %v", in, got, want)
		}
	}
}

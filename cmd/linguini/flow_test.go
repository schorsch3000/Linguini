package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRofi installs a rofi stand-in that replays scripted answers, one
// "exitcode<TAB>output" line per invocation, and logs each argument list.
func fakeRofi(t *testing.T, cfg *Config, answers ...string) (logFile string) {
	t.Helper()
	dir := t.TempDir()
	queue := filepath.Join(dir, "queue")
	logFile = filepath.Join(dir, "log")
	os.WriteFile(queue, []byte(strings.Join(answers, "\n")+"\n"), 0o644)
	script := `#!/bin/sh
printf '%s' "$*" | tr '\n' '|' >> ` + logFile + `
echo >> ` + logFile + `
cat > /dev/null
line=$(head -n1 ` + queue + `)
sed -i 1d ` + queue + `
code=${line%%	*}
out=${line#*	}
printf '%s\n' "$out"
exit $code
`
	bin := filepath.Join(dir, "rofi")
	os.WriteFile(bin, []byte(script), 0o755)
	cfg.Rofi.Bin = bin

	// Fake xclip writes the copied text to <dir>/clip-<selection>.
	os.WriteFile(filepath.Join(dir, "xclip"),
		[]byte("#!/bin/sh\ncat > "+dir+"/clip-$2\n"), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return logFile
}

// fakeDeepL translates with a fixed table, keyed "FROM>TO:text".
func fakeDeepL(t *testing.T, table map[string]string) *[]string {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text   []string `json:"text"`
			Source string   `json:"source_lang"`
			Target string   `json:"target_lang"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		key := req.Source + ">" + req.Target + ":" + req.Text[0]
		calls = append(calls, key)
		out, ok := table[key]
		if !ok {
			out = req.Text[0]
		}
		json.NewEncoder(w).Encode(map[string]any{
			"translations": []map[string]string{{"text": out}},
		})
	}))
	t.Cleanup(srv.Close)
	deeplURL, deeplFreeURL = srv.URL, srv.URL
	t.Cleanup(func() {
		deeplURL, deeplFreeURL = "https://api.deepl.com/v2/translate", "https://api-free.deepl.com/v2/translate"
	})
	return &calls
}

func testConfig() *Config {
	cfg := defaultConfig()
	cfg.DeepL.APIKey = "test:fx"
	cfg.Backend.Phrase = chain{"deepl"}
	cfg.Backend.Word = chain{"deepl"}
	cfg.Output.Selections = []string{"clipboard", "primary"}
	return cfg
}

func readClip(t *testing.T, log, sel string) string {
	data, _ := os.ReadFile(filepath.Join(filepath.Dir(log), "clip-"+sel))
	return string(data)
}

func TestFlowAmbiguousWordGoesHomeToForeign(t *testing.T) {
	cfg := testConfig()
	calls := fakeDeepL(t, map[string]string{"DE>EN-US:Gift": "poison"})
	log := fakeRofi(t, cfg, "0\tGift", "0\t0")

	if err := run(cfg, ""); err != nil {
		t.Fatal(err)
	}
	if got := readClip(t, log, "clipboard"); got != "poison" {
		t.Errorf("clipboard = %q, want poison", got)
	}
	if got := readClip(t, log, "primary"); got != "poison" {
		t.Errorf("primary = %q, want poison", got)
	}
	if len(*calls) != 1 {
		t.Errorf("calls = %v", *calls)
	}
}

func TestFlowIdentityFlip(t *testing.T) {
	cfg := testConfig()
	calls := fakeDeepL(t, map[string]string{"EN>DE:house": "Haus"})
	log := fakeRofi(t, cfg, "0\thouse", "0\t0")

	if err := run(cfg, ""); err != nil {
		t.Fatal(err)
	}
	if got := readClip(t, log, "clipboard"); got != "Haus" {
		t.Errorf("clipboard = %q, want Haus (calls %v)", got, *calls)
	}
	logData, _ := os.ReadFile(log)
	if !strings.Contains(string(logData), "-p en⇒de") {
		t.Errorf("result prompt should show en⇒de:\n%s", logData)
	}
}

func TestFlowToggleKeepsText(t *testing.T) {
	cfg := testConfig()
	fakeDeepL(t, map[string]string{"EN>DE:die": "sterben"})
	// Alt+t on the input screen (exit 10), then Enter, then copy row 0.
	log := fakeRofi(t, cfg, "10\tdie", "0\tdie", "0\t0")

	if err := run(cfg, ""); err != nil {
		t.Fatal(err)
	}
	logData, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(logData)), "\n")
	if !strings.Contains(lines[1], "-p en⇒de") || !strings.Contains(lines[1], "-filter die") {
		t.Errorf("second rofi should be en⇒de with text kept:\n%s", lines[1])
	}
	if got := readClip(t, log, "clipboard"); got != "sterben" {
		t.Errorf("clipboard = %q", got)
	}
}

func TestFlowPickLanguageKeepsText(t *testing.T) {
	cfg := testConfig()
	calls := fakeDeepL(t, map[string]string{"DE>FR:Guten Morgen": "Bonjour"})
	frIndex := -1
	for i, l := range pickerLangs(cfg.Home, cfg.Foreign) {
		if l.Code == "fr" {
			frIndex = i
		}
	}
	// Alt+o (exit 12), pick fr, back on input: Enter, copy row 0.
	log := fakeRofi(t, cfg, "12\tGuten Morgen", "0\t"+itoa(frIndex), "0\tGuten Morgen", "0\t0")

	if err := run(cfg, ""); err != nil {
		t.Fatal(err)
	}
	logData, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(logData)), "\n")
	if !strings.Contains(lines[2], "-p de⇒fr") || !strings.Contains(lines[2], "-filter Guten Morgen") {
		t.Errorf("input after picking should be de⇒fr with text kept:\n%s", lines[2])
	}
	if got := readClip(t, log, "clipboard"); got != "Bonjour" {
		t.Errorf("clipboard = %q (calls %v)", got, *calls)
	}
}

func TestFlowPickerEscapeKeepsDirection(t *testing.T) {
	cfg := testConfig()
	fakeDeepL(t, nil)
	// Alt+s (exit 11), Escape in picker, Escape on input.
	log := fakeRofi(t, cfg, "11\tHallo", "1\t", "1\tHallo")

	if err := run(cfg, ""); err != nil {
		t.Fatal(err)
	}
	logData, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(logData)), "\n")
	if !strings.Contains(lines[2], "-p de⇄en") || !strings.Contains(lines[2], "-filter Hallo") {
		t.Errorf("escape in picker should return unchanged:\n%s", lines[2])
	}
}

func TestFlowEditFromResult(t *testing.T) {
	cfg := testConfig()
	fakeDeepL(t, map[string]string{"DE>EN-US:Haus": "house", "DE>EN-US:Hausboot": "houseboat"})
	// Enter, Alt+e (exit 14) back to input, change text, Enter, copy.
	log := fakeRofi(t, cfg, "0\tHaus", "14\t0", "0\tHausboot", "0\t0")

	if err := run(cfg, ""); err != nil {
		t.Fatal(err)
	}
	logData, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(logData)), "\n")
	if !strings.Contains(lines[2], "-filter Haus") {
		t.Errorf("edit should restore text:\n%s", lines[2])
	}
	if got := readClip(t, log, "clipboard"); got != "houseboat" {
		t.Errorf("clipboard = %q", got)
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

func TestGoogleDictionary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("sl") != "de" || r.URL.Query().Get("tl") != "en" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"sentences":[{"trans":"poison","orig":"Gift"}],
			"dict":[{"pos":"noun","terms":["poison","venom","toxin"],"entry":[
				{"word":"poison","reverse_translation":["Gift","Vergiftung"]},
				{"word":"venom","reverse_translation":["Gift","Bosheit"]},
				{"word":"toxin","reverse_translation":["Toxin","Gift"]}]}],"src":"de"}`))
	}))
	defer srv.Close()
	old := googleURL
	googleURL = srv.URL
	defer func() { googleURL = old }()

	entries, err := google(t.Context(), defaultConfig(), "Gift", "de", "en")
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{"poison", "noun · Gift, Vergiftung"},
		{"venom", "noun · Gift, Bosheit"},
		{"toxin", "noun · Toxin, Gift"},
	}
	if len(entries) != len(want) {
		t.Fatalf("entries = %v", entries)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("entry %d = %v, want %v", i, entries[i], want[i])
		}
	}
}

func TestFlowTypeFromResult(t *testing.T) {
	cfg := testConfig()
	cfg.Output.TypeWaitMS = 0
	fakeDeepL(t, map[string]string{"DE>EN-US:Haus": "house", "EN>DE:house": "Haus"})
	// Enter, then Shift+Enter (exit 13) on row 0.
	log := fakeRofi(t, cfg, "0\tHaus", "13\t0")
	dir := filepath.Dir(log)
	os.WriteFile(filepath.Join(dir, "xdotool"),
		[]byte("#!/bin/sh\nfor a; do last=$a; done\nprintf %s \"$last\" > "+dir+"/typed\n"), 0o755)

	if err := run(cfg, ""); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "typed")); string(data) != "house" {
		t.Errorf("typed = %q", data)
	}
	if readClip(t, log, "clipboard") != "" {
		t.Error("typing must not copy")
	}
}

func TestFlowSwapOnResultScreen(t *testing.T) {
	cfg := testConfig()
	calls := fakeDeepL(t, map[string]string{"DE>EN-US:Kind": "child", "EN>DE:Kind": "Art"})
	// Enter (de=>en), Alt+t on the result (exit 10) retranslates en=>de, copy.
	log := fakeRofi(t, cfg, "0\tKind", "10\t0", "0\t0")

	if err := run(cfg, ""); err != nil {
		t.Fatal(err)
	}
	if got := readClip(t, log, "clipboard"); got != "Art" {
		t.Errorf("clipboard = %q (calls %v)", got, *calls)
	}
	logData, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(logData)), "\n")
	if !strings.Contains(lines[1], "-p de⇒en") || !strings.Contains(lines[2], "-p en⇒de") {
		t.Errorf("prompts:\n%s", logData)
	}
}

func TestFlowEmptyInputQuits(t *testing.T) {
	cfg := testConfig()
	fakeRofi(t, cfg, "0\t  ")
	if err := run(cfg, ""); err != nil {
		t.Fatal(err)
	}
}

func TestFlowBackendErrorIsReturned(t *testing.T) {
	cfg := testConfig()
	cfg.DeepL.APIKey = ""
	t.Setenv("DEEPL_API_KEY", "")
	fakeRofi(t, cfg, "0\tHaus")
	err := run(cfg, "")
	if err == nil || !strings.Contains(err.Error(), "is set up") {
		t.Errorf("err = %v", err)
	}
}

func TestFlowInitialText(t *testing.T) {
	cfg := testConfig()
	log := fakeRofi(t, cfg, "1\t")
	if err := run(cfg, "aus der Auswahl"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(log); !strings.Contains(string(data), "-filter aus der Auswahl") {
		t.Errorf("initial text not passed:\n%s", data)
	}
}

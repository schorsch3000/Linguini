package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func serve(t *testing.T, h http.HandlerFunc) string {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestChainFallsBack(t *testing.T) {
	cfg := defaultConfig()
	cfg.Backend.Phrase = chain{"llm", "google", "azure"} // llm unconfigured: skipped
	googleURL = serve(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "<html>Sorry...</html>", http.StatusTooManyRequests)
	})
	t.Cleanup(func() { googleURL = "https://translate.googleapis.com/translate_a/single" })
	cfg.Azure.APIKey = "k"
	cfg.Azure.Endpoint = serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"translations":[{"text":"good morning","to":"en"}]}]`))
	})

	res, err := translateOnce(t.Context(), cfg, "guten Morgen", "de", "en")
	if err != nil {
		t.Fatal(err)
	}
	if res.Backend != "azure" || res.Entries[0].Text != "good morning" {
		t.Errorf("res = %+v", res)
	}

	cfg.Backend.Phrase = chain{"google"}
	_, err = translateOnce(t.Context(), cfg, "guten Morgen", "de", "en")
	if err == nil || !strings.Contains(err.Error(), "google: HTTP 429: Too Many Requests") {
		t.Errorf("err = %v", err)
	}

	cfg.Backend.Phrase = chain{"llm", "google_cloud"}
	_, err = translateOnce(t.Context(), cfg, "guten Morgen", "de", "en")
	if err == nil || !strings.Contains(err.Error(), "is set up") {
		t.Errorf("all unconfigured: err = %v", err)
	}
}

func TestAzure(t *testing.T) {
	cfg := defaultConfig()
	cfg.Azure.APIKey = "secret"
	cfg.Azure.Region = "westeurope"
	cfg.Azure.Endpoint = serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Ocp-Apim-Subscription-Key") != "secret" ||
			r.Header.Get("Ocp-Apim-Subscription-Region") != "westeurope" {
			t.Errorf("headers = %v", r.Header)
		}
		if r.URL.Query().Get("from") != "de" || r.URL.Query().Get("to") != "en" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		switch r.URL.Path {
		case "/dictionary/lookup":
			w.Write([]byte(`[{"translations":[
				{"displayTarget":"poison","posTag":"NOUN","backTranslations":[{"displayText":"Gift"},{"displayText":"Vergiftung"}]},
				{"displayTarget":"venom","posTag":"NOUN","backTranslations":[{"displayText":"Gift"}]}]}]`))
		case "/translate":
			w.Write([]byte(`[{"translations":[{"text":"the poison","to":"en"}]}]`))
		default:
			t.Errorf("path %s", r.URL.Path)
		}
	})

	entries, err := azure(t.Context(), cfg, "Gift", "de", "en")
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{{"poison", "noun · Gift, Vergiftung"}, {"venom", "noun · Gift"}}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("word = %v", entries)
	}
	entries, err = azure(t.Context(), cfg, "das Gift", "de", "en")
	if err != nil || entries[0].Text != "the poison" {
		t.Errorf("phrase = %v, %v", entries, err)
	}
}

func TestGoogleCloud(t *testing.T) {
	cfg := defaultConfig()
	cfg.GoogleCloud.APIKey = "k&x"
	googleCloudURL = serve(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if r.URL.Query().Get("key") != "k&x" || req["target"] != "zh-CN" {
			t.Errorf("query %s body %v", r.URL.RawQuery, req)
		}
		w.Write([]byte(`{"data":{"translations":[{"translatedText":"it&#39;s fine"}]}}`))
	})
	t.Cleanup(func() { googleCloudURL = "https://translation.googleapis.com/language/translate/v2" })

	entries, err := googleCloud(t.Context(), cfg, "passt schon", "de", "zh")
	if err != nil || entries[0].Text != "it's fine" {
		t.Errorf("entries = %v, %v", entries, err)
	}
}

func TestLLM(t *testing.T) {
	var reply string
	var got map[string]any
	var auth string
	cfg := defaultConfig()
	cfg.LLM.Model = "test-model"
	cfg.LLM.BaseURL = serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&got)
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": reply}}},
		})
	}) + "/"

	// A local server without a key: no Authorization header.
	t.Setenv("OPENAI_API_KEY", "")
	reply = "```json\n{\"translations\":[{\"text\":\"poison\",\"note\":\"noun\"},{\"text\":\"venom\",\"note\":\"noun, of animals\"}]}\n```"
	entries, err := llm(t.Context(), cfg, "Gift", "de", "en")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(entries, []Entry{{"poison", "noun"}, {"venom", "noun, of animals"}}) {
		t.Errorf("entries = %v", entries)
	}
	if auth != "" || got["model"] != "test-model" || got["temperature"] != nil || got["response_format"] == nil {
		t.Errorf("auth %q request %v", auth, got)
	}
	if prompt := got["messages"].([]any)[0].(map[string]any)["content"].(string); !strings.Contains(prompt, "from German to English") ||
		!strings.Contains(prompt, "single word") {
		t.Errorf("prompt = %s", prompt)
	}

	// A model ignoring the JSON instruction still gives a result.
	cfg.LLM.APIKey = "sk"
	zero := 0.0
	cfg.LLM.Temperature = &zero
	reply = "Good morning"
	entries, err = llm(t.Context(), cfg, "Guten Morgen", "de", "en")
	if err != nil || !reflect.DeepEqual(entries, []Entry{{Text: "Good morning"}}) {
		t.Errorf("entries = %v, %v", entries, err)
	}
	if auth != "Bearer sk" || got["temperature"] != 0.0 {
		t.Errorf("auth %q temperature %v", auth, got["temperature"])
	}

	cfg.LLM.Model = ""
	if _, err := llm(t.Context(), cfg, "x", "de", "en"); err == nil {
		t.Error("no model should fail")
	}
}

func TestBergamot(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-translate")
	os.WriteFile(script, []byte(`#!/bin/sh
[ "$1 $2" = "-m de-en-base" ] || { echo "error: model $2 not found" >&2; exit 1; }
read line
echo "translated: $line"
`), 0o755)
	cfg := defaultConfig()
	cfg.Bergamot.Command = script

	entries, err := bergamot(t.Context(), cfg, "Hallo Welt", "de", "en")
	if err != nil || entries[0].Text != "translated: Hallo Welt" {
		t.Errorf("entries = %v, %v", entries, err)
	}
	_, err = bergamot(t.Context(), cfg, "Hallo", "de", "fr")
	if err == nil || !strings.Contains(err.Error(), "model de-fr-base not found") {
		t.Errorf("err = %v", err)
	}
	cfg.Bergamot.Command = "definitely-not-installed"
	if _, err := bergamot(t.Context(), cfg, "x", "de", "en"); err == nil {
		t.Error("missing command should fail")
	}
}

const testDict = `# dict.cc test file
# German	English	word class	subject
Gift {n}	poison	noun	[chem.]
Gift {n}	venom	noun	[zool.]
Gift {n}	poison	noun	[med.]
Haus {n}	house	noun
Haus {n}	home	noun
Hausboot {n}	houseboat	noun
(sich) verlieben	to fall in love	verb
vergiften	to poison sb./sth.	verb
Geschenk {n}	gift	noun
`

func setupDict(t *testing.T, content string) *Config {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	path := filepath.Join(dir, "de-en.txt")
	os.WriteFile(path, []byte(content), 0o644)
	cfg := defaultConfig()
	cfg.DictCC.File = path
	dictOnce = sync.Once{}
	t.Cleanup(func() { dictOnce = sync.Once{} })
	return cfg
}

func TestDictCC(t *testing.T) {
	cfg := setupDict(t, testDict)

	entries, err := dictcc(t.Context(), cfg, "gift", "de", "en")
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{"poison", "noun · [chem.] [med.]"},
		{"venom", "noun · [zool.]"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("de=>en = %v", entries)
	}

	entries, _ = dictcc(t.Context(), cfg, "Haus", "de", "en")
	if len(entries) != 2 || entries[0].Text != "house" || entries[1].Text != "home" {
		t.Errorf("Haus = %v", entries)
	}

	// English verbs are found without "to"; the result keeps annotations apart.
	entries, _ = dictcc(t.Context(), cfg, "fall in love", "en", "de")
	if len(entries) != 1 || entries[0].Text != "(sich) verlieben" || entries[0].Note != "verb" {
		t.Errorf("fall in love = %v", entries)
	}
	entries, _ = dictcc(t.Context(), cfg, "house", "en", "de")
	if len(entries) != 1 || entries[0] != (Entry{"Haus", "noun {n}"}) {
		t.Errorf("house = %v", entries)
	}
	if entries, _ := dictcc(t.Context(), cfg, "Hausbo", "de", "en"); len(entries) != 0 {
		t.Errorf("prefix must not match: %v", entries)
	}
	if _, err := dictcc(t.Context(), cfg, "Haus", "de", "fr"); err == nil {
		t.Error("de-fr should be unsupported")
	}
}

func TestDictCCDetection(t *testing.T) {
	cfg := setupDict(t, testDict)
	cases := map[string]string{
		"Gift":       "de", // both languages: home wins
		"house":      "en", // only English
		"houseboat":  "en",
		"Hausboot":   "de",
		"Quatsch":    "de", // unknown: heuristic, which defaults to home
		"vergiften":  "de",
		"throughout": "de", // unknown and no signal
	}
	for word, want := range cases {
		if from, _ := detectDirection(cfg, word); from != want {
			t.Errorf("detectDirection(%q) from = %s, want %s", word, from, want)
		}
	}
}

func TestDictCCIndexRebuild(t *testing.T) {
	cfg := setupDict(t, testDict)
	if entries, _ := dictcc(t.Context(), cfg, "Quatsch", "de", "en"); len(entries) != 0 {
		t.Fatalf("Quatsch should be unknown: %v", entries)
	}
	f, _ := os.OpenFile(cfg.DictCC.File, os.O_APPEND|os.O_WRONLY, 0)
	io.WriteString(f, "Quatsch {m}\tnonsense\tnoun\n")
	f.Close()
	dictOnce = sync.Once{}
	entries, err := dictcc(t.Context(), cfg, "Quatsch", "de", "en")
	if err != nil || len(entries) != 1 || entries[0].Text != "nonsense" {
		t.Errorf("after change: %v, %v", entries, err)
	}
}

func TestDictKeyAndSplit(t *testing.T) {
	if k := dictKey("to poison sb./sth.", "en"); k != "poison sb./sth." {
		t.Errorf("key = %q", k)
	}
	if k := dictKey("(sich) verlieben  {verb} [ugs.]", "de"); k != "verlieben" {
		t.Errorf("key = %q", k)
	}
	text, ann := splitTerm("Gift {n} [chem.] <Abk.>")
	if text != "Gift" || !reflect.DeepEqual(ann, []string{"{n}", "[chem.]", "<Abk.>"}) {
		t.Errorf("split = %q %v", text, ann)
	}
}

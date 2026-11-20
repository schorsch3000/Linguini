package main

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
)

var (
	googleURL      = "https://translate.googleapis.com/translate_a/single"
	googleCloudURL = "https://translation.googleapis.com/language/translate/v2"
)

// Google's codes differ from DeepL's for a few languages.
var googleCodes = map[string]string{"zh": "zh-CN", "nb": "no"}

func googleCode(c string) string {
	if g, ok := googleCodes[c]; ok {
		return g
	}
	return c
}

// google uses the unofficial endpoint translate-shell uses. Its "bd" data
// gives dictionary entries: alternatives grouped by part of speech.
func google(ctx context.Context, cfg *Config, text, from, to string) ([]Entry, error) {
	q := url.Values{
		"client": {"gtx"}, "sl": {googleCode(from)}, "tl": {googleCode(to)},
		"hl": {googleCode(cfg.Home)}, "dt": {"t", "bd"}, "dj": {"1"}, "q": {text},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		googleURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Sentences []struct {
			Trans string `json:"trans"`
		} `json:"sentences"`
		Dict []struct {
			Pos   string `json:"pos"`
			Entry []struct {
				Word    string   `json:"word"`
				Reverse []string `json:"reverse_translation"`
			} `json:"entry"`
		} `json:"dict"`
	}
	if err := doJSON(req, &out); err != nil {
		return nil, fmt.Errorf("google: %w", err)
	}

	var main strings.Builder
	for _, s := range out.Sentences {
		main.WriteString(s.Trans)
	}
	// The main translation comes first; a dictionary entry for the same word
	// only contributes its note.
	entries := []Entry{{Text: strings.TrimSpace(main.String())}}
	index := map[string]int{strings.ToLower(entries[0].Text): 0}
	for _, d := range out.Dict {
		for _, e := range d.Entry {
			note := d.Pos
			if len(e.Reverse) > 0 {
				note += " · " + strings.Join(e.Reverse[:min(len(e.Reverse), 4)], ", ")
			}
			key := strings.ToLower(e.Word)
			if i, ok := index[key]; ok {
				if entries[i].Note == "" {
					entries[i].Note = note
				}
				continue
			}
			index[key] = len(entries)
			entries = append(entries, Entry{Text: e.Word, Note: note})
		}
	}
	if entries[0].Text == "" {
		entries = entries[1:]
	}
	return entries, nil
}

// googleCloud is the official Cloud Translation API (v2, API key auth).
func googleCloud(ctx context.Context, cfg *Config, text, from, to string) ([]Entry, error) {
	key, err := cfg.GoogleCloud.resolve("GOOGLE_CLOUD_API_KEY")
	if err != nil {
		return nil, err
	}
	var out struct {
		Data struct {
			Translations []struct {
				TranslatedText string `json:"translatedText"`
			} `json:"translations"`
		} `json:"data"`
	}
	body := map[string]any{
		"q": []string{text}, "source": googleCode(from), "target": googleCode(to), "format": "text",
	}
	if err := postJSON(ctx, googleCloudURL+"?key="+url.QueryEscape(key), nil, body, &out); err != nil {
		return nil, err
	}
	var entries []Entry
	for _, t := range out.Data.Translations {
		// format=text should return plain text; unescape in case it doesn't.
		entries = append(entries, Entry{Text: html.UnescapeString(t.TranslatedText)})
	}
	return entries, nil
}

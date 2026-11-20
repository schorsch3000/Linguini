package main

import (
	"context"
	"net/url"
	"strings"
)

// Azure's codes differ from ours for a few languages.
var azureCodes = map[string]string{"zh": "zh-Hans"}

func azureCode(c string) string {
	if a, ok := azureCodes[c]; ok {
		return a
	}
	return c
}

// azure uses Microsoft Translator v3. Single words go to the dictionary
// lookup first, which returns alternatives with part of speech and
// back-translations; phrases (or words it does not know) go to /translate.
func azure(ctx context.Context, cfg *Config, text, from, to string) ([]Entry, error) {
	key, err := cfg.Azure.resolve("AZURE_TRANSLATOR_KEY")
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"Ocp-Apim-Subscription-Key": key}
	if cfg.Azure.Region != "" {
		headers["Ocp-Apim-Subscription-Region"] = cfg.Azure.Region
	}
	q := url.Values{"api-version": {"3.0"}, "from": {azureCode(from)}, "to": {azureCode(to)}}
	base := strings.TrimSuffix(cfg.Azure.Endpoint, "/")
	body := []map[string]string{{"Text": text}}

	if isWord(text) {
		var out []struct {
			Translations []struct {
				DisplayTarget    string `json:"displayTarget"`
				PosTag           string `json:"posTag"`
				BackTranslations []struct {
					DisplayText string `json:"displayText"`
				} `json:"backTranslations"`
			} `json:"translations"`
		}
		if err := postJSON(ctx, base+"/dictionary/lookup?"+q.Encode(), headers, body, &out); err != nil {
			return nil, err
		}
		var entries []Entry
		for _, r := range out {
			for _, t := range r.Translations {
				note := strings.ToLower(t.PosTag)
				var back []string
				for _, b := range t.BackTranslations[:min(len(t.BackTranslations), 4)] {
					back = append(back, b.DisplayText)
				}
				if len(back) > 0 {
					note += " · " + strings.Join(back, ", ")
				}
				entries = append(entries, Entry{Text: t.DisplayTarget, Note: note})
			}
		}
		if len(entries) > 0 {
			return entries, nil
		}
	}

	var out []struct {
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	if err := postJSON(ctx, base+"/translate?"+q.Encode(), headers, body, &out); err != nil {
		return nil, err
	}
	var entries []Entry
	for _, r := range out {
		for _, t := range r.Translations {
			entries = append(entries, Entry{Text: t.Text})
		}
	}
	return entries, nil
}

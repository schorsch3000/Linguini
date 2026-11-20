package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

var (
	deeplURL     = "https://api.deepl.com/v2/translate"
	deeplFreeURL = "https://api-free.deepl.com/v2/translate"
)

func deepl(ctx context.Context, cfg *Config, text, from, to string) ([]Entry, error) {
	key, err := cfg.DeepL.resolve("DEEPL_API_KEY")
	if err != nil {
		return nil, err
	}
	endpoint := deeplURL
	if strings.HasSuffix(key, ":fx") {
		endpoint = deeplFreeURL
	}
	target := upper(to)
	if v, ok := cfg.DeepL.TargetVariants[to]; ok {
		target = upper(v)
	}
	body, _ := json.Marshal(map[string]any{
		"text":        []string{text},
		"source_lang": upper(from),
		"target_lang": target,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+key)
	req.Header.Set("Content-Type", "application/json")

	var out struct {
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	if err := doJSON(req, &out); err != nil {
		return nil, fmt.Errorf("deepl: %w", err)
	}
	var entries []Entry
	for _, t := range out.Translations {
		entries = append(entries, Entry{Text: t.Text})
	}
	return entries, nil
}

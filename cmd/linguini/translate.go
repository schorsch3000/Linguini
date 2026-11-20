package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Entry is one selectable result row. Text is what gets copied or typed;
// Note is shown dimmed next to it (part of speech, back-translations).
type Entry struct {
	Text string
	Note string
}

type Result struct {
	From, To string
	Backend  string
	Entries  []Entry
}

type backendFunc func(ctx context.Context, cfg *Config, text, from, to string) ([]Entry, error)

var backends = map[string]backendFunc{
	"google":       google,
	"google_cloud": googleCloud,
	"azure":        azure,
	"deepl":        deepl,
	"bergamot":     bergamot,
	"dictcc":       dictcc,
	"llm":          llm,
}

// errUnsupported marks a backend that cannot handle this language pair or is
// not set up; the chain moves on without reporting it.
var errUnsupported = errors.New("not available")

func isWord(text string) bool {
	return !strings.ContainsAny(strings.TrimSpace(text), " \t\n")
}

// translate runs the configured backend chain. With auto set, it first
// detects the direction and retries the other way when the output just
// echoes the input (e.g. "house" guessed as German comes back as "house").
func translate(ctx context.Context, cfg *Config, text, from, to string, auto bool) (*Result, error) {
	text = strings.TrimSpace(text)
	if auto {
		from, to = detectDirection(cfg, text)
	}
	res, err := translateOnce(ctx, cfg, text, from, to)
	if err != nil {
		return nil, err
	}
	if auto && cfg.Detect.FlipOnIdentity && echoes(res, text) {
		if flipped, err := translateOnce(ctx, cfg, text, to, from); err == nil && !echoes(flipped, text) {
			return flipped, nil
		}
	}
	return res, nil
}

// detectDirection prefers the dictionary for single words: a word that only
// exists as a foreign headword is foreign. Everything else uses detect().
func detectDirection(cfg *Config, text string) (string, string) {
	if isWord(text) {
		inHome, errHome := dictHas(cfg, text, cfg.Home)
		inForeign, errForeign := dictHas(cfg, text, cfg.Foreign)
		if errHome == nil && errForeign == nil {
			switch {
			case inHome:
				return cfg.Home, cfg.Foreign
			case inForeign:
				return cfg.Foreign, cfg.Home
			}
		}
	}
	return detect(text, cfg.Home, cfg.Foreign, cfg.Detect.Margin)
}

func echoes(res *Result, text string) bool {
	return len(res.Entries) > 0 && strings.EqualFold(strings.TrimSpace(res.Entries[0].Text), text)
}

// translateOnce tries each backend of the chain in order and returns the
// first non-empty result. Each backend gets its own timeout.
func translateOnce(ctx context.Context, cfg *Config, text, from, to string) (*Result, error) {
	chain := cfg.Backend.Phrase
	if isWord(text) {
		chain = cfg.Backend.Word
	}
	var errs []error
	for _, name := range chain {
		bctx, cancel := context.WithTimeout(ctx, cfg.timeout())
		entries, err := backends[name](bctx, cfg, text, from, to)
		cancel()
		if err == nil && len(entries) == 0 {
			err = errors.New("no result")
		}
		if err == nil {
			return &Result{From: from, To: to, Backend: name, Entries: entries}, nil
		}
		if !errors.Is(err, errUnsupported) {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("no backend in %v is set up for %s", chain, direction(from, to))
	}
	return nil, errors.Join(errs...)
}

func doJSON(req *http.Request, out any) error {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 200 || strings.HasPrefix(msg, "<") {
			msg = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}
	return json.Unmarshal(data, out)
}

// postJSON sends body as JSON and decodes the response into out.
func postJSON(ctx context.Context, url string, headers map[string]string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return doJSON(req, out)
}

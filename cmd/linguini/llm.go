package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const llmPrompt = `You are a translation engine. Translate the user's text from %s to %s.
Reply with JSON only, in this shape:
{"translations": [{"text": "...", "note": "..."}]}
%s
Never add explanations outside the JSON.`

const llmWordHint = `The input is a single word: list up to 6 common translations, most common first.
"note" is the part of speech and, if the meanings differ, a short hint in %s which meaning it is.`

const llmPhraseHint = `Give the most natural translation first, then at most 2 alternatives if they differ
meaningfully. "note" is empty or a very short hint in %s (e.g. "formal", "idiom").`

// llm talks to any OpenAI-compatible chat completions API: OpenAI itself,
// Ollama (base_url http://localhost:11434/v1), llama.cpp, LM Studio, ...
func llm(ctx context.Context, cfg *Config, text, from, to string) ([]Entry, error) {
	if cfg.LLM.Model == "" {
		return nil, fmt.Errorf("llm.model not set: %w", errUnsupported)
	}
	headers := map[string]string{}
	key, err := cfg.LLM.resolve("OPENAI_API_KEY")
	switch {
	case err == nil:
		headers["Authorization"] = "Bearer " + key
	case strings.HasPrefix(cfg.LLM.BaseURL, "https://api.openai.com"):
		return nil, err
	}
	// Local servers usually need no key.

	hint := llmPhraseHint
	if isWord(text) {
		hint = llmWordHint
	}
	homeName := langName(cfg.Home)
	body := map[string]any{
		"model": cfg.LLM.Model,
		"messages": []map[string]string{
			{"role": "system", "content": fmt.Sprintf(llmPrompt, langName(from), langName(to), fmt.Sprintf(hint, homeName))},
			{"role": "user", "content": text},
		},
	}
	if cfg.LLM.JSONMode {
		body["response_format"] = map[string]string{"type": "json_object"}
	}
	// Some models only accept their default temperature, so it is opt-in.
	if cfg.LLM.Temperature != nil {
		body["temperature"] = *cfg.LLM.Temperature
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	url := strings.TrimSuffix(cfg.LLM.BaseURL, "/") + "/chat/completions"
	if err := postJSON(ctx, url, headers, body, &out); err != nil {
		return nil, err
	}
	if len(out.Choices) == 0 {
		return nil, nil
	}
	return parseLLM(out.Choices[0].Message.Content), nil
}

// parseLLM reads the JSON reply; models that ignore the format still give a
// usable result as plain text.
func parseLLM(content string) []Entry {
	var reply struct {
		Translations []struct {
			Text string `json:"text"`
			Note string `json:"note"`
		} `json:"translations"`
	}
	start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
	if start >= 0 && end > start && json.Unmarshal([]byte(content[start:end+1]), &reply) == nil {
		var entries []Entry
		for _, t := range reply.Translations {
			if t.Text = strings.TrimSpace(t.Text); t.Text != "" {
				entries = append(entries, Entry{Text: t.Text, Note: strings.TrimSpace(t.Note)})
			}
		}
		if len(entries) > 0 {
			return entries
		}
	}
	if content = strings.TrimSpace(content); content != "" {
		return []Entry{{Text: content}}
	}
	return nil
}

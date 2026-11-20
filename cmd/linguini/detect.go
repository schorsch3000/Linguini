package main

import (
	"strings"
	"unicode"
)

// Language detection only has to choose between the configured home and
// foreign language, which a handful of cheap signals handles well enough and
// in microseconds. Ambiguous input ("die", "Gift") falls back to home=>foreign.

type profile struct {
	stopwords map[string]bool
	// Pattern weights, matched against "^word$" so anchors work.
	patterns map[string]float64
	// Letters that (practically) only this language uses.
	letters string
	// German capitalises nouns, which is a strong hint in longer sentences.
	capitalNouns bool
}

func words(s ...string) map[string]bool {
	m := map[string]bool{}
	for _, w := range s {
		m[w] = true
	}
	return m
}

// Words common to both languages (an, in, so, was, war, will, also, man, hat)
// are deliberately missing from both stopword lists.
var profiles = map[string]profile{
	"de": {
		stopwords: words("der", "die", "das", "den", "dem", "des", "ein", "eine", "einen",
			"einem", "einer", "und", "oder", "aber", "nicht", "ist", "sind", "waren", "bin",
			"bist", "haben", "hatte", "wird", "werden", "ich", "du", "er", "sie", "es",
			"wir", "ihr", "mich", "dich", "sich", "mir", "dir", "uns", "euch", "mein",
			"dein", "sein", "kein", "keine", "zu", "mit", "von", "für", "auf", "aus", "bei",
			"nach", "über", "unter", "vor", "durch", "gegen", "ohne", "um", "im", "am",
			"vom", "zum", "zur", "ins", "auch", "noch", "schon", "nur", "sehr", "wie",
			"wer", "wo", "wann", "warum", "dass", "wenn", "weil", "ob", "als", "doch",
			"ja", "nein", "hier", "da", "dann", "jetzt", "mal", "alles", "viel", "gut",
			"heute", "kann", "muss", "soll", "habe", "geht", "gibt", "diese", "dieser"),
		patterns: map[string]float64{
			"sch": 1, "tz": 1, "pf": 1, "ung$": 1.2, "heit$": 1.5, "keit$": 1.5,
			"lich": 1, "chen$": 1, "^z": 0.6, "ei": 0.5, "eu": 0.4, "ie": 0.3,
			"ch": 0.3, "en$": 0.4, "^ge": 0.3,
		},
		letters:      "äöüß",
		capitalNouns: true,
	},
	"en": {
		stopwords: words("the", "a", "and", "or", "of", "to", "is", "are", "were", "be",
			"been", "it", "this", "that", "with", "for", "on", "at", "by", "from", "you",
			"i", "he", "she", "we", "they", "my", "your", "his", "her", "our", "their",
			"not", "do", "does", "did", "have", "has", "had", "would", "can", "could",
			"should", "what", "which", "who", "how", "why", "where", "when", "there",
			"if", "then", "than", "but", "about", "just", "all", "me", "him", "them",
			"its", "it's", "i'm", "don't", "can't", "won't", "isn't", "yes", "no",
			"some", "any", "very", "much", "get", "got", "make", "know", "think"),
		patterns: map[string]float64{
			"th": 0.5, "wh": 1, "ing$": 1, "ly$": 0.5, "oo": 0.5, "ea": 0.4,
			"ou": 0.4, "y": 0.5, "'": 1, "ce$": 0.5, "ght": 0.3,
			// A "c" outside ch/ck/sch is rare in native German words.
			"c[^hk]": 0.8, "c$": 0.8,
		},
	},
}

// detect returns the direction for text: home=>foreign unless the text looks
// clearly foreign.
func detect(text, home, foreign string, margin float64) (from, to string) {
	if score(text, foreign)-score(text, home) >= margin {
		return foreign, home
	}
	return home, foreign
}

func score(text, lang string) float64 {
	p, ok := profiles[lang]
	if !ok {
		return 0
	}
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && r != '\''
	})
	var s float64
	sentenceStart := true
	for i, raw := range fields {
		w := strings.ToLower(raw)
		if p.stopwords[w] {
			s += 2
		}
		for pat, weight := range p.patterns {
			s += weight * float64(countPattern("^"+w+"$", pat))
		}
		for _, r := range w {
			if strings.ContainsRune(p.letters, r) {
				s += 3
			}
		}
		if p.capitalNouns && len(fields) >= 3 && !sentenceStart && i > 0 &&
			unicode.IsUpper([]rune(raw)[0]) && raw != "I" {
			s += 0.5
		}
		sentenceStart = false
	}
	return s
}

// countPattern counts matches of a tiny pattern language: literal runes,
// "^"/"$" anchors (already present in s) and one "[^xy]" negated class.
func countPattern(s, pat string) int {
	if i := strings.Index(pat, "[^"); i >= 0 {
		j := strings.Index(pat[i:], "]") + i
		prefix, excluded, suffix := pat[:i], pat[i+2:j], pat[j+1:]
		n := 0
		for k := 0; k+len(prefix) < len(s); k++ {
			if !strings.HasPrefix(s[k:], prefix) {
				continue
			}
			r := []rune(s[k+len(prefix):])[0]
			rest := s[k+len(prefix)+len(string(r)):]
			if r != '$' && !strings.ContainsRune(excluded, r) && strings.HasPrefix(rest, suffix) {
				n++
			}
		}
		return n
	}
	return strings.Count(s, pat)
}

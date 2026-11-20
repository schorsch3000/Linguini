package main

import "testing"

func TestDetect(t *testing.T) {
	cases := []struct {
		text string
		from string
	}{
		// Ambiguous words default to home=>foreign.
		{"die", "de"},
		{"Gift", "de"},
		{"Kind", "de"},
		{"Hotel", "de"},
		{"Haus", "de"},
		{"Schmetterling", "de"},
		{"Müdigkeit", "de"},
		{"Ich weiß nicht", "de"},
		{"Wie geht's dir?", "de"},
		{"Das ist mir egal", "de"},
		{"Kannst du mir bitte helfen", "de"},
		{"Der Hund läuft über die Straße", "de"},
		{"thinking", "en"},
		{"I don't know", "en"},
		{"the house is big", "en"},
		{"What are you doing tonight?", "en"},
		{"This is a nice place", "en"},
		{"Could you please help me", "en"},
		// Plain English words like "nevertheless" carry no signal and go
		// de=>en; the identity check in translate() flips them.
		{"nevertheless", "de"},
	}
	for _, c := range cases {
		from, to := detect(c.text, "de", "en", defaultConfig().Detect.Margin)
		if from != c.from {
			t.Errorf("detect(%q) = %s=>%s (de %.1f, en %.1f), want from %s",
				c.text, from, to, score(c.text, "de"), score(c.text, "en"), c.from)
		}
	}
}

func TestCountPattern(t *testing.T) {
	cases := []struct {
		s, pat string
		want   int
	}{
		{"^city$", "c[^hk]", 1},
		{"^dach$", "c[^hk]", 0},
		{"^back$", "c[^hk]", 0},
		{"^zinc$", "c[^hk]", 0},
		{"^zinc$", "c$", 1},
		{"^zeitung$", "ung$", 1},
		{"^zeitung$", "^z", 1},
	}
	for _, c := range cases {
		if got := countPattern(c.s, c.pat); got != c.want {
			t.Errorf("countPattern(%q, %q) = %d, want %d", c.s, c.pat, got, c.want)
		}
	}
}

// Linguini is a rofi front end for quick translations between a home and a
// foreign language.
package main

import (
	"context"
	"flag"
	"fmt"
	"html"
	"os"
	"strings"
)

type state struct {
	text     string
	from, to string
	// auto: direction is detected from the text. Any manual change of
	// direction or language turns it off for the rest of the session.
	auto bool
}

// effective is the direction the current text would be translated in.
func (s *state) effective(cfg *Config) (string, string) {
	if s.auto {
		return detect(s.text, cfg.Home, cfg.Foreign, cfg.Detect.Margin)
	}
	return s.from, s.to
}

func (s *state) fix(cfg *Config) {
	s.from, s.to = s.effective(cfg)
	s.auto = false
}

func (s *state) prompt() string {
	if s.auto {
		return s.from + "⇄" + s.to
	}
	return direction(s.from, s.to)
}

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	configFile := flag.String("config", configPath(), "config file")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: linguini [-config file] [text]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("linguini", version)
		return
	}

	cfg, err := loadConfig(*configFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "linguini:", err)
		os.Exit(1)
	}
	if err := run(cfg, strings.Join(flag.Args(), " ")); err != nil {
		fmt.Fprintln(os.Stderr, "linguini:", err)
		rofiError(cfg, "linguini: "+err.Error())
		os.Exit(1)
	}
}

func run(cfg *Config, initial string) error {
	st := &state{text: initial, from: cfg.Home, to: cfg.Foreign, auto: true}
	for {
		done, err := inputScreen(cfg, st)
		if done || err != nil {
			return err
		}
		done, err = resultScreen(cfg, st)
		if done || err != nil {
			return err
		}
	}
}

// inputScreen loops until the user submits text (done=false) or quits.
func inputScreen(cfg *Config, st *state) (done bool, err error) {
	for {
		act, text, err := runRofi(cfg, menu{
			prompt: st.prompt(),
			mesg: hints(cfg, "Return", "translate", cfg.Keys.Toggle, "swap",
				cfg.Keys.From, "from", cfg.Keys.To, "to", "Escape", "quit"),
			format: "f",
			filter: st.text,
			extra:  []string{"-l", "0"},
		})
		if err != nil {
			return true, err
		}
		st.text = text
		switch act {
		case actAccept, actType:
			if strings.TrimSpace(st.text) == "" {
				return true, nil
			}
			return false, nil
		case actCancel:
			return true, nil
		default:
			if err := changeDirection(cfg, st, act); err != nil {
				return true, err
			}
		}
	}
}

func changeDirection(cfg *Config, st *state, act action) error {
	switch act {
	case actToggle:
		st.fix(cfg)
		st.from, st.to = st.to, st.from
	case actFrom, actTo:
		from, to := st.effective(cfg)
		target, other, label := &from, &to, "from"
		if act == actTo {
			target, other, label = &to, &from, "to"
		}
		lang, ok, err := pickLang(cfg, label, *target)
		if err != nil || !ok {
			return err
		}
		if lang == *other {
			*other = *target
		}
		*target = lang
		st.from, st.to, st.auto = from, to, false
	}
	return nil
}

func pickLang(cfg *Config, label, current string) (string, bool, error) {
	langs := pickerLangs(cfg.Home, cfg.Foreign)
	lines := make([]string, len(langs))
	selected := 0
	for i, l := range langs {
		lines[i] = l.Code + "  " + l.Name
		if l.Code == current {
			selected = i
		}
	}
	act, out, err := runRofi(cfg, menu{
		prompt: label,
		mesg:   hints(cfg, "Return", "select", "Escape", "back"),
		lines:  lines,
		format: "i",
		extra:  []string{"-i", "-no-custom", "-selected-row", fmt.Sprint(selected)},
	})
	if err != nil || act != actAccept {
		return "", false, err
	}
	i, ok := index(out, len(langs))
	if !ok {
		return "", false, nil
	}
	return langs[i].Code, true, nil
}

// resultScreen translates st.text and shows the result. done=false means go
// back to the input screen.
func resultScreen(cfg *Config, st *state) (done bool, err error) {
	for {
		res, err := translate(context.Background(), cfg, st.text, st.from, st.to, st.auto)
		if err != nil {
			return true, err
		}

		lines := make([]string, len(res.Entries))
		for i, e := range res.Entries {
			lines[i] = html.EscapeString(e.Text)
			if e.Note != "" {
				lines[i] += `   <span alpha="55%">` + html.EscapeString(e.Note) + `</span>`
			}
		}
		act, out, err := runRofi(cfg, menu{
			prompt: direction(res.From, res.To),
			mesg: joinMesg("<b>"+html.EscapeString(strings.TrimSpace(st.text))+"</b>"+
				`   <span alpha="55%">via `+res.Backend+`</span>`,
				hints(cfg, "Return", "copy", cfg.Keys.Type, "type", cfg.Keys.Toggle, "swap",
					cfg.Keys.From, "from", cfg.Keys.To, "to", cfg.Keys.Edit, "edit")),
			lines:  lines,
			format: "i",
			extra: []string{"-i", "-no-custom", "-markup-rows",
				"-l", fmt.Sprint(min(len(lines), cfg.Rofi.MaxLines))},
		})
		if err != nil {
			return true, err
		}

		switch act {
		case actAccept, actType:
			i, ok := index(out, len(res.Entries))
			if !ok {
				return true, nil
			}
			if act == actType {
				return true, typeText(cfg, res.Entries[i].Text)
			}
			return true, copyText(cfg, res.Entries[i].Text)
		case actCancel:
			return true, nil
		case actEdit:
			return false, nil
		default:
			// Keep the direction this result was shown in as the base.
			st.from, st.to, st.auto = res.From, res.To, false
			if err := changeDirection(cfg, st, act); err != nil {
				return true, err
			}
		}
	}
}

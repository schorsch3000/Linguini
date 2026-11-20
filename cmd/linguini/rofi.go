package main

import (
	"errors"
	"fmt"
	"html"
	"os/exec"
	"strconv"
	"strings"
)

type action int

const (
	actAccept action = iota
	actCancel
	actToggle
	actFrom
	actTo
	actType
	actEdit
)

// rofi -dmenu exits with 10+N-1 for -kb-custom-N.
var customActions = []action{actToggle, actFrom, actTo, actType, actEdit}

func (c *Config) customKeys() []string {
	return []string{c.Keys.Toggle, c.Keys.From, c.Keys.To, c.Keys.Type, c.Keys.Edit}
}

type menu struct {
	prompt string
	mesg   string
	lines  []string
	format string // rofi -format: "f" typed text, "i" row index
	filter string
	extra  []string
}

func runRofi(cfg *Config, m menu) (action, string, error) {
	args := []string{"-dmenu", "-p", m.prompt, "-format", m.format}
	if m.mesg != "" {
		args = append(args, "-mesg", m.mesg)
	}
	if m.filter != "" {
		args = append(args, "-filter", m.filter)
	}
	for _, kb := range cfg.Rofi.Unbind {
		args = append(args, "-"+kb, "")
	}
	for i, key := range cfg.customKeys() {
		args = append(args, fmt.Sprintf("-kb-custom-%d", i+1), key)
	}
	args = append(args, m.extra...)
	args = append(args, cfg.Rofi.Args...)

	cmd := exec.Command(cfg.Rofi.Bin, args...)
	cmd.Stdin = strings.NewReader(strings.Join(m.lines, "\n"))
	out, err := cmd.Output()
	text := strings.TrimSuffix(string(out), "\n")

	var exit *exec.ExitError
	switch {
	case err == nil:
		return actAccept, text, nil
	case errors.As(err, &exit):
		code := exit.ExitCode()
		if code == 1 {
			return actCancel, text, nil
		}
		if i := code - 10; i >= 0 && i < len(customActions) {
			return customActions[i], text, nil
		}
		return actCancel, "", fmt.Errorf("rofi exited %d: %s", code, strings.TrimSpace(string(exit.Stderr)))
	default:
		return actCancel, "", fmt.Errorf("rofi: %w", err)
	}
}

func rofiError(cfg *Config, msg string) {
	exec.Command(cfg.Rofi.Bin, "-e", msg).Run()
}

// hints renders "key label" pairs for -mesg, e.g. "<b>Alt+t</b> swap".
func hints(cfg *Config, pairs ...string) string {
	if !cfg.Rofi.ShowHints {
		return ""
	}
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		key := strings.ReplaceAll(pairs[i], "Return", "Enter")
		parts = append(parts, "<b>"+html.EscapeString(key)+"</b> "+html.EscapeString(pairs[i+1]))
	}
	return strings.Join(parts, "   ")
}

func joinMesg(parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, "\n")
}

func index(s string, n int) (int, bool) {
	i, err := strconv.Atoi(strings.TrimSpace(s))
	return i, err == nil && i >= 0 && i < n
}

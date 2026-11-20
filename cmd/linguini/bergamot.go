package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// bergamot runs an offline translator command, by default translateLocally
// with Mozilla's Bergamot models. The text goes to stdin; {from} and {to} in
// the arguments are replaced by the language codes.
func bergamot(ctx context.Context, cfg *Config, text, from, to string) ([]Entry, error) {
	bin, err := exec.LookPath(expandHome(cfg.Bergamot.Command))
	if err != nil {
		return nil, fmt.Errorf("%s not installed: %w", cfg.Bergamot.Command, errUnsupported)
	}
	r := strings.NewReplacer("{from}", from, "{to}", to)
	args := make([]string, len(cfg.Bergamot.Args))
	for i, a := range cfg.Bergamot.Args {
		args[i] = r.Replace(a)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = strings.NewReader(text + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			// Most likely no model for this pair.
			return nil, fmt.Errorf("%s %s: %s", cfg.Bergamot.Command,
				strings.Join(args, " "), lastLine(stderr.String()))
		}
		return nil, err
	}
	return []Entry{{Text: strings.TrimSpace(string(out))}}, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

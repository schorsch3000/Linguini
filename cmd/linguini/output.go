package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func copyText(cfg *Config, text string) error {
	for _, sel := range cfg.Output.Selections {
		var cmd *exec.Cmd
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", sel, "-in")
		} else {
			cmd = exec.Command("xsel", "--"+sel, "--input")
		}
		cmd.Stdin = strings.NewReader(text)
		// No stdout/stderr pipes: xclip forks a child that serves the
		// selection, and inherited pipes would make Run wait for it.
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("copy to %s: %w", sel, err)
		}
	}
	return nil
}

func typeText(cfg *Config, text string) error {
	// Give the window manager time to hand focus back after rofi closes.
	time.Sleep(time.Duration(cfg.Output.TypeWaitMS) * time.Millisecond)
	// --clearmodifiers: Shift is likely still held from Shift+Enter.
	return exec.Command("xdotool", "type", "--clearmodifiers",
		"--delay", strconv.Itoa(cfg.Output.TypeDelayMS), "--", text).Run()
}

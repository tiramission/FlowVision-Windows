//go:build windows

package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestHideConsoleWindow(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "echo hidden")
	hideConsoleWindow(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("no process attributes")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow is off")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatal("CREATE_NO_WINDOW is off")
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("hidden process failed: %v", err)
	}
	if !strings.Contains(string(out), "hidden") {
		t.Fatalf("hidden process output: %q", out)
	}
}

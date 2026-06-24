//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestLinuxDesktopContents(t *testing.T) {
	got := linuxDesktopContents("study-planner", "/opt/My App/study-planner")
	for _, want := range []string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=study-planner",
		`Exec="/opt/My App/study-planner"`, // quoted so spaces stay one argument
		"X-GNOME-Autostart-enabled=true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("desktop entry missing %q in:\n%s", want, got)
		}
	}
}

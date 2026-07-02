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

// TestDesktopExecArg pins the spec's two-layer escaping: exec-layer quoting
// (backslash, backtick, dollar, quote escaped inside double quotes; %% for a
// literal %) written through the value layer, where every exec-layer backslash
// doubles again. Each `want` is the exact byte sequence in the file.
func TestDesktopExecArg(t *testing.T) {
	cases := []struct {
		name, path, want string
	}{
		{"plain", `/opt/study-planner`, `"/opt/study-planner"`},
		{"space", `/opt/My App/study-planner`, `"/opt/My App/study-planner"`},
		{"double quote", `/opt/my"dir/app`, `"/opt/my\\"dir/app"`},
		{"backslash", `/opt/a\b/app`, `"/opt/a\\\\b/app"`},
		{"dollar", `/opt/$HOME/app`, `"/opt/\\$HOME/app"`},
		{"backtick", "/opt/`echo`/app", "\"/opt/\\\\`echo\\\\`/app\""},
		{"percent", `/opt/100%/app`, `"/opt/100%%/app"`},
		{"kitchen sink", `/opt/a\b"c$d%e/app`, `"/opt/a\\\\b\\"c\\$d%%e/app"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := desktopExecArg(c.path); got != c.want {
				t.Fatalf("desktopExecArg(%q) = %s, want %s", c.path, got, c.want)
			}
		})
	}
}

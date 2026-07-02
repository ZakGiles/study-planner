//go:build darwin

package main

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
)

// wellFormedXML reports whether s parses as XML (the DOCTYPE is consumed as a
// directive token).
func wellFormedXML(t *testing.T, s string) bool {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(s))
	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return true
		}
		if err != nil {
			return false
		}
	}
}

func TestDarwinPlistContents(t *testing.T) {
	got := darwinPlistContents("com.wails.study-planner", "/Applications/My App.app")
	for _, want := range []string{
		"<key>Label</key>",
		"<string>com.wails.study-planner</string>",
		"<string>/usr/bin/open</string>",
		"<string>/Applications/My App.app</string>", // spaces need no escaping in argv
		"<key>RunAtLoad</key>",
		"<true/>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plist missing %q in:\n%s", want, got)
		}
	}
	if !strings.HasPrefix(got, "<?xml") {
		t.Fatalf("plist should start with an XML declaration, got:\n%s", got)
	}
	if !wellFormedXML(t, got) {
		t.Fatalf("plist is not well-formed XML:\n%s", got)
	}
}

// TestDarwinPlistContentsEscapesXML: a bundle path containing XML-special
// characters must be escaped — written raw, launchd rejects the whole plist
// and the login item silently never fires.
func TestDarwinPlistContentsEscapesXML(t *testing.T) {
	got := darwinPlistContents("com.wails.study-planner", "/Users/x/Dev & Play/<Study>.app")
	if want := "<string>/Users/x/Dev &amp; Play/&lt;Study&gt;.app</string>"; !strings.Contains(got, want) {
		t.Fatalf("plist missing escaped path %q in:\n%s", want, got)
	}
	if strings.Contains(got, "Dev & Play") {
		t.Fatalf("raw unescaped path leaked into plist:\n%s", got)
	}
	if !wellFormedXML(t, got) {
		t.Fatalf("plist with special characters is not well-formed XML:\n%s", got)
	}
}

func TestResolveBundle(t *testing.T) {
	cases := []struct {
		name    string
		exe     string
		wantApp string
		wantOK  bool
	}{
		{
			name:    "packaged app (the wails build layout)",
			exe:     "/Users/zak/coding/study-planner/build/bin/study-planner.app/Contents/MacOS/study-planner",
			wantApp: "/Users/zak/coding/study-planner/build/bin/study-planner.app",
			wantOK:  true,
		},
		{
			name:    "installed app with spaces",
			exe:     "/Applications/Study Planner.app/Contents/MacOS/study-planner",
			wantApp: "/Applications/Study Planner.app",
			wantOK:  true,
		},
		{
			name:   "dev binary, not in a bundle",
			exe:    "/tmp/go-build123/b001/exe/study-planner",
			wantOK: false,
		},
		{
			name:   "binary in a MacOS dir but not under .app",
			exe:    "/opt/Contents/MacOS/study-planner",
			wantOK: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app, ok := resolveBundle(c.exe)
			if ok != c.wantOK || app != c.wantApp {
				t.Fatalf("resolveBundle(%q) = (%q, %v), want (%q, %v)",
					c.exe, app, ok, c.wantApp, c.wantOK)
			}
		})
	}
}

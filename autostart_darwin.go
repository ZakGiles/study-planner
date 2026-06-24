//go:build darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// macOS launch-on-login via a per-user LaunchAgent. We write a plist to
// ~/Library/LaunchAgents/<label>.plist with RunAtLoad=true; its presence is the
// enabled state. The agent launches the .app *bundle* through `open` (not the
// inner Mach-O binary) so it starts as a normal foregrounded GUI app in the
// user's session.

// autoStartPlistPath returns ~/Library/LaunchAgents/<label>.plist.
func autoStartPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", autoStartLabel+".plist"), nil
}

// resolveBundle walks up from .../Foo.app/Contents/MacOS/foo to .../Foo.app,
// returning ok=false when exe isn't inside a .app bundle layout. Pure (no I/O)
// so the detection is unit-testable.
func resolveBundle(exe string) (app string, ok bool) {
	macOS := filepath.Dir(exe)      // .../Contents/MacOS
	contents := filepath.Dir(macOS) // .../Contents
	app = filepath.Dir(contents)    // .../<Name>.app
	if filepath.Base(macOS) != "MacOS" ||
		filepath.Base(contents) != "Contents" ||
		!strings.HasSuffix(app, ".app") {
		return "", false
	}
	return app, true
}

// bundlePath resolves the running app's .app bundle from the executable path. It
// reports available=false when the executable isn't inside a .app bundle, which
// is the case under `wails dev` — there is no stable installed app to point the
// login item at, so the feature is hidden rather than wired to a transient path.
func bundlePath() (path string, available bool, err error) {
	exe, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", false, err
	}
	app, ok := resolveBundle(exe)
	return app, ok, nil
}

// autoStartTarget reports where the login item lives and whether the feature is
// usable in this build (a real .app bundle exists).
func autoStartTarget() (path string, available bool, err error) {
	_, available, err = bundlePath()
	if err != nil || !available {
		return "", false, err
	}
	path, err = autoStartPlistPath()
	return path, available, err
}

// autoStartEnabled reports whether the LaunchAgent plist exists.
func autoStartEnabled() (bool, error) {
	path, err := autoStartPlistPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// setAutoStartEnabled writes or removes the LaunchAgent plist. Enabling rewrites
// it so a moved/upgraded app picks up its current bundle path.
func setAutoStartEnabled(enabled bool) error {
	path, err := autoStartPlistPath()
	if err != nil {
		return err
	}
	if !enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	app, available, err := bundlePath()
	if err != nil {
		return err
	}
	if !available {
		return errAutoStartUnavailable
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(darwinPlistContents(autoStartLabel, app)), 0o644)
}

// darwinPlistContents builds the LaunchAgent plist. Pure (no I/O) so it can be
// unit-tested. The .app path goes through `open` as a separate ProgramArguments
// element, so spaces need no escaping.
func darwinPlistContents(label, bundlePath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/bin/open</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`, label, bundlePath)
}

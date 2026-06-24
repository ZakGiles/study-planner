package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// errAutoStartUnavailable is returned when the caller tries to toggle launch-on-
// login in a build where it isn't supported (an unbundled dev build on macOS).
var errAutoStartUnavailable = errors.New("launch on login is only available in the installed app")

// Launch-on-login support. The actual mechanism is OS-specific and lives in the
// autostart_{darwin,windows,linux}.go files behind build tags; this file holds
// the Wails-bound API and the bits shared across platforms. The OS artifact
// (a LaunchAgent plist, a registry value, or a .desktop file) is the single
// source of truth: "enabled" is derived from whether that artifact exists, so
// there is nothing to persist in the store.

// autoStartLabel is the stable identifier for the login item, reused as the
// macOS LaunchAgent label/filename and the Windows registry value. It matches
// the app's CFBundleIdentifier (build/darwin/Info.plist) so toggling and
// upgrades never orphan an entry under a different name.
const autoStartLabel = "com.wails.study-planner"

// autoStartName is the human/file-friendly app name (wails.json "name"), used
// for the Linux .desktop filename and its Name= field.
const autoStartName = "study-planner"

// exeIsTransient reports whether the running executable lives in the OS temp
// directory — the hallmark of a throwaway build from `go run`/`go test` or a
// `wails dev` session. Launching on login would persist that vanishing path and
// orphan the entry once the build is gone, so platforms that key availability on
// a stable installed binary (Linux, Windows) treat this as "unavailable". macOS
// has a stronger signal (a real .app bundle) and doesn't need this.
func exeIsTransient() bool {
	exe, err := os.Executable()
	if err != nil {
		return true // can't tell where we're running from; don't risk a stale entry
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	tmp := os.TempDir()
	if resolved, err := filepath.EvalSymlinks(tmp); err == nil {
		tmp = resolved
	}
	rel, err := filepath.Rel(tmp, exe)
	if err != nil {
		return false // different volume (e.g. Windows drive) — a real install
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// AutoStartStatus reports whether launching on login is enabled, and whether the
// feature is even usable in this build. Available is false for unbundled dev
// builds (`wails dev`), where there is no stable installed app to point a login
// item at — the UI shows the toggle disabled rather than writing a broken entry.
type AutoStartStatus struct {
	Enabled   bool `json:"enabled"`
	Available bool `json:"available"`
}

// GetAutoStart reports the current launch-on-login state. Any error reading the
// OS artifact is treated as "not enabled" so the Settings tab can always render;
// a genuine failure surfaces when the user actually toggles via SetAutoStart.
func (a *App) GetAutoStart() AutoStartStatus {
	_, available, err := autoStartTarget()
	if err != nil || !available {
		return AutoStartStatus{Available: false}
	}
	enabled, err := autoStartEnabled()
	if err != nil {
		return AutoStartStatus{Available: true}
	}
	return AutoStartStatus{Enabled: enabled, Available: true}
}

// SetAutoStart enables or disables launching the app on login and returns the
// resulting status. It is idempotent: enabling rewrites the artifact (picking up
// the app's current path), disabling is a no-op when nothing is installed.
func (a *App) SetAutoStart(enabled bool) (AutoStartStatus, error) {
	_, available, err := autoStartTarget()
	if err != nil {
		return AutoStartStatus{}, err
	}
	if !available {
		return AutoStartStatus{Available: false},
			errAutoStartUnavailable
	}
	if err := setAutoStartEnabled(enabled); err != nil {
		return AutoStartStatus{Available: true}, err
	}
	return a.GetAutoStart(), nil
}

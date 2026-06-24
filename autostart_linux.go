//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Linux launch-on-login via the freedesktop autostart spec: a .desktop file in
// ~/.config/autostart is launched by the session at login. Its presence is the
// enabled state.

// autoStartDesktopPath returns ~/.config/autostart/<name>.desktop, honouring
// $XDG_CONFIG_HOME when set.
func autoStartDesktopPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "autostart", autoStartName+".desktop"), nil
}

// autoStartTarget reports the .desktop path; the feature is available whenever
// we're running from a stable installed path (a packaged binary), but not from a
// throwaway `go run`/`wails dev` build whose path would vanish.
func autoStartTarget() (path string, available bool, err error) {
	if exeIsTransient() {
		return "", false, nil
	}
	path, err = autoStartDesktopPath()
	return path, true, err
}

// autoStartEnabled reports whether the autostart .desktop file exists.
func autoStartEnabled() (bool, error) {
	path, err := autoStartDesktopPath()
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

// setAutoStartEnabled writes or removes the .desktop file. Enabling rewrites it
// so a moved binary picks up its current path.
func setAutoStartEnabled(enabled bool) error {
	path, err := autoStartDesktopPath()
	if err != nil {
		return err
	}
	if !enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(linuxDesktopContents(autoStartName, exe)), 0o644)
}

// linuxDesktopContents builds the autostart .desktop entry. Pure (no I/O) so it
// can be unit-tested. The Exec path is wrapped in double quotes per the desktop
// entry spec so a path with spaces stays a single argument.
func linuxDesktopContents(name, exePath string) string {
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Exec="%s"
X-GNOME-Autostart-enabled=true
`, name, strings.ReplaceAll(exePath, `"`, `\"`))
}

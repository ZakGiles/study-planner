//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

// Windows launch-on-login via the per-user Run key. A value under
// HKCU\Software\Microsoft\Windows\CurrentVersion\Run named autoStartRunValue,
// holding the quoted executable path, makes Windows start the app at sign-in;
// its presence is the enabled state.

const (
	autoStartRunKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	autoStartRunValue = "StudyPlanner"
)

// autoStartTarget reports the registry value name. The feature is available when
// running from a stable installed .exe, but not from a throwaway `go run`/`wails
// dev` build in a temp dir whose path would vanish.
func autoStartTarget() (path string, available bool, err error) {
	if exeIsTransient() {
		return "", false, nil
	}
	return autoStartRunValue, true, nil
}

// autoStartEnabled reports whether the Run value exists.
func autoStartEnabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, autoStartRunKey, registry.QUERY_VALUE)
	if err != nil {
		return false, err
	}
	defer key.Close()
	_, _, err = key.GetStringValue(autoStartRunValue)
	if err == registry.ErrNotExist {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// setAutoStartEnabled writes or deletes the Run value. The stored command quotes
// the executable path so a path containing spaces is parsed as a single program.
func setAutoStartEnabled(enabled bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, autoStartRunKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if !enabled {
		if err := key.DeleteValue(autoStartRunValue); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return key.SetStringValue(autoStartRunValue, `"`+exe+`"`)
}

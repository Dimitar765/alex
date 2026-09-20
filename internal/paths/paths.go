// Package paths resolves per-user data locations for the desktop app
// across operating systems.
package paths

import (
	"os"
	"path/filepath"
)

// AppName is the directory name used under the OS config root.
const AppName = "Alexander"

// UserSavesDir returns the per-user saves directory:
//
//	Windows: %APPDATA%\Alexander\saves
//	macOS:   ~/Library/Application Support/Alexander/saves
//	Others:  $XDG_CONFIG_HOME/Alexander/saves
func UserSavesDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, AppName, "saves"), nil
}

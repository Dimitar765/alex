package paths

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestUserSavesDirIsAppScoped(t *testing.T) {
	dir, err := UserSavesDir()
	if err != nil {
		t.Fatalf("UserSavesDir() error = %v", err)
	}
	switch runtime.GOOS {
	case "windows":
		if filepath.Base(filepath.Dir(filepath.Dir(dir))) != AppName {
			t.Fatalf("dir %q must live under %q", dir, AppName)
		}
	default:
		// On unix-likes the config root is $XDG_CONFIG_HOME when set.
		if filepath.Base(filepath.Dir(dir)) != AppName || filepath.Base(dir) != "saves" {
			t.Fatalf("dir %q must be <config>/%s/saves", dir, AppName)
		}
	}
}

func TestUserSavesDirRespectsXDG(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("env override is not honored on this OS")
	}
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-root")
	dir, err := UserSavesDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := "/tmp/xdg-root/Alexander/saves"; dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
}

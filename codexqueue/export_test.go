package codexqueue

import (
	"os"
	"path/filepath"
	"testing"
)

// SyntheticCodexHome models the starter runtime layout: a canonical private
// home, a regular config, and auth.json symlinked to a synthetic default-home
// file. Tests use it without opening real credentials.
func SyntheticCodexHome(t testing.TB) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "role home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("# synthetic fixture; never runs a model\n"), 0600); err != nil {
		t.Fatal(err)
	}
	auth := filepath.Join(root, "synthetic-auth.json")
	if err := os.WriteFile(auth, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(auth, filepath.Join(home, "auth.json")); err != nil {
		t.Fatal(err)
	}
	return home
}

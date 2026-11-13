package codexqueue

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultHomeAliasesPreserveLegacyWithoutInspectingCredentials(t *testing.T) {
	defaultHome := SyntheticCodexHome(t)
	alias := filepath.Join(filepath.Dir(defaultHome), "default-alias")
	if err := os.Symlink(defaultHome, alias); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{defaultHome, defaultHome + "/", alias, alias + "/"} {
		if !isDefaultHome(value, defaultHome) {
			t.Fatal("default alias did not preserve legacy")
		}
	}
	if isDefaultHome(filepath.Join(defaultHome, "missing"), defaultHome) {
		t.Fatal("different home accepted as default")
	}
}

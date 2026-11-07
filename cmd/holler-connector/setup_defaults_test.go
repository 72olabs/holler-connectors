package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupDefaultsPreserveReconnectAndExplicitFlags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	makeFlags := func() *flag.FlagSet {
		f := flag.NewFlagSet("setup", flag.ContinueOnError)
		for _, n := range []string{"actor", "name-mode", "role", "attention", "project", "peer"} {
			f.String(n, "", "")
		}
		return f
	}
	f := makeFlags()
	if err := setupDefaults(f, "claude", path); err != nil {
		t.Fatal(err)
	}
	if f.Lookup("actor").Value.String() != "claude" || f.Lookup("name-mode").Value.String() != "allocate" {
		t.Fatal("bad fresh defaults")
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"attention_mode":"startup-only","actor":"existing","project":"custom"}`), 0600); err != nil {
		t.Fatal(err)
	}
	f = makeFlags()
	if err := f.Parse([]string{"--peer", "chosen"}); err != nil {
		t.Fatal(err)
	}
	if err := setupDefaults(f, "claude", path); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"actor": "existing", "attention": "startup-only", "project": "custom", "name-mode": "", "peer": "chosen"} {
		if f.Lookup(key).Value.String() != want {
			t.Fatal(key)
		}
	}
}

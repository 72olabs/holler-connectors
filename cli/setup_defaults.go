package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"github.com/72olabs/holler-connectors/connector"
	"os"
)

// Reconnecting preserves the selected identity and attention mode. Explicit flags
// win, including empty values. New terminal connections allocate their own identity.
func setupDefaults(flags *flag.FlagSet, harness, path string) error {
	explicit := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	var config interface{}
	var err error
	switch harness {
	case "claude":
		config, err = connector.LoadClaudeConnectorConfig(path)
	case "codex":
		config, err = connector.LoadCodexConnectorConfig(path)
	case "opencode":
		config, err = connector.LoadOpenCodeConnectorConfig(path)
	default:
		return nil
	}
	set := func(name, value string) {
		if !explicit[name] && flags.Lookup(name) != nil {
			_ = flags.Set(name, value)
		}
	}
	if errors.Is(err, os.ErrNotExist) {
		set("actor", harness)
		set("name-mode", "allocate")
		set("role", "assistant")
		modes := map[string]string{"claude": connector.AttentionHookLongPoll, "codex": connector.AttentionNativeQueue, "opencode": connector.AttentionNativePrompt}
		set("attention", modes[harness])
		return nil
	}
	if err != nil {
		return err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	values := map[string]interface{}{}
	if err = json.Unmarshal(raw, &values); err != nil {
		return err
	}
	for name, key := range map[string]string{"actor": "actor", "name-mode": "name_mode", "role": "role", "peer": "peer", "project": "project", "project-root": "project_root", "channel": "channel", "socket": "socket", "attention": "attention_mode", "profile": "profile", "plugin-id": "plugin_id", "client-binary": "client_binary"} {
		if value, ok := values[key].(string); ok {
			set(name, value)
		}
	}
	// Missing name_mode in legacy configs means exact, not allocate.
	if _, ok := values["name_mode"]; !ok {
		set("name-mode", "")
	}
	return nil
}

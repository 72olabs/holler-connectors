package bus

import "time"

// RuntimeDetails contains allowlisted connection reports, not an attestation.
// It is kept separate from the agent-authored profile and never grants authority.
type RuntimeDetails struct {
	Model          string `json:"model,omitempty"`
	Reasoning      string `json:"reasoning,omitempty"`
	HarnessVersion string `json:"harness_version,omitempty"`
	// HostApp names the app the session runs in, from HostApps only. It is display
	// metadata read from the session's start environment, never an attestation.
	HostApp string `json:"host_app,omitempty"`
}

// HostApps lists the host app names a session may report.
var HostApps = map[string]bool{"terminal": true, "iterm": true, "t3code": true, "vscode": true, "cursor": true, "warp": true, "ghostty": true, "zed": true}

// WithKnownHostApp keeps HostApp only when it is in HostApps. A name this daemon
// doesn't know, such as one a newer connector adds, is dropped, never an error.
func (d RuntimeDetails) WithKnownHostApp() RuntimeDetails {
	if !HostApps[d.HostApp] {
		d.HostApp = ""
	}
	return d
}

func (d RuntimeDetails) Validate() error {
	for _, f := range []struct {
		name, value string
		max         int
	}{
		{"model", d.Model, 128}, {"reasoning", d.Reasoning, 32}, {"harness_version", d.HarnessVersion, 64},
	} {
		if f.value != "" {
			if err := ValidateTextIdentifier(f.name, f.value, f.max); err != nil {
				return err
			}
		}
	}
	return nil
}

type AgentClient struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// AgentDetails is explicitly self-reported, not a permission or runtime attestation.
type AgentDetails struct {
	DisplayName    string   `json:"display_name"`
	Model          string   `json:"model"`
	Reasoning      string   `json:"reasoning"`
	HarnessVersion string   `json:"harness_version"`
	Specialties    []string `json:"specialties"`
}

type AgentCard struct {
	Actor                 string           `json:"actor"`
	Aliases               []string         `json:"aliases"`
	Role                  string           `json:"role"`
	Accepts               []string         `json:"accepts"`
	Details               AgentDetails     `json:"details"`
	DetailsSource         string           `json:"details_source"`
	DetailsAt             *time.Time       `json:"details_at,omitempty"`
	DetailsCurrent        bool             `json:"details_current"`
	Runtime               RuntimeDetails   `json:"runtime"`
	Client                AgentClient      `json:"client"`
	RuntimeAt             *time.Time       `json:"runtime_at,omitempty"`
	Harness               string           `json:"harness"`
	Connected             bool             `json:"connected"`
	LastSeen              *time.Time       `json:"last_seen,omitempty"`
	Channels              []ProfileChannel `json:"channels"`
	VisibleMessages14Days int64            `json:"visible_messages_14_days"`
}

type ProfileChannel struct {
	ID    string `json:"channel_id"`
	Title string `json:"title"`
}

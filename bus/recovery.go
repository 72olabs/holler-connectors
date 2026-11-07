package bus

import "time"

// Recovery is an operator operation. A preview token identifies an exact
// pending connection; possession of the token alone is not approval.
type IdentityRecoveryPreview struct {
	Alias              string    `json:"alias"`
	Actor              string    `json:"actor"`
	Successor          string    `json:"successor"`
	RunID              string    `json:"run_id"`
	ManagedMemberships int       `json:"managed_memberships"`
	Blockers           []string  `json:"blockers"`
	Token              string    `json:"preflight_token,omitempty"`
	ExpiresAt          time.Time `json:"expires_at"`
}

type IdentityRecoveryResult struct {
	Actor         string         `json:"actor"`
	PreviousActor string         `json:"previous_actor"`
	RunID         string         `json:"run_id"`
	State         string         `json:"state"`
	WakeVerified  bool           `json:"wake_verified"`
	Superseded    []Registration `json:"-"`
}

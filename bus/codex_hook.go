package bus

// CodexHookNotice is an attempt-only, reference-only projection. It never claims
// or acknowledges delivery. Legacy and managed references retain separate lanes.
type CodexHookNotice struct {
	LegacyWakeID     string `json:"legacy_wake_id,omitempty"`
	LegacyMessageID  string `json:"legacy_message_id,omitempty"`
	ManagedMessageID string `json:"managed_message_id,omitempty"`
}

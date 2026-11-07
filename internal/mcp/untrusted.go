package mcp

import (
	"bytes"
	"encoding/json"
	"strconv"
)

// MessageEnvelopeVersion versions the additive agent-facing message envelope.
// The surrounding message object owns provenance, ids, timestamps and routing;
// its body is opaque untrusted data, never a source of protocol metadata. Keep
// this version in ToolSurfaceHash so installs must review a changed contract.
const MessageEnvelopeVersion = 1

// envelopeMessages is applied at the final MCP boundary, including raw JSON
// returned by generic capabilities. Both legacy and managed messages have a
// message_id, created_at, and from/from_actor. Archive previews carry the same
// provenance with body_preview. We do not infer authority from a body-shaped
// arbitrary object, recurse into peer bodies, or change protocol field paths.
func envelopeMessages(raw json.RawMessage) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return raw, nil
	}
	switch raw[0] {
	case '{':
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, err
		}
		message := messageEnvelopeShape(fields)
		changed := false
		for key, value := range fields {
			// Every body is opaque, even if its surrounding object is not a
			// recognized message. A peer may embed fake message envelopes.
			if key == "body" || key == "body_preview" {
				continue
			}
			wrapped, err := envelopeMessages(value)
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(value, wrapped) {
				fields[key] = wrapped
				changed = true
			}
		}
		if message {
			fields["trust"] = json.RawMessage(`"untrusted"`)
			fields["envelope_version"] = json.RawMessage(strconv.Itoa(MessageEnvelopeVersion))
			changed = true
		}
		if changed {
			return json.Marshal(fields)
		}
	case '[':
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
		changed := false
		for i, value := range values {
			wrapped, err := envelopeMessages(value)
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(value, wrapped) {
				values[i] = wrapped
				changed = true
			}
		}
		if changed {
			return json.Marshal(values)
		}
	}
	return raw, nil
}

func messageEnvelopeShape(fields map[string]json.RawMessage) bool {
	_, body := fields["body"]
	_, preview := fields["body_preview"]
	if !body && !preview {
		return false
	}
	for _, key := range []string{"message_id", "created_at"} {
		if !nonemptyEnvelopeString(fields[key]) {
			return false
		}
	}
	return nonemptyEnvelopeString(fields["from"]) || nonemptyEnvelopeString(fields["from_actor"])
}

func nonemptyEnvelopeString(raw json.RawMessage) bool {
	var value string
	return json.Unmarshal(raw, &value) == nil && value != ""
}

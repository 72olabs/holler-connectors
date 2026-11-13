package bus

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestManagedWakeIsOneReferenceOnlyLineAndNamesBatchDrain(t *testing.T) {
	notice := ManagedWakeText("msg_reference")
	if !strings.HasPrefix(notice, "[holler] Managed conversation message msg_reference.") || strings.ContainsAny(notice, "\r\n") {
		t.Fatal("monitor filter prefix/one-line contract changed", notice)
	}
	for _, instruction := range []string{"holler_channel_claim", "holler_channel_ack", "holler_channel_inbox", "already_handled", "do not poll", "setting in_reply_to to the message you answer", "Legacy bus_inbox/bus_ack do not consume managed deliveries"} {
		if !strings.Contains(notice, instruction) {
			t.Fatal("missing safe batch instruction", instruction)
		}
	}
}

func TestManagedWakeDigestIsCountOnlyAndBounded(t *testing.T) {
	plain := ManagedWakeText("msg_reference")
	if got := ManagedWakeTextWithDigest("msg_reference", 3); !strings.HasPrefix(got, plain) || !strings.Contains(got, "3 new or active threads (channel.digest)") || strings.ContainsAny(got, "\r\n") {
		t.Fatal("count hint changed the reference-only contract", got)
	}
	for _, count := range []int64{0, -1} {
		if got := ManagedWakeTextWithDigest("msg_reference", count); got != plain {
			t.Fatal("invalid or empty count appended", got)
		}
	}
}

func TestDigestWakeCountIsNotMessageJSON(t *testing.T) {
	raw, err := json.Marshal(Message{ID: "reference", DigestThreadCount: 9})
	if err != nil || strings.Contains(string(raw), "digest") {
		t.Fatal("recipient metadata serialized into message content", string(raw), err)
	}
	var m Message
	if err := json.Unmarshal([]byte(`{"message_id":"reference","digest_thread_count":999}`), &m); err != nil || m.DigestThreadCount != 0 {
		t.Fatal("message JSON supplied the recipient count", m, err)
	}
}

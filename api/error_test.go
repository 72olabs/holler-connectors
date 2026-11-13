package api

import (
	"errors"
	"testing"

	"github.com/72olabs/holler-connectors/bus"
)

func TestRPCErrorContextDoesNotDuplicateSentinelText(t *testing.T) {
	wire := &RPCError{
		Code:    "alias_conflict",
		Message: "aliasA: " + bus.ErrAliasConflict.Error(),
	}
	decoded := wire.Err()
	if !errors.Is(decoded, bus.ErrAliasConflict) {
		t.Fatalf("decoded error = %v", decoded)
	}
	if got, want := decoded.Error(), wire.Message; got != want {
		t.Fatalf("decoded error = %q, want %q", got, want)
	}
}

func TestIdentityLookalikeRPCErrorIsNotMarkedAsLocalRebind(t *testing.T) {
	decoded := (&RPCError{
		Code:    "invalid_request",
		Message: "actor: does not match the authenticated API session",
	}).Err()
	if !errors.Is(decoded, bus.ErrInvalid) {
		t.Fatalf("decoded error = %v, want ErrInvalid", decoded)
	}
	if errors.Is(decoded, bus.ErrIdentityRebound) {
		t.Fatalf("daemon-returned error gained local identity marker: %v", decoded)
	}
}

func TestRPCErrorErrKeepsUnknownCodesAsTheirMessage(t *testing.T) {
	decoded := (&RPCError{Code: "some_future_code", Message: "the daemon said no"}).Err()
	if decoded == nil || decoded.Error() != "the daemon said no" {
		t.Fatalf("decoded error = %v", decoded)
	}
	var nilRPC *RPCError
	if err := nilRPC.Err(); err == nil || err.Error() != "hollerd returned an empty error" {
		t.Fatalf("nil RPCError = %v", err)
	}
}

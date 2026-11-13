// Package api is the client of the Holler daemon's Unix socket: the wire types
// (Request, Response, RPCError, Identity, DaemonInfo), the length-prefixed JSON
// framing (WriteFrame, ReadFrame), the hello negotiation and its capability
// names, and the mapping from wire error codes to bus errors (RPCError.Err).
//
// It is supported public API: the Holler daemon imports it too. The daemon
// serves the same frames and adds its own operations on top of Client through
// Call and WithErrorDecoder, so this is its only socket client.
package api

// Package fdliveness reports when the peer of a pipe or socket has closed its
// end, so a long-running wake monitor can stop once the harness that started it
// has gone. It works on darwin and linux and reports nothing elsewhere.
//
// It is supported public API: the Holler daemon imports it too.
package fdliveness

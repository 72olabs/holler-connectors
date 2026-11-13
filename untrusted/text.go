// Package untrusted contains presentation-only framing for peer-controlled text.
// Rendered identity labels may use this framing, but its output must never be
// stored as a canonical identity or fed back into protocol/routing inputs.
//
// It is supported public API: the Holler daemon imports it too.
package untrusted

import (
	"strings"
	"unicode"
)

// HookText renders a peer-controlled display field as one bounded line. Control,
// format (including bidi), and line/paragraph-separator characters cannot create
// another hook frame. Literal framing brackets are escaped as visible data, so
// a name or title cannot imitate Holler's [holler] or role/XML delimiters.
// The original value remains unchanged in storage and protocol identity fields.
func HookText(value string) string {
	var b strings.Builder
	runes := 0
	for _, r := range value {
		if unicode.Is(unicode.Cf, r) {
			continue
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			if unicode.IsSpace(r) {
				r = ' '
			} else {
				continue
			}
		}
		if runes == 512 {
			b.WriteRune('…')
			break
		}
		runes++
		switch r {
		case '[':
			b.WriteString(`\u005b`)
		case ']':
			b.WriteString(`\u005d`)
		case '<':
			b.WriteString(`\u003c`)
		case '>':
			b.WriteString(`\u003e`)
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

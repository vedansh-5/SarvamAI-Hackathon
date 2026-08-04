package sdef

import "strings"

// Writable reports whether a property can be modified.
//
// Per /System/Library/DTDs/sdef.dtd, access is (r | w | rw) and #IMPLIED —
// absent means rw. 135 of Mail's 191 properties omit it, so this default
// carries most of the classification rather than being an edge case.
func (p Property) Writable() bool {
	switch p.Access {
	case "r":
		return false
	case "w", "rw":
		return true
	default:
		return true
	}
}

// AccessKind classifies a property Read or Write.
func (p Property) AccessKind() Access {
	if p.Writable() {
		return Write
	}
	return Read
}

// readOnlyCommands are the commands known not to mutate state. Anything absent
// is classified Write: a write misread as a read is a silent unconfirmed side
// effect, so the default must be the safe one.
var readOnlyCommands = map[string]struct{}{
	"count":  {},
	"exists": {},
	"get":    {},
}

// AccessKind classifies a command Read or Write.
func (c Command) AccessKind() Access {
	if _, ok := readOnlyCommands[strings.ToLower(c.Name)]; ok {
		return Read
	}
	return Write
}

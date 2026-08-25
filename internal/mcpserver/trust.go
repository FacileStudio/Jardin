package mcpserver

import (
	"fmt"

	"github.com/FacileStudio/Mycelium/internal/flow"
)

// The four states a flow's pin can be in, spelled the way "mycelium flow list
// --json" already spells them so a model and the human it asks for help are
// reading the same word for the same thing.
const (
	trustTrusted   = "trusted"
	trustNotPinned = "not pinned"
	trustChanged   = "CHANGED"
	trustUnknown   = "unknown"
)

// trustState reports whether this machine has approved a flow's exact content.
//
// An unreadable pin store reads as unknown rather than as trusted: the answer
// feeds Runnable, and guessing in the permissive direction there would advertise
// a flow as ready to run when nothing has approved it.
func trustState(f *flow.Flow) string {
	pinned, err := flow.TrustedChecksum(f.Name)
	switch {
	case err != nil:
		return trustUnknown
	case pinned == "":
		return trustNotPinned
	case pinned == f.Checksum:
		return trustTrusted
	default:
		return trustChanged
	}
}

// untrusted explains a refusal in the terms that lift it. cause is whatever
// stopped the trust check, or nil when the check simply said no.
//
// It is an ordinary Go error, which the SDK packs into a CallToolResult with
// IsError set: a tool execution error handed back to the model, not a protocol
// error that stops at the client. The spec draws that line so the model can
// correct itself, and here it cannot, because only a human at a terminal can
// pin a flow. So the text names the exact command and says who has to run it.
func untrusted(f *flow.Flow, cause error) error {
	pinned, err := flow.TrustedChecksum(f.Name)
	if cause == nil {
		cause = err
	}
	if cause != nil {
		return fmt.Errorf("flow %q cannot run: this machine's trust pins are unreadable (%v); "+
			"a human must repair %s", f.Name, cause, flow.TrustPath())
	}
	if pinned == "" {
		return fmt.Errorf("flow %q is not trusted on this machine; a human must read it and run: "+
			"mycelium flow trust %s", f.Name, f.Name)
	}
	return fmt.Errorf("flow %q changed since this machine approved it (approved %s, now %s); "+
		"a human must read it again and run: mycelium flow trust %s", f.Name, pinned, f.Checksum, f.Name)
}

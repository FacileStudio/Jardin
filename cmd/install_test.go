package cmd

import (
	"strings"
	"testing"

	"github.com/FacileStudio/Mycelium/internal/adapter"
)

// TestInstallHelpListsEveryRegisteredAdapter pins a dependency that is
// invisible in the source it protects: installLong runs in this package's
// init() and reads a registry the adapter package's own init() fills. Go
// orders those correctly, and nothing else here checks it — move the
// derivation to a package-level var, or into a package that does not import
// adapter, and the help text silently empties with every other test green.
func TestInstallHelpListsEveryRegisteredAdapter(t *testing.T) {
	names := adapter.Names()
	if len(names) < 2 {
		t.Fatalf("expected several registered adapters, got %v", names)
	}
	for _, name := range names {
		if !strings.Contains(installCmd.Long, name) {
			t.Errorf("`install --help` never mentions %q; Long = %q", name, installCmd.Long)
		}
	}
}

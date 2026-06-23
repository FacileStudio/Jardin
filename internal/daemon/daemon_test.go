package daemon

import (
	"strings"
	"testing"
)

func TestPlistContent(t *testing.T) {
	p := PlistContent("/usr/local/bin/ruche")
	for _, want := range []string{Label, "<string>/usr/local/bin/ruche</string>", "<string>daemon</string>", "<string>run</string>", "<key>RunAtLoad</key>"} {
		if !strings.Contains(p, want) {
			t.Errorf("plist missing %q", want)
		}
	}
}

func TestSystemdContent(t *testing.T) {
	if !strings.Contains(ServiceContent("/x/ruche"), "ExecStart=/x/ruche daemon run") {
		t.Error("service missing ExecStart")
	}
	if !strings.Contains(TimerContent(), "OnUnitActiveSec=300sec") {
		t.Error("timer missing interval")
	}
}

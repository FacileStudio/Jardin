package flow

import (
	"strings"
	"testing"
)

func parseSteps(t *testing.T, body string) error {
	t.Helper()
	_, err := Parse("t.yml", []byte("name: t\nsteps:\n"+body))
	return err
}

func TestParseRejectsBadNeeds(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"forward reference": {
			body: "  - name: first\n    needs:\n      V: second.stdout\n    run: 'true'\n  - name: second\n    run: 'true'\n",
			want: "form a cycle",
		},
		"unknown step": {
			body: "  - name: first\n    needs:\n      V: nowhere.stdout\n    run: 'true'\n",
			want: "is not a step in this flow",
		},
		"unknown field": {
			body: "  - name: first\n    run: 'true'\n  - name: second\n    needs:\n      V: first.duration\n    run: 'true'\n",
			want: "exposes only",
		},
		"not a reference": {
			body: "  - name: first\n    run: 'true'\n  - name: second\n    needs:\n      V: first\n    run: 'true'\n",
			want: "is not a <step>.<field> reference",
		},
		"own output": {
			body: "  - name: first\n    needs:\n      V: first.stdout\n    run: 'true'\n",
			want: "needs its own output",
		},
		"collides with env": {
			body: "  - name: first\n    run: 'true'\n  - name: second\n    env:\n      V: set\n    needs:\n      V: first.stdout\n    run: 'true'\n",
			want: "in both env and needs",
		},
		"unusable variable name": {
			body: "  - name: first\n    run: 'true'\n  - name: second\n    needs:\n      my-var: first.stdout\n    run: 'true'\n",
			want: "not a usable environment variable name",
		},
		"binds the token": {
			body: "  - name: first\n    run: 'true'\n  - name: second\n    needs:\n      JARDIN_TOKEN: first.stdout\n    run: 'true'\n",
			want: "may not bind JARDIN_TOKEN",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := parseSteps(t, tc.body)
			if err == nil {
				t.Fatalf("accepted a flow that should not parse")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestParseAcceptsBackwardReference(t *testing.T) {
	body := "  - name: first\n    run: 'true'\n  - name: second\n    needs:\n      V: first.stdout\n      CODE: first.exit_code\n    run: 'true'\n"
	if err := parseSteps(t, body); err != nil {
		t.Fatalf("rejected a valid flow: %v", err)
	}
}

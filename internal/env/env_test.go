package env

import (
	"strings"
	"testing"
)

func TestJournalBrowserURLValidation(t *testing.T) {
	cases := []struct {
		url     string
		key     string
		wantErr string
	}{
		{
			url:     "https://journal.facile.studio",
			key:     "some-key",
			wantErr: "must end in /api",
		},
		{
			url:     "https://journal.facile.studio/api",
			key:     "",
			wantErr: "must be set together",
		},
		{
			url:     "",
			key:     "some-key",
			wantErr: "must be set together",
		},
		{
			url:     "https://journal.facile.studio/api",
			key:     "some-key",
			wantErr: "",
		},
		{
			url:     "",
			key:     "",
			wantErr: "",
		},
	}

	for _, tc := range cases {
		cfg := Config{
			JournalBrowserURL: strings.TrimSuffix(tc.url, "/"),
			JournalBrowserKey: tc.key,
		}
		err := cfg.validate()
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("url=%q key=%q: unexpected error: %v", tc.url, tc.key, err)
			}
		} else {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("url=%q key=%q: got error %v, want substring %q", tc.url, tc.key, err, tc.wantErr)
			}
		}
	}
}

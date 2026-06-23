package selfupdate

import "testing"

func TestAssetName(t *testing.T) {
	cases := []struct {
		version, goos, goarch, want string
	}{
		{"0.2.0", "darwin", "arm64", "Ruche_0.2.0_darwin_arm64.tar.gz"},
		{"v0.2.0", "linux", "amd64", "Ruche_0.2.0_linux_amd64.tar.gz"},
		{"1.0.0", "darwin", "amd64", "Ruche_1.0.0_darwin_amd64.tar.gz"},
	}
	for _, c := range cases {
		if got := assetName(c.version, c.goos, c.goarch); got != c.want {
			t.Errorf("assetName(%q, %q, %q) = %q, want %q", c.version, c.goos, c.goarch, got, c.want)
		}
	}
}

func TestUpdateNeeded(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"dev", "0.2.0", true},
		{"0.2.0", "0.2.0", false},
		{"v0.2.0", "0.2.0", false},
		{"0.1.0", "0.2.0", true},
		{"0.2.0", "0.1.0", true},
	}
	for _, c := range cases {
		if got := updateNeeded(c.current, c.latest); got != c.want {
			t.Errorf("updateNeeded(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

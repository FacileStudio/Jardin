package server

import (
	"encoding/json"
	"testing"
)

func TestAdoptLegacyKeepsNookSettings(t *testing.T) {
	raw := []byte(`{"nook":{"enabled":true,"instance":"https://antenne.facile.studio","secret":"s3cret","user_email":"a@b.c"}}`)

	var settings Settings
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	settings.adoptLegacy()

	if !settings.Antenne.Enabled || settings.Antenne.Instance != "https://antenne.facile.studio" || settings.Antenne.Secret != "s3cret" {
		t.Fatalf("legacy settings lost: %+v", settings.Antenne)
	}
	if settings.Legacy != nil {
		t.Fatal("legacy block should be cleared once adopted")
	}
}

func TestAdoptLegacyDoesNotOverrideCurrent(t *testing.T) {
	raw := []byte(`{"antenne":{"enabled":true,"instance":"https://new"},"nook":{"enabled":true,"instance":"https://old"}}`)

	var settings Settings
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	settings.adoptLegacy()

	if settings.Antenne.Instance != "https://new" {
		t.Fatalf("current settings overwritten by legacy: %+v", settings.Antenne)
	}
}

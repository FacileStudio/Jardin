// Package env loads the Mycelium server's configuration from the process
// environment once, at startup, so a misconfigured server refuses to boot
// instead of running half-configured.
package env

import (
	"fmt"

	"github.com/FacileStudio/Mycelium/internal/config"
	troncenv "github.com/FacileStudio/tronc/env"
)

// DefaultPort is the port `mycelium serve` binds when PORT is unset.
const DefaultPort = 8420

// OIDC is the Authentik client configuration. A nil *OIDC means SSO is dormant.
type OIDC struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	SuccessURL   string
}

// Config is the server configuration.
//
// It embeds troncenv.Core so the shared fields carry the same names as in every
// other Facile API, but fills Core field by field rather than through
// troncenv.LoadCore: LoadCore requires DATABASE_URL and Mycelium has no database,
// it stores everything as markdown files on disk.
type Config struct {
	troncenv.Core

	DataDir  string
	Password string
	SSOOnly  bool
	OIDC     *OIDC
}

// Load reads and validates the configuration. Every error it returns is a
// reason not to start. Every field troncenv.Core grows has to be repeated
// here too, because this literal exists to skip troncenv.LoadCore — a field
// left out keeps its zero value, which for TrustedProxies means
// TRUSTED_PROXIES is ignored while the deployment panel shows it set.
func Load() (Config, error) {
	port, err := troncenv.Int("PORT", DefaultPort)
	if err != nil {
		return Config{}, err
	}
	if port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("env: PORT must be a valid TCP port, got %d", port)
	}

	ssoOnly, err := troncenv.Bool("SSO_ONLY", false)
	if err != nil {
		return Config{}, err
	}

	trustedProxies, err := troncenv.TrustedProxies()
	if err != nil {
		return Config{}, err
	}
	cdnProxies, cdnHeader := troncenv.CDN()

	cfg := Config{
		Core: troncenv.Core{
			TrustedProxies:     trustedProxies,
			CDNProxies:         cdnProxies,
			CDNHeader:          cdnHeader,
			AppEnv:             troncenv.ParseEnvironment(troncenv.String("APP_ENV", string(troncenv.Development))),
			Port:               port,
			LogLevel:           troncenv.String("LOG_LEVEL", "info"),
			CORSAllowedOrigins: troncenv.CORSOrigins(),
			JournalURL:         troncenv.String("JOURNAL_URL", ""),
			JournalToken:       troncenv.String("JOURNAL_TOKEN", ""),
		},
		DataDir:  config.DataDir(),
		Password: troncenv.String("PASSWORD", ""),
		SSOOnly:  ssoOnly,
	}

	if issuer := troncenv.String("OIDC_ISSUER", ""); issuer != "" {
		oidc := OIDC{Issuer: issuer, SuccessURL: troncenv.String("OIDC_SUCCESS_URL", "")}
		for _, required := range []struct {
			key   string
			field *string
		}{
			{"OIDC_CLIENT_ID", &oidc.ClientID},
			{"OIDC_CLIENT_SECRET", &oidc.ClientSecret},
			{"OIDC_REDIRECT_URL", &oidc.RedirectURL},
		} {
			if *required.field, err = troncenv.Required(required.key); err != nil {
				return Config{}, err
			}
		}
		cfg.OIDC = &oidc
	}

	return cfg, cfg.validate()
}

// validate rejects the combinations that leave the server with no working way
// to tell callers apart.
func (c Config) validate() error {
	if c.SSOOnly && c.OIDC == nil {
		return fmt.Errorf("env: SSO_ONLY=true requires OIDC_ISSUER, otherwise no caller can authenticate at all")
	}
	if c.IsProduction() && c.Password == "" && c.OIDC == nil {
		return fmt.Errorf("env: APP_ENV=production requires PASSWORD or OIDC_ISSUER, otherwise every request is served as admin")
	}
	return nil
}

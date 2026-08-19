package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"

	"github.com/FacileStudio/Mycelium/internal/env"
	"golang.org/x/oauth2"
)

const sessionTTL = 30 * 24 * time.Hour

// The query parameters a CLI adds to the login URL. The spellings are porte's,
// so a Facile CLI that already speaks one server speaks this one.
const (
	flowParam     = "flow"
	flowCLI       = "cli"
	portParam     = "port"
	cliStateParam = "cli_state"
)

const (
	// loginCodeTTL bounds how long the one-time code handed to a loopback
	// listener is worth anything. It only has to survive one local redirect.
	loginCodeTTL = 60 * time.Second

	maxPendingLoginCodes = 256

	// maxCLIState bounds the echoed nonce. It is opaque to the server, so
	// the only question worth asking is whether it is safe to reflect.
	maxCLIState = 128
)

type oidcRuntime struct {
	mu       sync.Mutex
	provider *gooidc.Provider
	config   oauth2.Config
	verifier *gooidc.IDTokenVerifier
}

// oidcFlow is everything the callback needs and the IdP must not see. It rides
// in one cookie rather than four: the state, the CLI marker, the loopback port
// and the CLI's nonce share a lifetime and a failure mode, and one cookie
// cannot arrive half set.
type oidcFlow struct {
	State    string `json:"s"`
	CLI      bool   `json:"c,omitempty"`
	Port     string `json:"p,omitempty"`
	CLIState string `json:"cs,omitempty"`
}

func (f oidcFlow) encode() (string, error) {
	payload, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeOIDCFlow(value string) (oidcFlow, bool) {
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return oidcFlow{}, false
	}
	var decoded oidcFlow
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return oidcFlow{}, false
	}
	return decoded, decoded.State != ""
}

// ensureOIDC performs provider discovery lazily so the server still boots
// when the IdP is unreachable; the first login attempt retries it.
func (s *Server) ensureOIDC() (*oidcRuntime, *env.OIDC, error) {
	cfg := s.OIDC
	if cfg == nil {
		return nil, nil, os.ErrNotExist
	}
	s.oidc.mu.Lock()
	defer s.oidc.mu.Unlock()
	if s.oidc.provider != nil {
		return &s.oidc, cfg, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	provider, err := gooidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, cfg, err
	}
	s.oidc.provider = provider
	s.oidc.config = oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{gooidc.ScopeOpenID, "email", "profile"},
	}
	s.oidc.verifier = provider.Verifier(&gooidc.Config{ClientID: cfg.ClientID})
	return &s.oidc, cfg, nil
}

func (s *Server) successURL(r *http.Request, cfg *env.OIDC) string {
	if cfg.SuccessURL != "" {
		return cfg.SuccessURL
	}
	return s.baseURL(r) + "/auth/callback"
}

// loopbackPort returns the port a CLI is listening on, or "" if the value is
// not a plausible port. Only the number is taken from the request: the host is
// hardcoded to loopback at redirect time, so this parameter cannot be turned
// into an open redirect.
func loopbackPort(value string) string {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1024 || port > 65535 {
		return ""
	}
	return value
}

// cliState accepts an opaque nonce the caller will recognise and nothing else.
// The character set is the one a nonce needs, so a value that tries to be a
// second query parameter or a header never reaches the redirect.
func cliState(value string) string {
	if value == "" || len(value) > maxCLIState {
		return ""
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return ""
		}
	}
	return value
}

package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/FacileStudio/Jardin/internal/env"
	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
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

// loginCode is a bearer credential for sixty seconds, so it is kept hashed and
// carries the identity rather than an already-minted session: a code nobody
// exchanges must not leave a live token behind.
type loginCode struct {
	Email   string
	Scope   string
	Expires time.Time
}

type loginCodeStore struct {
	mu    sync.Mutex
	codes map[string]loginCode
}

func newLoginCodeStore() *loginCodeStore {
	return &loginCodeStore{codes: make(map[string]loginCode)}
}

func (l *loginCodeStore) sweep(now time.Time) {
	for hash, code := range l.codes {
		if now.After(code.Expires) {
			delete(l.codes, hash)
		}
	}
}

func (l *loginCodeStore) create(hash, email, scope string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)
	if len(l.codes) >= maxPendingLoginCodes {
		return false
	}
	l.codes[hash] = loginCode{Email: email, Scope: scope, Expires: now.Add(loginCodeTTL)}
	return true
}

// consume returns the identity behind a code and removes it in the same
// critical section, which is what makes the code single-use under concurrency.
// The second return distinguishes an expired code from one that never existed
// so the caller can log a replay as the incident it is.
func (l *loginCodeStore) consume(hash string, now time.Time) (loginCode, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	code, ok := l.codes[hash]
	delete(l.codes, hash)
	l.sweep(now)
	if !ok || now.After(code.Expires) {
		return loginCode{}, false
	}
	return code, true
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

func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	// The CLI parameters are checked before anything else happens, so a
	// caller that cannot be redirected back is told now rather than after a
	// round trip through the identity provider.
	var pending oidcFlow
	if r.URL.Query().Get(flowParam) == flowCLI {
		pending.CLI = true
		pending.Port = loopbackPort(r.URL.Query().Get(portParam))
		pending.CLIState = cliState(r.URL.Query().Get(cliStateParam))
		if pending.Port == "" {
			httpjson.WriteError(w, apierrors.Invalid("flow=cli requires a loopback port between 1024 and 65535"))
			return
		}
	}

	rt, cfg, err := s.ensureOIDC()
	if err != nil {
		s.Log.Error("oidc: unavailable", slog.Any("error", err))
		writeStatusError(w, http.StatusServiceUnavailable, "unavailable", "sso unavailable")
		return
	}
	buf := make([]byte, 16)
	rand.Read(buf)
	pending.State = base64.RawURLEncoding.EncodeToString(buf)
	encoded, err := pending.encode()
	if err != nil {
		s.Log.Error("oidc: failed to start the login", slog.Any("error", err))
		writeStatusError(w, http.StatusInternalServerError, "internal", "could not start the login")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "oidc_state", Value: encoded, Path: "/", MaxAge: 600,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: strings.HasPrefix(s.successURL(r, cfg), "https://"),
	})
	http.Redirect(w, r, rt.config.AuthCodeURL(pending.State), http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	rt, cfg, err := s.ensureOIDC()
	if err != nil {
		writeStatusError(w, http.StatusServiceUnavailable, "unavailable", "sso unavailable")
		return
	}
	cookie, err := r.Cookie("oidc_state")
	if err != nil || cookie.Value == "" {
		httpjson.WriteError(w, apierrors.Invalid("invalid oauth2 state"))
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "oidc_state", Value: "", Path: "/", MaxAge: -1})

	pending, ok := decodeOIDCFlow(cookie.Value)
	if !ok || subtle.ConstantTimeCompare([]byte(pending.State), []byte(r.URL.Query().Get("state"))) != 1 {
		httpjson.WriteError(w, apierrors.Invalid("invalid oauth2 state"))
		return
	}

	token, err := rt.config.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		s.Log.Error("oidc: exchange failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Unauthorized("unauthorized"))
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		httpjson.WriteError(w, apierrors.Unauthorized("unauthorized"))
		return
	}
	idToken, err := rt.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		s.Log.Error("oidc: id_token verify failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Unauthorized("unauthorized"))
		return
	}
	var claims struct {
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Email == "" {
		httpjson.WriteError(w, apierrors.Invalid("email claim required"))
		return
	}
	name := claims.Name
	if name == "" {
		name = claims.PreferredUsername
	}

	user := s.upsertUser(claims.Email, name)
	scope := scopeUser
	if user.Admin {
		scope = scopeAdmin
	}

	if pending.CLI {
		// A CLI flow that lost its port must not silently fall back to
		// the web redirect: that would put a live session token in the
		// fragment of a page nobody asked for.
		if pending.Port == "" {
			httpjson.WriteError(w, apierrors.Invalid("the cli login lost its loopback port, start again"))
			return
		}
		s.issueLoginCode(w, r, user.Email, scope, pending.Port, pending.CLIState)
		return
	}

	session, err := s.mintSessionToken(user.Email, scope)
	if err != nil {
		s.Log.Error("oidc: session mint failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}

	dest := s.successURL(r, cfg) + "#token=" + session
	http.Redirect(w, r, dest, http.StatusFound)
}

// issueLoginCode ends the CLI half of the flow: a one-time code goes to the
// listener the CLI opened, never a token. The host is ours and only the port
// came from the request, so this redirect cannot be pointed off the machine.
func (s *Server) issueLoginCode(w http.ResponseWriter, r *http.Request, email, scope, port, nonce string) {
	code, err := generateToken()
	if err != nil {
		s.Log.Error("oidc: failed to issue a login code", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	if !s.loginCodes.create(hashToken(code), email, scope, time.Now().UTC()) {
		httpjson.WriteError(w, apierrors.RateLimited("too many pending logins"))
		return
	}

	query := url.Values{"code": {code}}
	// Echoed only when the CLI sent one: a binary installed before this
	// flow existed sends nothing, and must still complete its login.
	if nonce != "" {
		query.Set("state", nonce)
	}
	target := url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("127.0.0.1", port),
		Path:     "/",
		RawQuery: query.Encode(),
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target.String(), http.StatusFound)
}

// oidcExchange is the CLI's half: one-time code in, session token out. It is a
// token endpoint in everything but name, so it answers under the no-store that
// OAuth 2.1 §7.1 requires of any response carrying a credential.
func (s *Server) oidcExchange(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		httpjson.WriteError(w, apierrors.Invalid("missing code"))
		return
	}

	code, ok := s.loginCodes.consume(hashToken(req.Code), time.Now().UTC())
	if !ok {
		// A code presented twice is a replay, not a typo: it was worth
		// a session once, so either the CLI retried or someone else
		// has it. Either way it is worth a line in the log.
		s.Log.Warn("oidc: login code rejected", slog.String("ip", clientIP(r)))
		httpjson.WriteError(w, apierrors.Unauthorized("invalid or expired code"))
		return
	}

	// The same credential a browser login gets — same scope, same TTL —
	// under its own name. Sharing the browser's name would make a CLI login
	// sign the dashboard out and the next dashboard login break the daemon,
	// because minting a session evicts every token that shares its name.
	token, err := s.mintNamedSession("cli:"+code.Email, code.Email, code.Scope)
	if err != nil {
		s.Log.Error("oidc: session mint failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]string{"token": token})
}

// mintSessionToken issues an expiring browser session bound to a user; the
// token name is keyed by email so re-login rotates instead of accumulating.
func (s *Server) mintSessionToken(email, scope string) (string, error) {
	return s.mintNamedSession("session:"+email, email, scope)
}

func (s *Server) mintNamedSession(name, email, scope string) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}
	hash := hashToken(token)
	s.mu.Lock()
	for k, v := range s.tokens {
		if v.Name == name {
			delete(s.tokens, k)
		}
	}
	s.tokens[hash] = TokenInfo{
		Hash:      hash,
		Name:      name,
		Scope:     scope,
		UserEmail: email,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		ExpiresAt: time.Now().UTC().Add(sessionTTL).Format(time.RFC3339),
	}
	s.saveTokens()
	s.mu.Unlock()
	return token, nil
}

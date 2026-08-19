package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	apierrors "github.com/FacileStudio/tronc/errors"

	"github.com/FacileStudio/tronc/httpjson"
)

// oidcStart begins the browser flow. The CLI parameters are checked before
// anything else happens, so a caller that cannot be redirected back is told
// now rather than after a round trip through the identity provider.
func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
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
// The state is echoed only when the CLI sent one: a binary installed before
// this flow existed sends nothing, and must still complete its login.
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
// OAuth 2.1 §7.1 requires of any response carrying a credential. A code
// presented twice is a replay, not a typo — it was worth a session once, so
// either the CLI retried or someone else has it, and either way it is worth a
// line in the log. The token issued is the same credential a browser login
// gets, under its own name: sharing the browser's name would sign the dashboard
// out and the next dashboard login would break the daemon, because minting a
// session evicts every token that shares its name.
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
		s.Log.Warn("oidc: login code rejected", slog.String("ip", clientIP(r)))
		httpjson.WriteError(w, apierrors.Unauthorized("invalid or expired code"))
		return
	}

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

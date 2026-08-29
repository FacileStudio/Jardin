package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	apierrors "github.com/FacileStudio/tronc/errors"

	"github.com/FacileStudio/tronc/httpjson"
)

// oidcStart begins the browser flow. A CLI states the loopback port it is
// listening on, and a value that is not a port is dropped here rather than
// refused: a CLI with nowhere to redirect back to is the ordinary case on a
// machine whose browser lives elsewhere, and the callback hands that login a
// code to paste instead. Only the number survives loopbackPort, so nothing
// from the query string can become the host of the redirect home.
func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	var pending oidcFlow
	if r.URL.Query().Get(flowParam) == flowCLI {
		pending.CLI = true
		pending.Port = loopbackPort(r.URL.Query().Get(portParam))
		pending.CLIState = cliState(r.URL.Query().Get(cliStateParam))
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
	pending, ok := s.pendingOIDCFlow(w, r)
	if !ok {
		return
	}
	user, scope, ok := s.oidcIdentity(w, r, rt)
	if !ok {
		return
	}

	if pending.CLI {
		s.issueLoginCode(w, r, loginCodeGrant{
			Email: user.Email, Scope: scope, Port: pending.Port, Nonce: pending.CLIState,
		})
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

// pendingOIDCFlow recovers the login this callback belongs to and clears the
// cookie carrying it. The state comparison is constant time and is the only
// thing binding the callback to the browser that started the login, so both
// failures answer identically and neither reveals which one it was.
func (s *Server) pendingOIDCFlow(w http.ResponseWriter, r *http.Request) (oidcFlow, bool) {
	cookie, err := r.Cookie("oidc_state")
	if err != nil || cookie.Value == "" {
		httpjson.WriteError(w, apierrors.Invalid("invalid oauth2 state"))
		return oidcFlow{}, false
	}
	http.SetCookie(w, &http.Cookie{Name: "oidc_state", Value: "", Path: "/", MaxAge: -1})

	pending, ok := decodeOIDCFlow(cookie.Value)
	if !ok || subtle.ConstantTimeCompare([]byte(pending.State), []byte(r.URL.Query().Get("state"))) != 1 {
		httpjson.WriteError(w, apierrors.Invalid("invalid oauth2 state"))
		return oidcFlow{}, false
	}
	return pending, true
}

// oidcIdentity exchanges the authorization code, verifies the id_token it
// carries, and resolves the account it names, upserting so a first login
// creates one. The email claim is required: it is the identity, and a provider
// that omits it has told us nothing to key an account on.
//
// A failed identity never answers with the API's JSON envelope. This runs only
// under oidcCallback, which is a top-level browser navigation whichever flow
// started it, so a refusal arrives as a redirect to the login page with an
// error parameter, exactly as a success is a redirect to the app. The CLI flow
// is no exception: the browser is the same browser, and answering it in JSON
// dead-ended a failed `mycelium login` on an error object rendered as a web
// page. JSON belongs to POST /api/auth/oidc/exchange, where the caller really
// is a program. A refusal must cost what the acceptance costs, and neither the
// reason nor the status may reveal which step failed.
func (s *Server) oidcIdentity(w http.ResponseWriter, r *http.Request, rt *oidcRuntime) (User, string, bool) {
	token, err := rt.config.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		s.Log.Error("oidc: exchange failed", slog.Any("error", err))
		s.oidcFailureRedirect(w, r)
		return User{}, "", false
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		s.oidcFailureRedirect(w, r)
		return User{}, "", false
	}
	idToken, err := rt.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		s.Log.Error("oidc: id_token verify failed", slog.Any("error", err))
		s.oidcFailureRedirect(w, r)
		return User{}, "", false
	}
	var claims struct {
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Email == "" {
		s.oidcFailureRedirect(w, r)
		return User{}, "", false
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
	return user, scope, true
}

// oidcFailureRedirect ends a refused browser login on the login page rather
// than on a JSON envelope the browser cannot use. A success and a refusal are
// both a 302; the reason rides in an error query parameter the login page
// renders. The state cookie was already consumed by pendingOIDCFlow, so
// nothing from the failed attempt survives to poison the next one.
func (s *Server) oidcFailureRedirect(w http.ResponseWriter, r *http.Request) {
	dest := s.baseURL(r) + "/login?error=" + url.QueryEscape("sso")
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, dest, http.StatusFound)
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

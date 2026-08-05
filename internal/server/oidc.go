package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/FacileStudio/Mycelium/internal/env"
	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

const sessionTTL = 30 * 24 * time.Hour

type oidcRuntime struct {
	mu       sync.Mutex
	provider *gooidc.Provider
	config   oauth2.Config
	verifier *gooidc.IDTokenVerifier
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

func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	rt, cfg, err := s.ensureOIDC()
	if err != nil {
		s.Log.Error("oidc: unavailable", slog.Any("error", err))
		writeStatusError(w, http.StatusServiceUnavailable, "unavailable", "sso unavailable")
		return
	}
	buf := make([]byte, 16)
	rand.Read(buf)
	state := base64.RawURLEncoding.EncodeToString(buf)
	http.SetCookie(w, &http.Cookie{
		Name: "oidc_state", Value: state, Path: "/", MaxAge: 600,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: strings.HasPrefix(s.successURL(r, cfg), "https://"),
	})
	http.Redirect(w, r, rt.config.AuthCodeURL(state), http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	rt, cfg, err := s.ensureOIDC()
	if err != nil {
		writeStatusError(w, http.StatusServiceUnavailable, "unavailable", "sso unavailable")
		return
	}
	cookie, err := r.Cookie("oidc_state")
	if err != nil || cookie.Value == "" || cookie.Value != r.URL.Query().Get("state") {
		httpjson.WriteError(w, apierrors.Invalid("invalid oauth2 state"))
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "oidc_state", Value: "", Path: "/", MaxAge: -1})

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
	session, err := s.mintSessionToken(user.Email, scope)
	if err != nil {
		s.Log.Error("oidc: session mint failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}

	dest := s.successURL(r, cfg) + "#token=" + session
	http.Redirect(w, r, dest, http.StatusFound)
}

// mintSessionToken issues an expiring browser session bound to a user; the
// token name is keyed by email so re-login rotates instead of accumulating.
func (s *Server) mintSessionToken(email, scope string) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}
	hash := hashToken(token)
	name := "session:" + email
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

package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const sessionTTL = 30 * 24 * time.Hour

type oidcRuntime struct {
	mu       sync.Mutex
	provider *gooidc.Provider
	config   oauth2.Config
	verifier *gooidc.IDTokenVerifier
}

type oidcEnv struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	SuccessURL   string
}

func loadOIDCEnv() *oidcEnv {
	issuer := os.Getenv("OIDC_ISSUER")
	if issuer == "" {
		return nil
	}
	return &oidcEnv{
		Issuer:       issuer,
		ClientID:     os.Getenv("OIDC_CLIENT_ID"),
		ClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("OIDC_REDIRECT_URL"),
		SuccessURL:   os.Getenv("OIDC_SUCCESS_URL"),
	}
}

func ssoOnly() bool {
	return strings.ToLower(os.Getenv("SSO_ONLY")) == "true"
}

// ensureOIDC performs provider discovery lazily so the server still boots
// when the IdP is unreachable; the first login attempt retries it.
func (s *Server) ensureOIDC() (*oidcRuntime, *oidcEnv, error) {
	env := loadOIDCEnv()
	if env == nil {
		return nil, nil, os.ErrNotExist
	}
	s.oidc.mu.Lock()
	defer s.oidc.mu.Unlock()
	if s.oidc.provider != nil {
		return &s.oidc, env, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	provider, err := gooidc.NewProvider(ctx, env.Issuer)
	if err != nil {
		return nil, env, err
	}
	s.oidc.provider = provider
	s.oidc.config = oauth2.Config{
		ClientID:     env.ClientID,
		ClientSecret: env.ClientSecret,
		RedirectURL:  env.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{gooidc.ScopeOpenID, "email", "profile"},
	}
	s.oidc.verifier = provider.Verifier(&gooidc.Config{ClientID: env.ClientID})
	return &s.oidc, env, nil
}

func (s *Server) successURL(r *http.Request, env *oidcEnv) string {
	if env.SuccessURL != "" {
		return env.SuccessURL
	}
	return s.baseURL(r) + "/auth/callback"
}

func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	rt, env, err := s.ensureOIDC()
	if err != nil {
		log.Printf("oidc: unavailable: %v", err)
		http.Error(w, "sso unavailable", http.StatusServiceUnavailable)
		return
	}
	buf := make([]byte, 16)
	rand.Read(buf)
	state := base64.RawURLEncoding.EncodeToString(buf)
	http.SetCookie(w, &http.Cookie{
		Name: "oidc_state", Value: state, Path: "/", MaxAge: 600,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: strings.HasPrefix(s.successURL(r, env), "https://"),
	})
	http.Redirect(w, r, rt.config.AuthCodeURL(state), http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	rt, env, err := s.ensureOIDC()
	if err != nil {
		http.Error(w, "sso unavailable", http.StatusServiceUnavailable)
		return
	}
	cookie, err := r.Cookie("oidc_state")
	if err != nil || cookie.Value == "" || cookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid oauth2 state", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "oidc_state", Value: "", Path: "/", MaxAge: -1})

	token, err := rt.config.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		log.Printf("oidc: exchange failed: %v", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	idToken, err := rt.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		log.Printf("oidc: id_token verify failed: %v", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var claims struct {
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Email == "" {
		http.Error(w, "email claim required", http.StatusBadRequest)
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
		log.Printf("oidc: session mint failed: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	dest := s.successURL(r, env) + "#token=" + session
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

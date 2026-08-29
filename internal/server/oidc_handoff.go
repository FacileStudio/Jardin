package server

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

// handoffMarkup is the page every Facile app shows at the end of a CLI login.
// It is a file rather than a Go string on purpose: the same markup lives in
// porte, so a plain diff between the two repositories is what keeps the suite's
// pages identical, and nobody has to read Go quoting to see what the page says.
//
//go:embed handoff.html.tmpl
var handoffMarkup string

// handoffPage is the template's data. Warn recolours Body for a refusal; a
// successful hand-off leaves it false.
type handoffPage struct {
	AppName string
	LogoURL string
	Heading string
	Body    string
	Hint    string
	Code    string
	Warn    bool
}

// loginCodeGrant is everything a completed CLI login has to hand back: who
// logged in, what they may do, and how to reach them. Port is empty when the
// CLI has no loopback listener, which is what the paste page exists for. Nonce
// is empty for a binary installed before the flow echoed one.
type loginCodeGrant struct {
	Email string
	Scope string
	Port  string
	Nonce string
}

// issueLoginCode ends the CLI half of the flow: a one-time code goes back to
// the CLI, never a token. With a loopback port the code rides a redirect to the
// listener the CLI opened. The host is ours and only the port came from the
// request, so that redirect cannot be pointed off the machine. The nonce is
// echoed only when the CLI sent one: a binary installed before this flow
// existed sends nothing and must still complete its login.
//
// Without a port the code is shown for the person to carry to their terminal by
// hand. That is not an error case, it is the machine whose browser lives
// somewhere else: an SSH session, a container, a box with no desktop. Refusing
// it left `mycelium login` with no route through at all, where every
// porte-based app in the suite has offered a paste code for exactly this. The
// code is stored first and identically either way, same TTL and same single
// use, so the two hand-offs differ only in how the code travels.
func (s *Server) issueLoginCode(w http.ResponseWriter, r *http.Request, grant loginCodeGrant) {
	code, err := generateToken()
	if err != nil {
		s.Log.Error("oidc: failed to issue a login code", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	if !s.loginCodes.create(hashToken(code), grant.Email, grant.Scope, time.Now().UTC()) {
		httpjson.WriteError(w, apierrors.RateLimited("too many pending logins"))
		return
	}
	w.Header().Set("Cache-Control", "no-store")

	if grant.Port == "" {
		s.renderLoginCode(w, code)
		return
	}

	query := url.Values{"code": {code}}
	if grant.Nonce != "" {
		query.Set("state", grant.Nonce)
	}
	target := url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("127.0.0.1", grant.Port),
		Path:     "/",
		RawQuery: query.Encode(),
	}
	http.Redirect(w, r, target.String(), http.StatusFound)
}

// renderLoginCode shows the one-time code to the person instead of handing it
// to a socket. The response carries a credential and inherits the no-store the
// caller set: a paste code left in a shared browser cache is the same mistake
// as a token in a query string.
//
// LogoURL is deliberately empty. The template only draws the image when it has
// a URL, and the SvelteKit build under apps/client/static ships no logo.svg, so
// naming one would put a broken image on the page that ends a login.
func (s *Server) renderLoginCode(w http.ResponseWriter, code string) {
	body, err := renderHandoff(handoffPage{
		AppName: "Mycelium",
		Heading: "Signed in",
		Body:    "Paste this code into your terminal.",
		Hint:    fmt.Sprintf("The code works once and expires in %d seconds.", int(loginCodeTTL.Seconds())),
		Code:    code,
	})
	if err != nil {
		s.Log.Error("oidc: the hand-off page did not render", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(body)
}

// renderHandoff buffers the whole page before any of it reaches the wire. A
// template that fails partway through Execute has already written a broken
// document over a 200, and half a page carrying a credential is not something
// to hand a browser. Buffering keeps that failure recoverable as a 500.
//
// The template is parsed per render rather than once into a package variable.
// It is a few kilobytes on a path that runs once per interactive login, and the
// cost buys a parse error that arrives as a logged 500 instead of a panic at
// process start.
func renderHandoff(page handoffPage) ([]byte, error) {
	tmpl, err := template.New("handoff").Parse(handoffMarkup)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, page); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

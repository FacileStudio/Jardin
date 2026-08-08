package usage

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// The usage endpoint reads subscription limits, which only a subscription OAuth
// token can do: the long-lived sk-ant-oat01-… that `claude setup-token` mints. A
// standard sk-ant-api03-… API key cannot see 5h/7d limits at all, and the Admin
// API's usage endpoints need an org-scoped admin key individual accounts do not
// have.
const (
	TokenEnv    = "CLAUDE_CODE_OAUTH_TOKEN"
	TokenEnvAlt = "JARDIN_USAGE_TOKEN"

	// KeychainService is Jardin's own entry. Claude Code's
	// "Claude Code-credentials" entry is deliberately never read: refreshing
	// or rotating that token would sign the user out of their own CLI.
	KeychainService = "jardin-usage-token"
)

// TokenNotice is printed before a token is stored. Plain prose on purpose: it is
// a security statement, not a status message.
const TokenNotice = "This token is used only to read your subscription usage limits from Anthropic's usage endpoint.\n" +
	"Jardin never sends it to the Jardin server, never writes it into the synced data directory, and never uses it to make model requests.\n" +
	"Create one with `claude setup-token`."

// ErrNoKeychain means this machine has no secret store Jardin knows how to
// drive, so the caller must decide whether the plaintext fallback is acceptable.
var ErrNoKeychain = errors.New("no OS keychain backend available")

// ErrTokenRejected is a 401 from the usage endpoint: the token is present but
// no longer valid. Callers fall back to the statusline-recorded snapshot.
var ErrTokenRejected = errors.New("usage token rejected — run `jardin usage login` with a fresh `claude setup-token`")

// ResolveToken walks the sources in order of decreasing safety: the two env
// vars, then the OS keychain, then the plaintext ~/.jardin.yml field the caller
// passes in, which exists only for backwards compatibility. A keychain lookup
// that fails falls through quietly; a missing token is not an error here.
func ResolveToken(configured string) string {
	for _, key := range []string{TokenEnv, TokenEnvAlt} {
		if token := strings.TrimSpace(os.Getenv(key)); token != "" {
			return token
		}
	}
	if token, err := keychainGet(); err == nil && token != "" {
		return token
	}
	return strings.TrimSpace(configured)
}

func keychainGet() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("security", "find-generic-password", "-s", KeychainService, "-w").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	case "linux":
		out, err := exec.Command("secret-tool", "lookup", "service", KeychainService).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", ErrNoKeychain
}

// KeychainStore writes the token to the OS secret store, handing it over on
// stdin so it never lands in argv where `ps` and the shell history would see it.
func KeychainStore(token string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		account := os.Getenv("USER")
		if account == "" {
			account = "jardin"
		}
		cmd = exec.Command("security", "add-generic-password", "-U", "-s", KeychainService, "-a", account, "-w")
	case "linux":
		if _, err := exec.LookPath("secret-tool"); err != nil {
			return ErrNoKeychain
		}
		cmd = exec.Command("secret-tool", "store", "--label=Jardin usage token", "service", KeychainService)
	default:
		return ErrNoKeychain
	}
	cmd.Stdin = strings.NewReader(token + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New("keychain write failed: " + strings.TrimSpace(string(out)))
	}
	return nil
}

// KeychainDelete removes Jardin's entry. A missing entry is not an error.
func KeychainDelete() error {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("security", "delete-generic-password", "-s", KeychainService).Run()
		return nil
	case "linux":
		exec.Command("secret-tool", "clear", "service", KeychainService).Run()
		return nil
	}
	return ErrNoKeychain
}

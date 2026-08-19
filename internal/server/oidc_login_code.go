package server

import (
	"sync"
	"time"
)

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

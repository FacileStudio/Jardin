package server

import (
	"log"
	"net/http"
	"runtime/debug"
)

type recoveryWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *recoveryWriter) WriteHeader(code int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *recoveryWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// recoverer turns a panic in any handler into a logged stack trace plus a 500,
// so one bad request cannot take down the sync daemon.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &recoveryWriter{ResponseWriter: w}
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			log.Printf("panic: %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
			if !rw.wrote {
				http.Error(rw.ResponseWriter, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(rw, r)
	})
}

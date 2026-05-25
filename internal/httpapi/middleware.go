package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/iberkayC/case1back/internal/platform/apperror"
	"golang.org/x/time/rate"
)

// chain composes middleware so the leftmost runs first.
func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.ErrorContext(r.Context(), "panic",
					slog.Any("recovered", rec),
					slog.String("stack", string(debug.Stack())),
					slog.String("path", r.URL.Path),
				)
				writeError(w, r, apperror.Internal(fmt.Errorf("panic: %v", rec)))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota + 1
)

// RequestIDFromContext returns the per-request correlation id, or "" if absent.
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRequestID).(string)
	return v
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("rid-%d", time.Now().UnixNano())
	}
	return "rid-" + hex.EncodeToString(b[:])
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		slog.InfoContext(r.Context(), "http",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Int("bytes", rec.bytes),
			slog.Duration("dur", time.Since(start)),
			slog.String("request_id", RequestIDFromContext(r.Context())),
			slog.String("remote", clientIP(r)),
		)
	})
}

func contentTypeJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodDelete {
			next.ServeHTTP(w, r)
			return
		}
		if r.ContentLength == 0 {
			next.ServeHTTP(w, r)
			return
		}
		ct := r.Header.Get("Content-Type")
		if ct == "" || isJSONContentType(ct) {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, r, apperror.New(http.StatusUnsupportedMediaType,
			apperror.CodeUnsupportedMediaType, "expected application/json"))
	})
}

func isJSONContentType(ct string) bool {
	mediaType, _, err := mime.ParseMediaType(ct)
	return err == nil && mediaType == "application/json"
}

const maxBodyBytes = 256 * 1024

func bodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

const (
	bucketIdleEviction = 10 * time.Minute
	sweepEvery         = 1 * time.Minute
)

// rateLimiter throttles each client IP to rps with a token bucket of size
// burst. Idle buckets are swept periodically.
func rateLimiter(rps, burst int) func(http.Handler) http.Handler {
	if rps <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	var mu sync.Mutex
	buckets := map[string]*clientLimiter{}
	b := burst
	if b <= 0 {
		b = rps * 2
	}
	lastSweep := time.Now()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ip := clientIP(req)
			now := time.Now()
			mu.Lock()
			if now.Sub(lastSweep) > sweepEvery {
				for k, v := range buckets {
					if now.Sub(v.lastSeen) > bucketIdleEviction {
						delete(buckets, k)
					}
				}
				lastSweep = now
			}
			bk, ok := buckets[ip]
			if !ok {
				bk = &clientLimiter{
					limiter: rate.NewLimiter(rate.Limit(rps), b),
				}
				buckets[ip] = bk
			}
			bk.lastSeen = now
			mu.Unlock()
			if !bk.limiter.Allow() {
				writeError(w, req, apperror.New(http.StatusTooManyRequests,
					apperror.CodeRateLimited, "too many requests"))
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

func apiKeyGuard(key string) func(http.Handler) http.Handler {
	if key == "" {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			got := r.Header.Get("X-API-Key")
			if subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
				writeError(w, r, apperror.New(http.StatusUnauthorized,
					apperror.CodeUnauthorized, "missing or invalid API key"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP picks a client IP for rate limiting/logging, trusting
// X-Real-IP / X-Forwarded-For if set, then falling back to RemoteAddr.
func clientIP(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i >= 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

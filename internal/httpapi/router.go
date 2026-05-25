package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/iberkayC/case1back/internal/app"
	"github.com/iberkayC/case1back/internal/config"
	"github.com/iberkayC/case1back/internal/platform/apperror"
)

type RouterDeps struct {
	Handlers    *Handlers
	Idempotency *app.IdempotencyService
	Config      *config.Config
}

func NewRouter(d RouterDeps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", d.Handlers.Healthz)
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /docs/", swaggerUI)
	mux.HandleFunc("GET /openapi.yaml", openAPISpec)

	mux.HandleFunc("POST /api/v1/leagues", d.Handlers.CreateLeague)
	mux.HandleFunc("GET /api/v1/leagues", d.Handlers.ListLeagues)
	mux.HandleFunc("GET /api/v1/leagues/{leagueID}", d.Handlers.GetLeague)
	mux.HandleFunc("DELETE /api/v1/leagues/{leagueID}", d.Handlers.DeleteLeague)
	mux.HandleFunc("POST /api/v1/leagues/{leagueID}/reset", d.Handlers.ResetLeague)

	mux.HandleFunc("GET /api/v1/leagues/{leagueID}/teams", d.Handlers.GetTeams)
	mux.HandleFunc("GET /api/v1/leagues/{leagueID}/fixtures", d.Handlers.GetFixtures)
	mux.HandleFunc("GET /api/v1/leagues/{leagueID}/standings", d.Handlers.GetStandings)

	mux.HandleFunc("GET /api/v1/leagues/{leagueID}/matches", d.Handlers.ListMatches)
	mux.HandleFunc("GET /api/v1/leagues/{leagueID}/matches/{matchID}", d.Handlers.GetMatch)
	mux.HandleFunc("PATCH /api/v1/leagues/{leagueID}/matches/{matchID}", d.Handlers.EditMatch)

	mux.HandleFunc("POST /api/v1/leagues/{leagueID}/weeks/next",
		requireIdempotency(d.Idempotency, d.Handlers.PlayNextWeekReplayable))
	mux.HandleFunc("POST /api/v1/leagues/{leagueID}/play-all",
		requireIdempotency(d.Idempotency, d.Handlers.PlayAllReplayable))

	mux.HandleFunc("POST /api/v1/leagues/{leagueID}/predictions", d.Handlers.CreatePrediction)
	mux.HandleFunc("GET /api/v1/leagues/{leagueID}/predictions", d.Handlers.ListPredictions)
	mux.HandleFunc("GET /api/v1/leagues/{leagueID}/predictions/{runID}", d.Handlers.GetPrediction)

	mux.HandleFunc("POST /api/v1/leagues/{leagueID}/what-if", d.Handlers.WhatIf)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, apperror.NotFound(apperror.CodeNotFound, "route not found"))
	})

	mux.Handle("/", staticHandler(d.Config.App.WebDir))

	stack := chain(
		mux,
		recoverPanics,
		requestID,
		requestLogger,
		contentTypeJSON,
		bodyLimit,
		rateLimiter(d.Config.HTTP.RateLimitRPS, d.Config.HTTP.RateLimitBurst),
		apiKeyGuard(d.Config.App.APIKey),
	)
	return stack
}

// staticHandler serves files from webDir. If webDir is empty, missing, or
// the requested file does not exist, it falls back to a JSON 404 envelope so
// the API contract stays consistent for unknown routes. Requests to "/" are
// rewritten to "/index.html".
func staticHandler(webDir string) http.Handler {
	jsonNotFound := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, apperror.NotFound(apperror.CodeNotFound, "route not found"))
	})
	if webDir == "" {
		return jsonNotFound
	}
	info, err := os.Stat(webDir)
	if err != nil || !info.IsDir() {
		return jsonNotFound
	}
	root := filepath.Clean(webDir)
	fs := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reject path traversal defensively (FileServer also does).
		if strings.Contains(r.URL.Path, "..") {
			jsonNotFound.ServeHTTP(w, r)
			return
		}
		target := r.URL.Path
		if target == "/" || target == "" {
			target = "/index.html"
		}
		full := filepath.Join(root, filepath.FromSlash(target))
		if st, err := os.Stat(full); err != nil || st.IsDir() {
			jsonNotFound.ServeHTTP(w, r)
			return
		}
		fs.ServeHTTP(w, r)
	})
}

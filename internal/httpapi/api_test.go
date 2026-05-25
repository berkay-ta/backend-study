package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/iberkayC/case1back/internal/app"
	"github.com/iberkayC/case1back/internal/config"
	"github.com/iberkayC/case1back/internal/domain"
	"github.com/iberkayC/case1back/internal/httpapi"
	"github.com/iberkayC/case1back/internal/platform/random"
	"github.com/iberkayC/case1back/internal/predict/montecarlo"
	"github.com/iberkayC/case1back/internal/predict/validating"
	"github.com/iberkayC/case1back/internal/storage/fakerepo"
)

// newServer assembles the full HTTP stack against the in-memory fake store. The
// returned cleanup closes the test server.
func newServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	store := fakerepo.New()

	leagueQueries := app.NewLeagueQueryService(store.Leagues(), store.Matches())

	predictors := []app.Predictor{
		validating.New(montecarlo.New(500)),
	}
	predService := app.NewPredictionService(
		store.Leagues(), store.Matches(), store.Snapshots(), store.Predictions(),
		store, predictors,
	)
	leagueCommands := app.NewLeagueCommandService(store.Leagues(), store.Matches(), store, predService.MarkPriorRunsStale)
	seasonAdvancer := app.NewSeasonAdvancer(store.Leagues(), store.Matches(), store, random.NewSeeded(11), predService.MarkPriorRunsStale)
	idempotencyService := app.NewIdempotencyService(store, store.Idempotency())

	cfg := &config.Config{
		HTTP: config.HTTPConfig{RateLimitRPS: 0}, // disable rate limit in tests
	}
	router := httpapi.NewRouter(httpapi.RouterDeps{
		Handlers: &httpapi.Handlers{
			Queries:    leagueQueries,
			Commands:   leagueCommands,
			Seasons:    seasonAdvancer,
			Prediction: predService,
		},
		Idempotency: idempotencyService,
		Config:      cfg,
	})
	srv := httptest.NewServer(router)
	return srv, srv.Close
}

// envelope mirrors httpapi.Envelope but uses raw JSON for the data field so
// we can decode it per-test.
type envelope struct {
	Data json.RawMessage `json:"data"`
	Meta struct {
		RequestID     string `json:"request_id"`
		LeagueVersion *int64 `json:"league_version,omitempty"`
	} `json:"meta"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Fields  []struct {
			Field   string `json:"field"`
			Message string `json:"message"`
		} `json:"fields"`
	} `json:"error"`
}

func do(t *testing.T, srv *httptest.Server, method, path string, body any, headers map[string]string) (*http.Response, envelope) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, reqBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNoContent {
		return resp, envelope{}
	}
	var env envelope
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decode envelope (%s %s, status=%d): %v\nbody=%s",
				method, path, resp.StatusCode, err, string(raw))
		}
	}
	return resp, env
}

func idem(key string) map[string]string {
	return map[string]string{"Idempotency-Key": key}
}

// createTestLeague seeds a 4-team league with deterministic strengths and
// returns its ID for downstream tests.
func createTestLeague(t *testing.T, srv *httptest.Server) int64 {
	t.Helper()
	body := map[string]any{
		"name": "Test League",
		"teams": []map[string]any{
			{"name": "Alpha", "strength": 90},
			{"name": "Bravo", "strength": 70},
			{"name": "Charlie", "strength": 60},
			{"name": "Delta", "strength": 50},
		},
	}
	resp, env := do(t, srv, http.MethodPost, "/api/v1/leagues", body, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create league: status=%d body=%s", resp.StatusCode, env.Error)
	}
	var created struct {
		League struct {
			ID int64 `json:"id"`
		} `json:"league"`
	}
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.League.ID == 0 {
		t.Fatal("expected league id, got 0")
	}
	return created.League.ID
}

func TestHealthz(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("get healthz: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestSwaggerDocs(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()

	resp, err := http.Get(srv.URL + "/docs/")
	if err != nil {
		t.Fatalf("get docs: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(raw), "SwaggerUIBundle") || !strings.Contains(string(raw), "/openapi.yaml") {
		t.Fatalf("docs html missing swagger bootstrap: %s", string(raw))
	}

	resp, err = http.Get(srv.URL + "/openapi.yaml")
	if err != nil {
		t.Fatalf("get openapi: %v", err)
	}
	defer resp.Body.Close()
	raw, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(raw), "openapi: 3.1.0") {
		t.Fatalf("openapi spec missing header: %s", string(raw))
	}
}

func TestIdempotency_KeyRequired(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()
	leagueID := createTestLeague(t, srv)

	resp, env := do(t, srv, http.MethodPost,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/weeks/next", nil, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	if env.Error == nil || env.Error.Code != "idempotency_key_required" {
		t.Fatalf("expected idempotency_key_required, got %+v", env.Error)
	}

	resp, env = do(t, srv, http.MethodPost,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/play-all", nil, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	if env.Error == nil || env.Error.Code != "idempotency_key_required" {
		t.Fatalf("expected idempotency_key_required, got %+v", env.Error)
	}
}

func TestIdempotency_ReplaysWithoutAdvancingAgain(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()
	leagueID := createTestLeague(t, srv)
	path := "/api/v1/leagues/" + strconv.FormatInt(leagueID, 10) + "/weeks/next"

	resp1, env1 := do(t, srv, http.MethodPost, path, nil, idem("replay-week-1"))
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first call: status=%d err=%+v", resp1.StatusCode, env1.Error)
	}
	resp2, env2 := do(t, srv, http.MethodPost, path, nil, idem("replay-week-1"))
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("replay: status=%d err=%+v", resp2.StatusCode, env2.Error)
	}
	if resp2.Header.Get("X-Idempotent-Replay") != "true" {
		t.Fatalf("expected replay header")
	}
	if string(env1.Data) != string(env2.Data) {
		t.Fatalf("expected replayed data\nfirst=%s\nsecond=%s", env1.Data, env2.Data)
	}

	resp, env := do(t, srv, http.MethodGet,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/standings", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("standings: status=%d err=%+v", resp.StatusCode, env.Error)
	}
	var rows []struct {
		Played int `json:"played"`
	}
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode standings: %v", err)
	}
	for _, row := range rows {
		if row.Played != 1 {
			t.Fatalf("expected replay not to advance again; played=%d", row.Played)
		}
	}
}

func TestIdempotency_SameKeyDifferentRequestConflicts(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()
	leagueID := createTestLeague(t, srv)
	path := "/api/v1/leagues/" + strconv.FormatInt(leagueID, 10) + "/weeks/next"

	resp, env := do(t, srv, http.MethodPost, path, nil, idem("conflict-key"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first call: status=%d err=%+v", resp.StatusCode, env.Error)
	}
	resp, env = do(t, srv, http.MethodPost, path, map[string]any{}, idem("conflict-key"))
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	if env.Error == nil || env.Error.Code != "idempotency_key_conflict" {
		t.Fatalf("expected idempotency_key_conflict, got %+v", env.Error)
	}
}

func TestIdempotency_FailedMutationDoesNotCompleteKey(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()

	resp, env := do(t, srv, http.MethodPost,
		"/api/v1/leagues/999/weeks/next", nil, idem("failed-key"))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	if env.Error == nil || env.Error.Code != "league_not_found" {
		t.Fatalf("expected league_not_found, got %+v", env.Error)
	}

	leagueID := createTestLeague(t, srv)
	resp, env = do(t, srv, http.MethodPost,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/weeks/next",
		nil, idem("failed-key"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("key should be reusable after failed mutation: status=%d err=%+v", resp.StatusCode, env.Error)
	}
}

// TestReviewerFlow runs the eight-step Postman flow as a Go test against the
// in-memory store.
func TestReviewerFlow(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()

	leagueID := createTestLeague(t, srv)

	resp, env := do(t, srv, http.MethodGet, "/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/fixtures", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fixtures: status=%d", resp.StatusCode)
	}
	var weeks []struct {
		Week    int               `json:"week"`
		Matches []json.RawMessage `json:"matches"`
	}
	if err := json.Unmarshal(env.Data, &weeks); err != nil {
		t.Fatalf("decode fixtures: %v", err)
	}
	wantWeeks := domain.TotalWeeksForTeamCount(4)
	if len(weeks) != wantWeeks {
		t.Errorf("expected %d weeks, got %d", wantWeeks, len(weeks))
	}
	totalMatches := 0
	for _, w := range weeks {
		totalMatches += len(w.Matches)
	}
	if totalMatches != domain.TotalMatchesForTeamCount(4) {
		t.Errorf("expected %d fixtures, got %d", domain.TotalMatchesForTeamCount(4), totalMatches)
	}

	for i := 1; i <= 4; i++ {
		resp, _ := do(t, srv, http.MethodPost,
			"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/weeks/next",
			nil, idem("reviewer-week-"+strconv.Itoa(i)),
		)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("play week %d: status=%d", i, resp.StatusCode)
		}
	}

	resp, env = do(t, srv, http.MethodGet,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/standings", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("standings: status=%d", resp.StatusCode)
	}
	var rows []struct {
		TeamName string `json:"team_name"`
		Played   int    `json:"played"`
	}
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode standings: %v", err)
	}
	for _, r := range rows {
		if r.Played != 4 {
			t.Errorf("team %q played=%d, expected 4", r.TeamName, r.Played)
		}
	}

	// prediction is gated to week>=4; we played 4 weeks so it's open.
	resp, env = do(t, srv, http.MethodPost,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/predictions?strategy=monte_carlo",
		nil, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create prediction: status=%d err=%+v", resp.StatusCode, env.Error)
	}
	var pred struct {
		ID     int64 `json:"id"`
		Stale  bool  `json:"stale"`
		Result struct {
			Entries []struct {
				TeamName        string  `json:"team_name"`
				ChampionshipPct float64 `json:"championship_pct"`
				ExpectedPoints  float64 `json:"expected_points"`
			} `json:"entries"`
		} `json:"result"`
	}
	if err := json.Unmarshal(env.Data, &pred); err != nil {
		t.Fatalf("decode prediction: %v", err)
	}
	if len(pred.Result.Entries) != 4 {
		t.Errorf("expected 4 entries, got %d", len(pred.Result.Entries))
	}
	sum := 0.0
	for _, e := range pred.Result.Entries {
		sum += e.ChampionshipPct
	}
	if sum != 100 {
		t.Errorf("expected pct sum=100, got %.2f", sum)
	}
	predID := pred.ID

	// AI prediction without key gives 503 ai_disabled.
	resp, env = do(t, srv, http.MethodPost,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/predictions?strategy=ai_analyst",
		nil, nil)
	if resp.StatusCode != http.StatusServiceUnavailable || env.Error == nil || env.Error.Code != "ai_disabled" {
		t.Errorf("expected 503 ai_disabled; got status=%d err=%+v", resp.StatusCode, env.Error)
	}

	matchesResp, matchesEnv := do(t, srv, http.MethodGet,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/matches?week=1", nil, nil)
	if matchesResp.StatusCode != http.StatusOK {
		t.Fatalf("list matches: status=%d", matchesResp.StatusCode)
	}
	var wk1 []struct {
		ID     int64 `json:"id"`
		Played bool  `json:"played"`
	}
	if err := json.Unmarshal(matchesEnv.Data, &wk1); err != nil {
		t.Fatalf("decode week 1 matches: %v", err)
	}
	if len(wk1) == 0 {
		t.Fatal("expected at least one match in week 1")
	}
	resp, env = do(t, srv, http.MethodPost,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/what-if",
		map[string]any{
			"overrides": []map[string]any{
				{"match_id": wk1[0].ID, "home_goals": 7, "away_goals": 0},
			},
			"strategy": "monte_carlo",
		}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("what-if: status=%d err=%+v", resp.StatusCode, env.Error)
	}

	resp, env = do(t, srv, http.MethodPost,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/play-all",
		nil, idem("reviewer-play-all"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("play-all: status=%d err=%+v", resp.StatusCode, env.Error)
	}

	// editing a played match marks the prior prediction stale.
	resp, env = do(t, srv, http.MethodPatch,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/matches/"+strconv.FormatInt(wk1[0].ID, 10),
		map[string]any{"home_goals": 5, "away_goals": 5}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("edit match: status=%d err=%+v", resp.StatusCode, env.Error)
	}
	resp, env = do(t, srv, http.MethodGet,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/predictions/"+strconv.FormatInt(predID, 10), nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get prediction after edit: status=%d", resp.StatusCode)
	}
	var afterEdit struct {
		Stale bool `json:"stale"`
	}
	if err := json.Unmarshal(env.Data, &afterEdit); err != nil {
		t.Fatalf("decode prediction: %v", err)
	}
	if !afterEdit.Stale {
		t.Error("expected earlier prediction to be marked stale after match edit")
	}
}

func TestPredictionGated_BeforeWeek4(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()
	leagueID := createTestLeague(t, srv)

	resp, env := do(t, srv, http.MethodPost,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/predictions?strategy=monte_carlo",
		nil, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
	if env.Error == nil || env.Error.Code != "prediction_not_available" {
		t.Errorf("expected prediction_not_available, got %+v", env.Error)
	}
}

func TestPredictionGated_AfterSeasonComplete(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()
	leagueID := createTestLeague(t, srv)

	resp, env := do(t, srv, http.MethodPost,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/play-all",
		nil, idem("complete-before-prediction"),
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("play-all: status=%d err=%+v", resp.StatusCode, env.Error)
	}

	resp, env = do(t, srv, http.MethodPost,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/predictions?strategy=monte_carlo",
		nil, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
	if env.Error == nil || env.Error.Code != "league_already_complete" {
		t.Errorf("expected league_already_complete, got %+v", env.Error)
	}
}

func TestGetPrediction_WrongLeagueReturnsNotFound(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()
	leagueA := createTestLeague(t, srv)
	leagueB := createTestLeague(t, srv)

	for i := 1; i <= 4; i++ {
		resp, env := do(t, srv, http.MethodPost,
			"/api/v1/leagues/"+strconv.FormatInt(leagueA, 10)+"/weeks/next",
			nil, idem("wrong-league-week-"+strconv.Itoa(i)),
		)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("play week %d: status=%d err=%+v", i, resp.StatusCode, env.Error)
		}
	}

	resp, env := do(t, srv, http.MethodPost,
		"/api/v1/leagues/"+strconv.FormatInt(leagueA, 10)+"/predictions?strategy=monte_carlo",
		nil, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create prediction: status=%d err=%+v", resp.StatusCode, env.Error)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode prediction: %v", err)
	}

	resp, env = do(t, srv, http.MethodGet,
		"/api/v1/leagues/"+strconv.FormatInt(leagueB, 10)+"/predictions/"+strconv.FormatInt(created.ID, 10),
		nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	if env.Error == nil || env.Error.Code != "prediction_not_found" {
		t.Errorf("expected prediction_not_found, got %+v", env.Error)
	}
}

func TestEditUnplayedMatch(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()
	leagueID := createTestLeague(t, srv)

	_, env := do(t, srv, http.MethodGet,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/matches?week=1", nil, nil)
	var matches []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &matches); err != nil {
		t.Fatalf("decode matches: %v", err)
	}
	resp, env := do(t, srv, http.MethodPatch,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/matches/"+strconv.FormatInt(matches[0].ID, 10),
		map[string]any{"home_goals": 1, "away_goals": 0}, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
	if env.Error == nil || env.Error.Code != "match_not_played" {
		t.Errorf("expected match_not_played, got %+v", env.Error)
	}
}

func TestMutationResponsesIncludePersistedTimestamps(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()
	leagueID := createTestLeague(t, srv)

	resp, env := do(t, srv, http.MethodPost,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/weeks/next",
		nil, idem("timestamps-week-1"),
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("play next week: status=%d err=%+v", resp.StatusCode, env.Error)
	}
	var played struct {
		Matches []struct {
			ID       int64   `json:"id"`
			PlayedAt *string `json:"played_at"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(env.Data, &played); err != nil {
		t.Fatalf("decode play response: %v", err)
	}
	if len(played.Matches) == 0 {
		t.Fatal("expected played matches")
	}
	if played.Matches[0].PlayedAt == nil {
		t.Fatal("expected play response to include played_at")
	}

	resp, env = do(t, srv, http.MethodPatch,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/matches/"+strconv.FormatInt(played.Matches[0].ID, 10),
		map[string]any{"home_goals": 2, "away_goals": 2}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("edit match: status=%d err=%+v", resp.StatusCode, env.Error)
	}
	var edited struct {
		Match struct {
			EditedAt *string `json:"edited_at"`
		} `json:"match"`
	}
	if err := json.Unmarshal(env.Data, &edited); err != nil {
		t.Fatalf("decode edit response: %v", err)
	}
	if edited.Match.EditedAt == nil {
		t.Fatal("expected edit response to include edited_at")
	}
}

func TestCreateLeague_Validation(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()
	body := map[string]any{
		"name":  "Bad",
		"teams": []map[string]any{{"name": "A", "strength": 50}},
	}
	resp, env := do(t, srv, http.MethodPost, "/api/v1/leagues", body, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	if env.Error == nil || env.Error.Code != "validation_failed" {
		t.Errorf("expected validation_failed, got %+v", env.Error)
	}
	found := false
	for _, f := range env.Error.Fields {
		if strings.Contains(f.Field, "teams") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected field error mentioning teams, got %+v", env.Error.Fields)
	}
}

// firstMatchID returns the ID of the first week-1 fixture for a league.
func firstMatchID(t *testing.T, srv *httptest.Server, leagueID int64) int64 {
	t.Helper()
	_, env := do(t, srv, http.MethodGet,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/matches?week=1", nil, nil)
	var matches []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &matches); err != nil {
		t.Fatalf("decode matches: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one week-1 match")
	}
	return matches[0].ID
}

// TestEditMatch_MissingRequiredField: a PATCH body must carry both scores;
// omitting one is a 400 validation_failed naming the missing field.
func TestEditMatch_MissingRequiredField(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()
	leagueID := createTestLeague(t, srv)
	matchID := firstMatchID(t, srv, leagueID)

	resp, env := do(t, srv, http.MethodPatch,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/matches/"+strconv.FormatInt(matchID, 10),
		map[string]any{"home_goals": 2}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	if env.Error == nil || env.Error.Code != "validation_failed" {
		t.Errorf("expected validation_failed, got %+v", env.Error)
	}
	found := false
	for _, f := range env.Error.Fields {
		if f.Field == "away_goals" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected field error for away_goals, got %+v", env.Error.Fields)
	}
}

// TestWhatIf_OverrideMissingMatchID: every override must carry match_id;
// omitting it is a 400 validation_failed naming the missing field.
func TestWhatIf_OverrideMissingMatchID(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()
	leagueID := createTestLeague(t, srv)

	resp, env := do(t, srv, http.MethodPost,
		"/api/v1/leagues/"+strconv.FormatInt(leagueID, 10)+"/what-if",
		map[string]any{
			"overrides": []map[string]any{
				{"home_goals": 1, "away_goals": 0},
			},
		}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	if env.Error == nil || env.Error.Code != "validation_failed" {
		t.Errorf("expected validation_failed, got %+v", env.Error)
	}
	found := false
	for _, f := range env.Error.Fields {
		if f.Field == "overrides[0].match_id" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected field error for overrides[0].match_id, got %+v", env.Error.Fields)
	}
}

func TestNotFoundEnvelope(t *testing.T) {
	srv, cleanup := newServer(t)
	defer cleanup()
	resp, env := do(t, srv, http.MethodGet, "/api/v1/nonsense", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	if env.Error == nil || env.Error.Code != "not_found" {
		t.Errorf("expected not_found envelope, got %+v", env.Error)
	}
}

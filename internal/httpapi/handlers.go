package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/iberkayC/case1back/internal/app"
	"github.com/iberkayC/case1back/internal/domain"
	"github.com/iberkayC/case1back/internal/platform/apperror"
)

type Handlers struct {
	Queries    *app.LeagueQueryService
	Commands   *app.LeagueCommandService
	Seasons    *app.SeasonAdvancer
	Prediction *app.PredictionService
}

func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (h *Handlers) CreateLeague(w http.ResponseWriter, r *http.Request) {
	var body createLeagueReq
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	res, err := h.Commands.Create(r.Context(), body.toService())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeCreated(w, r, createLeagueRespDTO{
		League:   toLeagueDTO(res.League),
		Teams:    toTeamDTOs(res.Teams),
		Fixtures: toMatchDTOs(res.Fixtures),
	}, &res.League.Version)
}

func (h *Handlers) ListLeagues(w http.ResponseWriter, r *http.Request) {
	leagues, err := h.Queries.List(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]leagueDTO, len(leagues))
	for i, l := range leagues {
		out[i] = toLeagueDTO(l)
	}
	writeOK(w, r, out, nil)
}

func (h *Handlers) GetLeague(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	l, err := h.Queries.Get(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, toLeagueDTO(l), &l.Version)
}

func (h *Handlers) DeleteLeague(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := h.Commands.Delete(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ResetLeague(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	l, err := h.Commands.Reset(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, toLeagueDTO(l), &l.Version)
}

func (h *Handlers) GetTeams(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	teams, err := h.Queries.GetTeams(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, toTeamDTOs(teams), nil)
}

func (h *Handlers) GetFixtures(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	matches, err := h.Queries.GetFixtures(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, toFixturesByWeek(matches), nil)
}

func (h *Handlers) GetStandings(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	league, rows, err := h.Queries.StandingsWithLeague(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, toStandingRowDTOs(rows), &league.Version)
}

func (h *Handlers) ListMatches(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	league, err := h.Queries.Get(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	week := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("week")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > league.TotalWeeks {
			writeError(w, r, apperror.BadRequest("invalid week: must be 1.."+strconv.Itoa(league.TotalWeeks)))
			return
		}
		week = v
	}
	matches, err := h.Queries.ListMatches(r.Context(), id, week)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, toMatchDTOs(matches), nil)
}

func (h *Handlers) GetMatch(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	matchID, err := parseInt64Path(r, "matchID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	m, err := h.Queries.GetMatch(r.Context(), matchID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if m.LeagueID != leagueID {
		writeError(w, r, domain.ErrMatchNotFound)
		return
	}
	writeOK(w, r, toMatchDTO(m), nil)
}

func (h *Handlers) EditMatch(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	matchID, err := parseInt64Path(r, "matchID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var body editMatchReq
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if err := body.validate(); err != nil {
		writeError(w, r, err)
		return
	}
	match, league, rows, err := h.Commands.EditMatch(r.Context(), leagueID, matchID, *body.HomeGoals, *body.AwayGoals)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, editMatchRespDTO{
		Match:     toMatchDTO(match),
		Standings: toStandingRowDTOs(rows),
	}, &league.Version)
}

func (h *Handlers) PlayNextWeek(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	out, version, err := h.playNextWeekResponse(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, out, version)
}

func (h *Handlers) PlayAll(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	out, version, err := h.playAllResponse(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, out, version)
}

func (h *Handlers) PlayNextWeekReplayable(r *http.Request) (int, []byte, error) {
	id, err := parseInt64Path(r, "leagueID")
	if err != nil {
		return 0, nil, err
	}
	out, version, err := h.playNextWeekResponse(r.Context(), id)
	if err != nil {
		return 0, nil, err
	}
	body, err := marshalData(r, out, version)
	if err != nil {
		return 0, nil, apperror.Internal(err)
	}
	return http.StatusOK, body, nil
}

func (h *Handlers) PlayAllReplayable(r *http.Request) (int, []byte, error) {
	id, err := parseInt64Path(r, "leagueID")
	if err != nil {
		return 0, nil, err
	}
	out, version, err := h.playAllResponse(r.Context(), id)
	if err != nil {
		return 0, nil, err
	}
	body, err := marshalData(r, out, version)
	if err != nil {
		return 0, nil, apperror.Internal(err)
	}
	return http.StatusOK, body, nil
}

func (h *Handlers) playNextWeekResponse(ctx context.Context, id int64) (playWeekRespDTO, *int64, error) {
	week, league, standings, err := h.Seasons.PlayNextWeek(ctx, id)
	if err != nil {
		return playWeekRespDTO{}, nil, err
	}
	return playWeekRespDTO{
		Week:      week.Week,
		Matches:   toMatchDTOs(week.Matches),
		Standings: toStandingRowDTOs(standings),
		League:    toLeagueDTO(league),
	}, &league.Version, nil
}

func (h *Handlers) playAllResponse(ctx context.Context, id int64) (playAllRespDTO, *int64, error) {
	weeks, league, standings, err := h.Seasons.PlayAll(ctx, id)
	if err != nil {
		return playAllRespDTO{}, nil, err
	}
	weekDTOs := make([]weekPlayedDTO, len(weeks))
	for i, wp := range weeks {
		weekDTOs[i] = weekPlayedDTO{Week: wp.Week, Matches: toMatchDTOs(wp.Matches)}
	}
	return playAllRespDTO{
		Weeks:     weekDTOs,
		Standings: toStandingRowDTOs(standings),
		League:    toLeagueDTO(league),
	}, &league.Version, nil
}

func (h *Handlers) CreatePrediction(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	strategy := strings.TrimSpace(r.URL.Query().Get("strategy"))
	if strategy == "" {
		strategy = string(domain.StrategyMonteCarlo)
	}
	run, err := h.Prediction.Create(r.Context(), id, domain.Strategy(strategy))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeCreated(w, r, toPredictionRunDTO(run), &run.LeagueVersion)
}

func (h *Handlers) ListPredictions(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	runs, err := h.Prediction.ListByLeague(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, toPredictionRunDTOs(runs), nil)
}

func (h *Handlers) GetPrediction(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	runID, err := parseInt64Path(r, "runID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	run, err := h.Prediction.GetForLeague(r.Context(), leagueID, runID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, toPredictionRunDTO(run), &run.LeagueVersion)
}

func (h *Handlers) WhatIf(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Path(r, "leagueID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var body whatIfReq
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if err := body.validate(); err != nil {
		writeError(w, r, err)
		return
	}
	res, err := h.Prediction.WhatIf(r.Context(), body.toService(id))
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := whatIfRespDTO{Standings: toStandingRowDTOs(res.Standings)}
	if res.Prediction != nil {
		dto := toPredictionResultDTO(*res.Prediction)
		out.Prediction = &dto
	}
	writeOK(w, r, out, nil)
}

func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return apperror.BadRequest("missing request body")
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, http.ErrBodyReadAfterClose) {
			return apperror.BadRequest("body already consumed")
		}
		if tooLarge := requestBodyTooLargeError(err); tooLarge != nil {
			return tooLarge
		}
		return apperror.BadRequest("invalid JSON body")
	}
	// Reject trailing data: a second decode must hit EOF, else `{...}{...}`
	// or `{...} garbage` passes with only the first value parsed.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return apperror.BadRequest("body must contain a single JSON object")
	}
	return nil
}

func requestBodyTooLargeError(err error) *apperror.Error {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return apperror.New(http.StatusRequestEntityTooLarge,
			apperror.CodeRequestTooLarge, "request body too large")
	}
	return nil
}

func parseInt64Path(r *http.Request, name string) (int64, error) {
	raw := r.PathValue(name)
	if raw == "" {
		return 0, apperror.BadRequest("missing path parameter: " + name)
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, apperror.BadRequest("invalid integer path parameter: " + name)
	}
	if v <= 0 {
		return 0, apperror.BadRequest("path parameter must be positive: " + name)
	}
	return v, nil
}

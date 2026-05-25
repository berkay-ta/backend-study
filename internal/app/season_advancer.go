package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/iberkayC/case1back/internal/domain"
	"github.com/iberkayC/case1back/internal/platform/apperror"
)

type SeasonAdvancer struct {
	leagues    LeagueRepository
	matches    MatchRepository
	tx         TxRunner
	calc       domain.StandingCalculator
	rng        domain.Randomizer
	resultGen  domain.ResultGenerator
	onMutation MutationNotifier
}

// NewSeasonAdvancer requires onMutation; pass NoopMutationNotifier when
// staleness propagation is not needed (e.g. isolated unit tests).
func NewSeasonAdvancer(
	leagues LeagueRepository,
	matches MatchRepository,
	tx TxRunner,
	rng domain.Randomizer,
	onMutation MutationNotifier,
) *SeasonAdvancer {
	if onMutation == nil {
		panic("app.NewSeasonAdvancer: onMutation must not be nil")
	}
	return &SeasonAdvancer{leagues: leagues, matches: matches, tx: tx, rng: rng, onMutation: onMutation}
}

// PlayNextWeek plays every unplayed match in league.CurrentWeek and advances.
func (s *SeasonAdvancer) PlayNextWeek(ctx context.Context, leagueID int64) (WeekPlayedResult, domain.League, []domain.StandingRow, error) {
	var (
		played    WeekPlayedResult
		league    domain.League
		standings []domain.StandingRow
	)
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		l, err := s.leagues.GetForUpdate(ctx, leagueID)
		if err != nil {
			return err
		}
		if l.IsComplete() {
			return domain.ErrLeagueAlreadyComplete
		}
		ms, err := s.playWeek(ctx, l)
		if err != nil {
			return err
		}
		newWeek := l.CurrentWeek + 1
		if err := s.leagues.SetCurrentWeek(ctx, leagueID, newWeek); err != nil {
			return err
		}
		v, err := s.leagues.BumpVersion(ctx, leagueID)
		if err != nil {
			return err
		}
		l.CurrentWeek = newWeek
		l.Version = v
		league = l
		played = WeekPlayedResult{Week: ms[0].Week, Matches: ms}
		standings, err = s.standings(ctx, leagueID)
		if err != nil {
			return err
		}
		return s.onMutation(ctx, l.ID, l.Version)
	})
	if err != nil {
		return WeekPlayedResult{}, domain.League{}, nil, err
	}
	slog.InfoContext(ctx, "week_advanced",
		slog.Int64("league_id", league.ID),
		slog.Int("week", played.Week),
		slog.Int("matches", len(played.Matches)),
		slog.Int64("league_version", league.Version),
	)
	return played, league, standings, nil
}

// PlayAll plays every remaining week in a single transaction.
func (s *SeasonAdvancer) PlayAll(ctx context.Context, leagueID int64) ([]WeekPlayedResult, domain.League, []domain.StandingRow, error) {
	var (
		weeks     []WeekPlayedResult
		league    domain.League
		standings []domain.StandingRow
	)
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		l, err := s.leagues.GetForUpdate(ctx, leagueID)
		if err != nil {
			return err
		}
		if l.IsComplete() {
			return domain.ErrLeagueAlreadyComplete
		}
		for !l.IsComplete() {
			ms, err := s.playWeek(ctx, l)
			if err != nil {
				return err
			}
			weeks = append(weeks, WeekPlayedResult{Week: ms[0].Week, Matches: ms})
			l.CurrentWeek++
		}
		if err := s.leagues.SetCurrentWeek(ctx, leagueID, l.CurrentWeek); err != nil {
			return err
		}
		v, err := s.leagues.BumpVersion(ctx, leagueID)
		if err != nil {
			return err
		}
		l.Version = v
		league = l
		standings, err = s.standings(ctx, leagueID)
		if err != nil {
			return err
		}
		return s.onMutation(ctx, l.ID, l.Version)
	})
	if err != nil {
		return nil, domain.League{}, nil, err
	}
	slog.InfoContext(ctx, "league_played_to_end",
		slog.Int64("league_id", league.ID),
		slog.Int("weeks_played", len(weeks)),
		slog.Int64("league_version", league.Version),
	)
	return weeks, league, standings, nil
}

func (s *SeasonAdvancer) playWeek(ctx context.Context, league domain.League) ([]domain.Match, error) {
	weekMatches, err := s.matches.ListByWeek(ctx, league.ID, league.CurrentWeek)
	if err != nil {
		return nil, err
	}
	if len(weekMatches) == 0 {
		return nil, apperror.Internal(errors.New("no matches found for current week"))
	}
	teams, err := s.leagues.GetTeams(ctx, league.ID)
	if err != nil {
		return nil, err
	}
	byID := domain.TeamsByID(teams)

	out := make([]domain.Match, 0, len(weekMatches))
	for _, m := range weekMatches {
		if m.Played() {
			out = append(out, m)
			continue
		}
		home, ok1 := byID[m.HomeTeamID]
		away, ok2 := byID[m.AwayTeamID]
		if !ok1 || !ok2 {
			return nil, apperror.Internal(fmt.Errorf("team not found for match %d", m.ID))
		}
		hg, ag := s.resultGen.Play(home, away, s.rng)
		if err := s.matches.RecordResult(ctx, m.ID, hg, ag, false); err != nil {
			return nil, err
		}
		updated, err := s.matches.Get(ctx, m.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, updated)
	}
	return out, nil
}

func (s *SeasonAdvancer) standings(ctx context.Context, leagueID int64) ([]domain.StandingRow, error) {
	teams, err := s.leagues.GetTeams(ctx, leagueID)
	if err != nil {
		return nil, err
	}
	matches, err := s.matches.List(ctx, leagueID)
	if err != nil {
		return nil, err
	}
	return s.calc.Compute(teams, matches), nil
}

package app

import (
	"context"
	"log/slog"
	"strings"

	"github.com/iberkayC/case1back/internal/domain"
	"github.com/iberkayC/case1back/internal/platform/apperror"
)

type LeagueCommandService struct {
	leagues    LeagueRepository
	matches    MatchRepository
	tx         TxRunner
	fixtureGen domain.FixtureGenerator
	calc       domain.StandingCalculator
	onMutation MutationNotifier
}

// NewLeagueCommandService requires onMutation; pass NoopMutationNotifier
// when staleness propagation is not needed (in unit tests).
func NewLeagueCommandService(
	leagues LeagueRepository,
	matches MatchRepository,
	tx TxRunner,
	onMutation MutationNotifier,
) *LeagueCommandService {
	if onMutation == nil {
		panic("app.NewLeagueCommandService: onMutation must not be nil")
	}
	return &LeagueCommandService{leagues: leagues, matches: matches, tx: tx, onMutation: onMutation}
}

func (s *LeagueCommandService) Create(ctx context.Context, in CreateLeagueInput) (CreateLeagueResult, error) {
	if err := validateCreateInput(in); err != nil {
		return CreateLeagueResult{}, err
	}

	teams := make([]domain.Team, len(in.Teams))
	for i, t := range in.Teams {
		teams[i] = domain.Team{Name: strings.TrimSpace(t.Name), Strength: t.Strength}
	}

	var out CreateLeagueResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		totalWeeks := domain.TotalWeeksForTeamCount(len(teams))
		league, persistedTeams, err := s.leagues.Create(ctx, strings.TrimSpace(in.Name), totalWeeks, teams)
		if err != nil {
			return err
		}
		fixtures := s.fixtureGen.Generate(league.ID, persistedTeams)
		inserted, err := s.matches.InsertFixtures(ctx, fixtures)
		if err != nil {
			return err
		}
		version, err := s.leagues.BumpVersion(ctx, league.ID)
		if err != nil {
			return err
		}
		league.Version = version
		out = CreateLeagueResult{League: league, Teams: persistedTeams, Fixtures: inserted}
		return s.onMutation(ctx, league.ID, league.Version)
	})
	if err != nil {
		return CreateLeagueResult{}, err
	}
	slog.InfoContext(ctx, "league_created",
		slog.Int64("league_id", out.League.ID),
		slog.String("name", out.League.Name),
		slog.Int("teams", len(out.Teams)),
		slog.Int("fixtures", len(out.Fixtures)),
	)
	return out, nil
}

func (s *LeagueCommandService) Delete(ctx context.Context, id int64) error {
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		// Lock the row first so delete serializes with other mutations
		// (advance/reset/edit). GetForUpdate also returns not-found, so no
		// separate existence check is needed.
		if _, err := s.leagues.GetForUpdate(ctx, id); err != nil {
			return err
		}
		return s.leagues.Delete(ctx, id)
	})
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "league_deleted", slog.Int64("league_id", id))
	return nil
}

func (s *LeagueCommandService) Reset(ctx context.Context, leagueID int64) (domain.League, error) {
	var out domain.League
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		league, err := s.leagues.GetForUpdate(ctx, leagueID)
		if err != nil {
			return err
		}
		if err := s.matches.ClearAllResults(ctx, leagueID); err != nil {
			return err
		}
		if err := s.leagues.SetCurrentWeek(ctx, leagueID, 1); err != nil {
			return err
		}
		v, err := s.leagues.BumpVersion(ctx, leagueID)
		if err != nil {
			return err
		}
		league.CurrentWeek = 1
		league.Version = v
		out = league
		return s.onMutation(ctx, league.ID, league.Version)
	})
	if err != nil {
		return domain.League{}, err
	}
	slog.InfoContext(ctx, "league_reset",
		slog.Int64("league_id", out.ID),
		slog.Int64("league_version", out.Version),
	)
	return out, nil
}

func (s *LeagueCommandService) EditMatch(ctx context.Context, leagueID, matchID int64, homeGoals, awayGoals int) (domain.Match, domain.League, []domain.StandingRow, error) {
	if homeGoals < 0 || awayGoals < 0 {
		return domain.Match{}, domain.League{}, nil, apperror.ValidationFailed(
			apperror.FieldError{Field: "home_goals", Message: "must be non-negative"},
			apperror.FieldError{Field: "away_goals", Message: "must be non-negative"},
		)
	}

	var (
		match  domain.Match
		league domain.League
		rows   []domain.StandingRow
	)
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		l, err := s.leagues.GetForUpdate(ctx, leagueID)
		if err != nil {
			return err
		}
		m, err := s.matches.Get(ctx, matchID)
		if err != nil {
			return err
		}
		if m.LeagueID != leagueID {
			return domain.ErrMatchNotFound
		}
		if !m.Played() {
			return domain.ErrMatchNotPlayed
		}
		if err := s.matches.RecordResult(ctx, matchID, homeGoals, awayGoals, true); err != nil {
			return err
		}
		m, err = s.matches.Get(ctx, matchID)
		if err != nil {
			return err
		}
		v, err := s.leagues.BumpVersion(ctx, leagueID)
		if err != nil {
			return err
		}
		l.Version = v
		match = m
		league = l

		teams, err := s.leagues.GetTeams(ctx, leagueID)
		if err != nil {
			return err
		}
		all, err := s.matches.List(ctx, leagueID)
		if err != nil {
			return err
		}
		rows = s.calc.Compute(teams, all)
		return s.onMutation(ctx, l.ID, l.Version)
	})
	if err != nil {
		return domain.Match{}, domain.League{}, nil, err
	}
	slog.InfoContext(ctx, "match_edited",
		slog.Int64("league_id", league.ID),
		slog.Int64("match_id", match.ID),
		slog.Int("home_goals", homeGoals),
		slog.Int("away_goals", awayGoals),
		slog.Int64("league_version", league.Version),
	)
	return match, league, rows, nil
}

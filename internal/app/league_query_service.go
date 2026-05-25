package app

import (
	"context"

	"github.com/iberkayC/case1back/internal/domain"
)

type LeagueQueryService struct {
	leagues LeagueReader
	matches MatchReader
	calc    domain.StandingCalculator
}

func NewLeagueQueryService(leagues LeagueReader, matches MatchReader) *LeagueQueryService {
	return &LeagueQueryService{leagues: leagues, matches: matches}
}

func (s *LeagueQueryService) List(ctx context.Context) ([]domain.League, error) {
	return s.leagues.List(ctx)
}

func (s *LeagueQueryService) Get(ctx context.Context, id int64) (domain.League, error) {
	return s.leagues.Get(ctx, id)
}

func (s *LeagueQueryService) GetTeams(ctx context.Context, leagueID int64) ([]domain.Team, error) {
	if _, err := s.leagues.Get(ctx, leagueID); err != nil {
		return nil, err
	}
	return s.leagues.GetTeams(ctx, leagueID)
}

func (s *LeagueQueryService) GetFixtures(ctx context.Context, leagueID int64) ([]domain.Match, error) {
	if _, err := s.leagues.Get(ctx, leagueID); err != nil {
		return nil, err
	}
	return s.matches.List(ctx, leagueID)
}

func (s *LeagueQueryService) GetMatch(ctx context.Context, matchID int64) (domain.Match, error) {
	return s.matches.Get(ctx, matchID)
}

// ListMatches returns matches, optionally filtered by week (week<=0 means all).
func (s *LeagueQueryService) ListMatches(ctx context.Context, leagueID int64, week int) ([]domain.Match, error) {
	if _, err := s.leagues.Get(ctx, leagueID); err != nil {
		return nil, err
	}
	if week <= 0 {
		return s.matches.List(ctx, leagueID)
	}
	return s.matches.ListByWeek(ctx, leagueID, week)
}

func (s *LeagueQueryService) Standings(ctx context.Context, leagueID int64) ([]domain.StandingRow, error) {
	teams, matches, err := s.loadTeamsAndMatches(ctx, leagueID)
	if err != nil {
		return nil, err
	}
	return s.calc.Compute(teams, matches), nil
}

func (s *LeagueQueryService) StandingsWithLeague(ctx context.Context, leagueID int64) (domain.League, []domain.StandingRow, error) {
	league, err := s.leagues.Get(ctx, leagueID)
	if err != nil {
		return domain.League{}, nil, err
	}
	teams, err := s.leagues.GetTeams(ctx, leagueID)
	if err != nil {
		return domain.League{}, nil, err
	}
	matches, err := s.matches.List(ctx, leagueID)
	if err != nil {
		return domain.League{}, nil, err
	}
	return league, s.calc.Compute(teams, matches), nil
}

func (s *LeagueQueryService) loadTeamsAndMatches(ctx context.Context, leagueID int64) ([]domain.Team, []domain.Match, error) {
	if _, err := s.leagues.Get(ctx, leagueID); err != nil {
		return nil, nil, err
	}
	teams, err := s.leagues.GetTeams(ctx, leagueID)
	if err != nil {
		return nil, nil, err
	}
	matches, err := s.matches.List(ctx, leagueID)
	if err != nil {
		return nil, nil, err
	}
	return teams, matches, nil
}

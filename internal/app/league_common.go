package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/iberkayC/case1back/internal/domain"
	"github.com/iberkayC/case1back/internal/platform/apperror"
)

// MutationNotifier runs inside the mutation's tx before commit; an error rolls
// it back, keeping the version bump and side-effects (e.g. marking predictions
// stale) atomic.
type MutationNotifier func(ctx context.Context, leagueID, newVersion int64) error

func NoopMutationNotifier(context.Context, int64, int64) error { return nil }

type CreateLeagueInput struct {
	Name  string
	Teams []TeamInput
}

type TeamInput struct {
	Name     string
	Strength int
}

type CreateLeagueResult struct {
	League   domain.League
	Teams    []domain.Team
	Fixtures []domain.Match
}

type WeekPlayedResult struct {
	Week    int
	Matches []domain.Match
}

func validateCreateInput(in CreateLeagueInput) error {
	var fields []apperror.FieldError
	if strings.TrimSpace(in.Name) == "" {
		fields = append(fields, apperror.FieldError{Field: "name", Message: "must not be empty"})
	}
	if len(in.Teams) != domain.LeagueSize {
		fields = append(fields, apperror.FieldError{
			Field:   "teams",
			Message: fmt.Sprintf("must contain exactly %d teams", domain.LeagueSize),
		})
	}
	seen := map[string]bool{}
	for i, t := range in.Teams {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			fields = append(fields, apperror.FieldError{
				Field:   fmt.Sprintf("teams[%d].name", i),
				Message: "must not be empty",
			})
		}
		if seen[strings.ToLower(name)] {
			fields = append(fields, apperror.FieldError{
				Field:   fmt.Sprintf("teams[%d].name", i),
				Message: "duplicate team name",
			})
		}
		seen[strings.ToLower(name)] = true
		if t.Strength < 1 || t.Strength > 100 {
			fields = append(fields, apperror.FieldError{
				Field:   fmt.Sprintf("teams[%d].strength", i),
				Message: "must be between 1 and 100",
			})
		}
	}
	if len(fields) > 0 {
		return apperror.ValidationFailed(fields...)
	}
	return nil
}

package domain

import "errors"

var (
	ErrLeagueNotFound = errors.New("league not found")
	ErrMatchNotFound  = errors.New("match not found")

	ErrMatchNotPlayed         = errors.New("match has not been played yet")
	ErrLeagueAlreadyComplete  = errors.New("league is already complete")
	ErrPredictionNotAvailable = errors.New("predictions are available after week 4")
	ErrPredictionNotFound     = errors.New("prediction not found")
	ErrSnapshotNotFound       = errors.New("snapshot not found")

	ErrUnknownStrategy = errors.New("unknown prediction strategy")
	ErrAIDisabled      = errors.New("AI predictor is disabled (set OPENAI_API_KEY to enable)")
)

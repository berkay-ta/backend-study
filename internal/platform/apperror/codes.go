// Package apperror defines typed application errors and stable error codes.
package apperror

type Code string

const (
	CodeBadRequest           Code = "bad_request"
	CodeValidationFailed     Code = "validation_failed"
	CodeUnauthorized         Code = "unauthorized"
	CodeNotFound             Code = "not_found"
	CodeMethodNotAllowed     Code = "method_not_allowed"
	CodeConflict             Code = "conflict"
	CodeRateLimited          Code = "rate_limited"
	CodeInternal             Code = "internal_error"
	CodeUnsupportedMediaType Code = "unsupported_media_type"
	CodeRequestTooLarge      Code = "request_too_large"
	CodeBadGateway           Code = "bad_gateway"

	CodeLeagueNotFound         Code = "league_not_found"
	CodeMatchNotFound          Code = "match_not_found"
	CodePredictionNotFound     Code = "prediction_not_found"
	CodeSnapshotNotFound       Code = "snapshot_not_found"
	CodeMatchNotPlayed         Code = "match_not_played"
	CodeLeagueAlreadyComplete  Code = "league_already_complete"
	CodePredictionNotAvailable Code = "prediction_not_available"
	CodeIdempotencyKeyRequired Code = "idempotency_key_required"
	CodeIdempotencyKeyConflict Code = "idempotency_key_conflict"
	CodeUnknownStrategy        Code = "unknown_strategy"
	CodeAIDisabled             Code = "ai_disabled"
	CodeAIPredictionInvalid    Code = "ai_prediction_invalid"
)

package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/iberkayC/case1back/internal/domain"
	"github.com/iberkayC/case1back/internal/platform/apperror"
)

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	ae := toHTTPError(err)
	if ae.Status >= 500 {
		slog.ErrorContext(r.Context(), "request_error",
			slog.String("code", string(ae.Code)),
			slog.String("msg", ae.Msg),
			slog.Any("cause", ae.Cause),
			slog.String("path", r.URL.Path),
			slog.String("method", r.Method),
		)
	}
	writeJSON(w, ae.Status, Envelope{
		Meta: Meta{RequestID: RequestIDFromContext(r.Context())},
		Error: &ErrorEnvelope{
			Code:    ae.Code,
			Message: ae.Msg,
			Fields:  ae.Fields,
		},
	})
}

func toHTTPError(err error) *apperror.Error {
	switch {
	case errors.Is(err, domain.ErrLeagueNotFound):
		return apperror.NotFound(apperror.CodeLeagueNotFound, domain.ErrLeagueNotFound.Error())
	case errors.Is(err, domain.ErrMatchNotFound):
		return apperror.NotFound(apperror.CodeMatchNotFound, domain.ErrMatchNotFound.Error())
	case errors.Is(err, domain.ErrPredictionNotFound):
		return apperror.NotFound(apperror.CodePredictionNotFound, domain.ErrPredictionNotFound.Error())
	case errors.Is(err, domain.ErrSnapshotNotFound):
		return apperror.NotFound(apperror.CodeSnapshotNotFound, domain.ErrSnapshotNotFound.Error())
	case errors.Is(err, domain.ErrMatchNotPlayed):
		return apperror.Conflict(apperror.CodeMatchNotPlayed, domain.ErrMatchNotPlayed.Error())
	case errors.Is(err, domain.ErrLeagueAlreadyComplete):
		return apperror.Conflict(apperror.CodeLeagueAlreadyComplete, domain.ErrLeagueAlreadyComplete.Error())
	case errors.Is(err, domain.ErrPredictionNotAvailable):
		return apperror.Conflict(apperror.CodePredictionNotAvailable, domain.ErrPredictionNotAvailable.Error())
	case errors.Is(err, domain.ErrUnknownStrategy):
		return apperror.New(http.StatusBadRequest, apperror.CodeUnknownStrategy, domain.ErrUnknownStrategy.Error())
	case errors.Is(err, domain.ErrAIDisabled):
		return apperror.Unavailable(apperror.CodeAIDisabled, domain.ErrAIDisabled.Error())
	default:
		return apperror.FromError(err)
	}
}

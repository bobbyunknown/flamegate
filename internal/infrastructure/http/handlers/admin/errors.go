package admin

import (
	"errors"

	"github.com/danielgtaylor/huma/v2"

	domain "github.com/bobbyunknown/flamegate/internal/domain"
)

// MapError maps domain/application errors to Huma HTTP error responses.
func MapError(err error) error {
	if pe := domain.AsProviderError(err); pe != nil {
		switch pe.Kind {
		case domain.ErrAuth:
			return huma.Error401Unauthorized(pe.Message)
		case domain.ErrRateLimit, domain.ErrQuotaExhausted:
			return huma.Error429TooManyRequests(pe.Message)
		case domain.ErrBadRequest:
			return huma.Error400BadRequest(pe.Message)
		case domain.ErrCapability:
			return huma.Error422UnprocessableEntity(pe.Message)
		case domain.ErrPolicyBlocked:
			return huma.Error403Forbidden(pe.Message)
		case domain.ErrBudgetBlocked:
			return huma.Error429TooManyRequests(pe.Message)
		default:
			return huma.Error502BadGateway(pe.Message)
		}
	}
	// Domain sentinel errors
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return huma.Error404NotFound(err.Error())
	case errors.Is(err, domain.ErrConflict):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, domain.ErrValidation):
		return huma.Error422UnprocessableEntity(err.Error())
	case errors.Is(err, domain.ErrUnauthorized):
		return huma.Error401Unauthorized(err.Error())
	case errors.Is(err, domain.ErrForbidden):
		return huma.Error403Forbidden(err.Error())
	}
	return huma.Error500InternalServerError("internal error")
}

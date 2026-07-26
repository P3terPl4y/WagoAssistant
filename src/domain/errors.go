package domain

import "errors"

// Sentinel errors for the domain layer.
// These are mapped to HTTP status codes by the apperror package.
var (
	ErrNotFound          = errors.New("resource not found")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrForbidden         = errors.New("forbidden")
	ErrBotBlocked        = errors.New("bot is blocked")
	ErrPaymentPending    = errors.New("payment is pending")
	ErrPaymentInvalid    = errors.New("invalid payment status")
	ErrDuplicateResource = errors.New("resource already exists")
	ErrBotAlreadyActive  = errors.New("bot is already active")
	ErrBotLimitReached   = errors.New("bot limit reached")
	ErrInvalidInput      = errors.New("invalid input")
	ErrTimeout           = errors.New("operation timed out")
	ErrAllAIFailed       = errors.New("all AI providers failed")
)

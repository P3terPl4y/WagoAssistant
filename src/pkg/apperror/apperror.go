package apperror

import (
	"App/src/domain"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v3"
)

// AppError represents a structured application error with HTTP status context.
type AppError struct {
	Code    int    // HTTP status code
	Message string // User-facing message
	Err     error  // Underlying error (not exposed to client)
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// Constructor helpers

func BadRequest(msg string) *AppError {
	return &AppError{Code: 400, Message: msg}
}

func Unauthorized(msg string) *AppError {
	return &AppError{Code: 401, Message: msg}
}

func Forbidden(msg string) *AppError {
	return &AppError{Code: 403, Message: msg}
}

func NotFound(msg string) *AppError {
	return &AppError{Code: 404, Message: msg}
}

func TooManyRequests(msg string) *AppError {
	return &AppError{Code: 429, Message: msg}
}

func Internal(msg string, err error) *AppError {
	return &AppError{Code: 500, Message: msg, Err: err}
}

// Wrap converts a domain error into an AppError with appropriate HTTP status.
func Wrap(err error) *AppError {
	if err == nil {
		return nil
	}
	// Check for AppError first
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	// Map domain errors to HTTP status codes
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return &AppError{Code: 404, Message: "Recurso no encontrado", Err: err}
	case errors.Is(err, domain.ErrUnauthorized):
		return &AppError{Code: 401, Message: "No autenticado", Err: err}
	case errors.Is(err, domain.ErrForbidden):
		return &AppError{Code: 403, Message: "Acceso denegado", Err: err}
	case errors.Is(err, domain.ErrBotBlocked):
		return &AppError{Code: 403, Message: "El bot está bloqueado", Err: err}
	case errors.Is(err, domain.ErrPaymentPending):
		return &AppError{Code: 402, Message: "Pago pendiente", Err: err}
	case errors.Is(err, domain.ErrDuplicateResource):
		return &AppError{Code: 409, Message: "El recurso ya existe", Err: err}
	case errors.Is(err, domain.ErrBotLimitReached):
		return &AppError{Code: 400, Message: "Límite de bots alcanzado", Err: err}
	case errors.Is(err, domain.ErrInvalidInput):
		return &AppError{Code: 400, Message: "Datos inválidos", Err: err}
	default:
		return &AppError{Code: 500, Message: "Error interno del servidor", Err: err}
	}
}

// FiberErrorHandler is the centralized error handler for the Fiber app.
// It intercepts errors returned by handlers and sends structured JSON responses.
func FiberErrorHandler(c fiber.Ctx, err error) error {
	// Handle Fiber's own errors (404, etc.)
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return c.Status(fe.Code).JSON(fiber.Map{"error": fe.Message})
	}
	// Handle AppErrors
	var appErr *AppError
	if errors.As(err, &appErr) {
		return c.Status(appErr.Code).JSON(fiber.Map{"error": appErr.Message})
	}
	// Fallback
	return c.Status(500).JSON(fiber.Map{"error": "Ha ocurrido un error. Inténtelo de nuevo más tarde."})
}

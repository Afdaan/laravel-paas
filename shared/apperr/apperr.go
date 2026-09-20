// ===========================================
// Application Errors
// ===========================================
// Defined custom error types for consistent API responses
// ===========================================
package apperr

import "fmt"

type AppError struct {
	Code       string      `json:"code"`
	Message    string      `json:"message"`
	HTTPStatus int         `json:"-"`
	Details    interface{} `json:"details,omitempty"`
	Cause      error       `json:"-"`
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

func New(status int, code, message string) *AppError {
	return &AppError{
		HTTPStatus: status,
		Code:       code,
		Message:    message,
	}
}

// Standard Error Definitions
var (
	ErrNotFound      = New(404, "NOT_FOUND", "The requested resource was not found")
	ErrUnauthorized  = New(401, "UNAUTHORIZED", "Unauthorized access")
	ErrForbidden     = New(403, "FORBIDDEN", "Insufficient permissions")
	ErrBadRequest    = New(400, "BAD_REQUEST", "Invalid request parameters")
	ErrInternal      = New(500, "INTERNAL_ERROR", "An unexpected error occurred")
	ErrConflict      = New(409, "CONFLICT", "A conflict occurred with existing data")
	ErrUnprocessable = New(422, "UNPROCESSABLE", "The request could not be processed due to validation errors")
)

// Specific Business Errors
func NewBadRequest(message string) *AppError {
	return New(400, "BAD_REQUEST", message)
}

func NewNotFound(resource string, id interface{}) *AppError {
	return New(404, "NOT_FOUND", fmt.Sprintf("%s with identity %v not found", resource, id))
}

func NewValidationErr(details interface{}) *AppError {
	err := New(422, "VALIDATION_ERROR", "Validation failed for the input")
	err.Details = details
	return err
}

func NewRateLimited(message string, retryAfterSec int) *AppError {
	err := New(429, "RATE_LIMITED", message)
	if retryAfterSec > 0 {
		err.Details = map[string]interface{}{
			"retry_after": retryAfterSec,
		}
	}
	return err
}

func NewSecretDecryptionFailed(message string, cause error) *AppError {
	err := New(409, "SECRET_DECRYPTION_FAILED", message)
	err.Cause = cause
	return err
}

func NewSecretDecryptionUnavailable(cause error) *AppError {
	return NewSecretDecryptionFailed("This secret is unavailable and needs administrator attention.", cause)
}

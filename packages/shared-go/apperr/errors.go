package apperr

import "fmt"

type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *AppError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func New(code, message string, status int) *AppError {
	return &AppError{Code: code, Message: message, Status: status}
}

var (
	ErrUnauthorized = New("unauthorized", "authentication required", 401)
	ErrForbidden    = New("forbidden", "insufficient permissions", 403)
	ErrNotFound     = New("not_found", "resource not found", 404)
	ErrConflict     = New("conflict", "resource conflict", 409)
)

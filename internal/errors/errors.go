package errors

import (
    "net/http"
)

type ErrorType string

const (
    ErrorTypeNotFound     ErrorType = "NOT_FOUND"
    ErrorTypeValidation   ErrorType = "VALIDATION"
    ErrorTypeInternal     ErrorType = "INTERNAL"
    ErrorTypeUnauthorized ErrorType = "UNAUTHORIZED"
)

type Error struct {
    Type    ErrorType `json:"type"`
    Message string    `json:"message"`
    Code    int      `json:"code"`
    Details any      `json:"details,omitempty"`
}

func (e *Error) Error() string {
    return e.Message
}

func NotFound(message string) *Error {
    return &Error{
        Type:    ErrorTypeNotFound,
        Message: message,
        Code:    http.StatusNotFound,
    }
}

func ValidationError(message string, details any) *Error {
    return &Error{
        Type:    ErrorTypeValidation,
        Message: message,
        Code:    http.StatusBadRequest,
        Details: details,
    }
}
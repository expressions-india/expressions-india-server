package utils

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type Response struct {
	Success bool       `json:"success"`
	Data    any        `json:"data,omitempty"`
	Error   *ErrorInfo `json:"error,omitempty"`
	Meta    *Meta      `json:"meta,omitempty"`
}

type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Meta struct {
	Page       int   `json:"page,omitempty"`
	PerPage    int   `json:"perPage,omitempty"`
	Total      int64 `json:"total,omitempty"`
	TotalPages int   `json:"totalPages,omitempty"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    data,
	})
}

func PaginatedOK(c *gin.Context, data any, meta Meta) {
	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    data,
		Meta:    &meta,
	})
}

// Send a error msg to the user.
//
// Takes in gin.Context, status code and message.
func Fail(c *gin.Context, status int, code, message string) {
	c.JSON(status, Response{
		Success: false,
		Error:   &ErrorInfo{Code: code, Message: message},
	})
}

// FailInternal logs the full error server-side and returns a generic message to the client.
// Use this instead of Fail for 5xx errors to avoid leaking internal details.
func FailInternal(c *gin.Context, code string, clientMsg string, err error) {
	log.Printf("[%s] %s: %v", code, clientMsg, err)
	Fail(c, http.StatusInternalServerError, code, clientMsg)
}

// FormatBindError converts a Gin/validator binding error into a user-friendly
// message. Falls back to err.Error() if the error isn't a validator one.
func FormatBindError(err error) string {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return err.Error()
	}
	msgs := make([]string, 0, len(ve))
	for _, fe := range ve {
		field := humanizeField(fe.Field())
		switch fe.Tag() {
		case "required":
			msgs = append(msgs, fmt.Sprintf("%s is required", field))
		case "email":
			msgs = append(msgs, fmt.Sprintf("%s must be a valid email", field))
		case "url":
			msgs = append(msgs, fmt.Sprintf("%s must be a valid URL", field))
		case "min":
			msgs = append(msgs, fmt.Sprintf("%s must be at least %s", field, fe.Param()))
		case "max":
			msgs = append(msgs, fmt.Sprintf("%s must be at most %s", field, fe.Param()))
		default:
			msgs = append(msgs, fmt.Sprintf("%s is invalid", field))
		}
	}
	return strings.Join(msgs, "; ")
}

// humanizeField turns "StartDate" → "Start date".
func humanizeField(name string) string {
	var b strings.Builder
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte(' ')
			b.WriteRune(unicode.ToLower(r))
		} else if i == 0 {
			b.WriteRune(unicode.ToUpper(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SafeTotalPages calculates total pages without dividing by zero.
func SafeTotalPages(total int64, limit int) int {
	if limit <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(limit)))
}

type ValidationError struct {
	Msg string
}

func (e ValidationError) Error() string {
	return e.Msg
}

func NewValidationError(msg string) error {
	return ValidationError{Msg: msg}
}

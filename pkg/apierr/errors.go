package apierr

import "errors"

// Sentinel errors for typed error handling across layers.
var (
	ErrNotFound      = errors.New("not found")
	ErrDuplicate     = errors.New("duplicate")
	ErrValidation    = errors.New("validation failed")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrAlreadyPaid   = errors.New("already paid")
	ErrActiveClient  = errors.New("active client cannot be deleted")
	ErrCannotBeEmpty = errors.New("cannot be empty")
)

// ValidationError wraps a validation message.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string       { return e.Message }
func (e *ValidationError) Is(target error) bool { return target == ErrValidation }

// NotFoundError wraps a not-found message.
type NotFoundError struct{ Resource string }

func (e *NotFoundError) Error() string       { return e.Resource + " not found" }
func (e *NotFoundError) Is(target error) bool { return target == ErrNotFound }

// DuplicateError wraps a duplicate message.
type DuplicateError struct{ Message string }

func (e *DuplicateError) Error() string       { return e.Message }
func (e *DuplicateError) Is(target error) bool { return target == ErrDuplicate }

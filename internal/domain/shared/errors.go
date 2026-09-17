package shared

import "errors"

// Sentinel domain errors. Application/interface layers translate these into
// transport-specific responses (HTTP status codes, CLI exit codes, etc.).
var (
	ErrNotFound         = errors.New("resource not found")
	ErrAlreadyExists    = errors.New("resource already exists")
	ErrConflict         = errors.New("resource conflict")
	ErrInvalidInput     = errors.New("invalid input")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbidden        = errors.New("forbidden")
	ErrNotImplemented   = errors.New("not implemented")
	ErrCapabilityUnmet  = errors.New("capability not available")
	ErrPreconditionFail = errors.New("precondition failed")
)

// Page describes a bounded, offset-based page request.
type Page struct {
	Limit  int
	Offset int
}

// DefaultPage returns sane defaults when a caller supplies none.
func DefaultPage() Page {
	return Page{Limit: 50, Offset: 0}
}

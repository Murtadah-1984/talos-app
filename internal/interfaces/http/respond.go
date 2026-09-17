// Package http wires the REST API: routing, middleware, and handlers for
// every resource in §27 of the product spec. Handlers translate between
// transport concerns (JSON, path/query params, status codes) and the
// application/domain layers; they contain no business logic themselves.
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

type errorBody struct {
	Error string `json:"error"`
}

// writeError maps domain sentinel errors to HTTP status codes uniformly
// across every handler, so callers get a consistent status/JSON shape.
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, shared.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, shared.ErrAlreadyExists), errors.Is(err, shared.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, shared.ErrInvalidInput):
		status = http.StatusBadRequest
	case errors.Is(err, shared.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, shared.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, shared.ErrNotImplemented), errors.Is(err, shared.ErrCapabilityUnmet):
		status = http.StatusNotImplemented
	case errors.Is(err, shared.ErrPreconditionFail):
		status = http.StatusPreconditionFailed
	}
	writeJSON(w, status, errorBody{Error: err.Error()})
}

func decodeJSON(r *http.Request, v any) error {
	defer func() { _ = r.Body.Close() }()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errors.Join(shared.ErrInvalidInput, err)
	}
	return nil
}

func pageFromQuery(r *http.Request) shared.Page {
	page := shared.DefaultPage()
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		if n, err := parsePositiveInt(v); err == nil {
			page.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := parsePositiveInt(v); err == nil {
			page.Offset = n
		}
	}
	return page
}

func parsePositiveInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, shared.ErrInvalidInput
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

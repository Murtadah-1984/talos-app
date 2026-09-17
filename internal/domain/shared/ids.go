// Package shared holds value objects and small helpers used across every
// domain package. It has zero dependencies on infrastructure or integrations.
package shared

import (
	"time"

	"github.com/google/uuid"
)

// ID is a strongly-typed identifier shared by every aggregate root.
type ID = uuid.UUID

// NewID generates a new random identifier.
func NewID() ID {
	return uuid.New()
}

// ParseID parses a string into an ID.
func ParseID(s string) (ID, error) {
	return uuid.Parse(s)
}

// Timestamps is embedded by every persisted entity.
type Timestamps struct {
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Touch stamps CreatedAt (if zero) and always refreshes UpdatedAt.
func (t *Timestamps) Touch() {
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
}

// CapabilityState reports whether an integration is usable, per the platform-wide
// rule that unfinished integrations must say so explicitly rather than fake success.
type CapabilityState string

const (
	CapabilityNotConfigured CapabilityState = "NOT_CONFIGURED"
	CapabilityNotSupported  CapabilityState = "NOT_SUPPORTED"
	CapabilityAvailable     CapabilityState = "AVAILABLE"
	CapabilityDegraded      CapabilityState = "DEGRADED"
)

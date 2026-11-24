// Package store persists simulations and reproducible experiment series.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrNotFound is returned when a requested record does not exist.
	ErrNotFound = errors.New("store: record not found")
	// ErrConflict is returned when an attempt is made to create an existing record.
	ErrConflict = errors.New("store: record already exists")
)

// ValidationError describes a record that cannot be safely stored.
// The HTTP layer can map it to a 400 response without inspecting strings.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return "store: invalid value: " + e.Message
	}
	return fmt.Sprintf("store: invalid %s: %s", e.Field, e.Message)
}

// Config controls a SQLite-backed Store.
// Path accepts a SQLite filename or :memory: for tests. BusyTimeout defaults to
// five seconds. The store intentionally keeps one connection so per-connection
// SQLite pragmas (notably foreign_keys) apply to every operation.
type Config struct {
	Path        string
	BusyTimeout time.Duration
}

// ListOptions bounds history queries. A zero Limit selects the safe default.
type ListOptions struct {
	Limit  int
	Offset int
}

// SimulationRecord is a complete, reproducible calculation record. Complex
// values remain JSON because their schema belongs to the mathematical core and
// HTTP contract, while the store guarantees they are valid JSON.
type SimulationRecord struct {
	ID                  string          `json:"id"`
	ExperimentID        *string         `json:"experiment_id,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	Model               json.RawMessage `json:"model"`
	InitialDistribution json.RawMessage `json:"initial_distribution"`
	Algorithm           string          `json:"algorithm"`
	Seed                *int64          `json:"seed,omitempty"`
	CoreVersion         string          `json:"core_version"`
	Trace               json.RawMessage `json:"trace"`
	Result              json.RawMessage `json:"result"`
	Status              string          `json:"status"`
	EquilibriumStatus   string          `json:"equilibrium_status"`
	OptimumStatus       string          `json:"optimum_status"`
	PoAStatus           string          `json:"poa_status"`
	ErrorMessage        *string         `json:"error_message,omitempty"`
}

// ExperimentRecord represents a reproducible series of calculations. Results
// is normally a JSON array, but is kept generic so partially completed or
// cancelled experiments can still be faithfully saved and exported.
type ExperimentRecord struct {
	ID           string          `json:"id"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	Name         string          `json:"name"`
	Config       json.RawMessage `json:"config"`
	Results      json.RawMessage `json:"results"`
	Status       string          `json:"status"`
	CoreVersion  string          `json:"core_version"`
	Seed         *int64          `json:"seed,omitempty"`
	ErrorMessage *string         `json:"error_message,omitempty"`
}

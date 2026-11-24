package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	defaultListLimit = 100
	maxListLimit     = 1000
)

// Store is safe for concurrent callers. Operations share one SQLite connection
// so foreign-key enforcement and busy_timeout cannot be bypassed by a newly
// opened pooled connection.
type Store struct {
	db *sql.DB
}

// Open opens a SQLite database, configures its required pragmas, and applies
// every outstanding migration before returning the store.
func Open(ctx context.Context, config Config) (*Store, error) {
	config, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	if err := ensureParentDirectory(config.Path); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", config.Path)
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Store{db: db}
	if err := s.configure(ctx, config.BusyTimeout); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the SQLite connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Migrate idempotently applies versioned schema migrations.
func (s *Store) Migrate(ctx context.Context) error {
	if err := s.ensureOpen(); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}

	for _, m := range migrations {
		var applied int
		err := s.db.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE version = ?`, m.version).Scan(&applied)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: inspect migration %d: %w", m.version, err)
		}

		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("store: begin migration %d: %w", m.version, err)
		}
		if err := applyMigration(ctx, tx, m); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("store: commit migration %d: %w", m.version, err)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, tx *sql.Tx, m migration) error {
	for _, statement := range m.statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("store: apply migration %d (%s): %w", m.version, m.name, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.version, m.name, dbTime(time.Now()),
	); err != nil {
		return fmt.Errorf("store: record migration %d: %w", m.version, err)
	}
	return nil
}

// CreateSimulation inserts a completed or partial calculation record. Missing
// IDs and timestamps are assigned by the store.
func (s *Store) CreateSimulation(ctx context.Context, record SimulationRecord) (SimulationRecord, error) {
	if err := s.ensureOpen(); err != nil {
		return SimulationRecord{}, err
	}
	if record.ID == "" {
		id, err := newID()
		if err != nil {
			return SimulationRecord{}, err
		}
		record.ID = id
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	} else {
		record.CreatedAt = record.CreatedAt.UTC()
	}
	record.UpdatedAt = now
	if err := validateSimulation(&record); err != nil {
		return SimulationRecord{}, err
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO simulations (
		id, experiment_id, created_at, updated_at, model_json, initial_distribution_json,
		algorithm, seed, core_version, trace_json, result_json, status,
		equilibrium_status, optimum_status, poa_status, error_message
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID, nullableString(record.ExperimentID), dbTime(record.CreatedAt), dbTime(record.UpdatedAt),
		string(record.Model), string(record.InitialDistribution), record.Algorithm, nullableInt64(record.Seed),
		record.CoreVersion, string(record.Trace), string(record.Result), record.Status,
		record.EquilibriumStatus, record.OptimumStatus, record.PoAStatus, nullableString(record.ErrorMessage),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return SimulationRecord{}, ErrConflict
		}
		return SimulationRecord{}, fmt.Errorf("store: create simulation: %w", err)
	}
	return record, nil
}

// GetSimulation loads one calculation record by ID.
func (s *Store) GetSimulation(ctx context.Context, id string) (SimulationRecord, error) {
	if err := s.ensureOpen(); err != nil {
		return SimulationRecord{}, err
	}
	if strings.TrimSpace(id) == "" {
		return SimulationRecord{}, invalid("id", "must not be empty")
	}
	record, err := scanSimulation(s.db.QueryRowContext(ctx, simulationSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return SimulationRecord{}, ErrNotFound
	}
	if err != nil {
		return SimulationRecord{}, fmt.Errorf("store: get simulation: %w", err)
	}
	return record, nil
}

// ListSimulations returns newest records first, subject to bounded pagination.
func (s *Store) ListSimulations(ctx context.Context, options ListOptions) ([]SimulationRecord, error) {
	if err := s.ensureOpen(); err != nil {
		return nil, err
	}
	limit, offset, err := normalizeListOptions(options)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, simulationSelect+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("store: list simulations: %w", err)
	}
	defer rows.Close()

	records := make([]SimulationRecord, 0)
	for rows.Next() {
		record, err := scanSimulation(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan simulation list: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate simulation list: %w", err)
	}
	return records, nil
}

// UpdateSimulation replaces the mutable data of an existing simulation while
// retaining its original creation timestamp.
func (s *Store) UpdateSimulation(ctx context.Context, record SimulationRecord) (SimulationRecord, error) {
	if err := s.ensureOpen(); err != nil {
		return SimulationRecord{}, err
	}
	if strings.TrimSpace(record.ID) == "" {
		return SimulationRecord{}, invalid("id", "must not be empty")
	}
	record.UpdatedAt = time.Now().UTC()
	if err := validateSimulation(&record); err != nil {
		return SimulationRecord{}, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE simulations SET
		experiment_id = ?, updated_at = ?, model_json = ?, initial_distribution_json = ?,
		algorithm = ?, seed = ?, core_version = ?, trace_json = ?, result_json = ?, status = ?,
		equilibrium_status = ?, optimum_status = ?, poa_status = ?, error_message = ?
		WHERE id = ?`,
		nullableString(record.ExperimentID), dbTime(record.UpdatedAt), string(record.Model),
		string(record.InitialDistribution), record.Algorithm, nullableInt64(record.Seed), record.CoreVersion,
		string(record.Trace), string(record.Result), record.Status, record.EquilibriumStatus,
		record.OptimumStatus, record.PoAStatus, nullableString(record.ErrorMessage), record.ID,
	)
	if err != nil {
		return SimulationRecord{}, fmt.Errorf("store: update simulation: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return SimulationRecord{}, fmt.Errorf("store: inspect simulation update: %w", err)
	}
	if changed == 0 {
		return SimulationRecord{}, ErrNotFound
	}
	return s.GetSimulation(ctx, record.ID)
}

// CreateExperiment inserts a reproducible series record.
func (s *Store) CreateExperiment(ctx context.Context, record ExperimentRecord) (ExperimentRecord, error) {
	if err := s.ensureOpen(); err != nil {
		return ExperimentRecord{}, err
	}
	if record.ID == "" {
		id, err := newID()
		if err != nil {
			return ExperimentRecord{}, err
		}
		record.ID = id
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	} else {
		record.CreatedAt = record.CreatedAt.UTC()
	}
	record.UpdatedAt = now
	if err := validateExperiment(&record); err != nil {
		return ExperimentRecord{}, err
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO experiments (
		id, created_at, updated_at, name, config_json, results_json, status, core_version, seed, error_message
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID, dbTime(record.CreatedAt), dbTime(record.UpdatedAt), record.Name, string(record.Config),
		string(record.Results), record.Status, record.CoreVersion, nullableInt64(record.Seed), nullableString(record.ErrorMessage),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return ExperimentRecord{}, ErrConflict
		}
		return ExperimentRecord{}, fmt.Errorf("store: create experiment: %w", err)
	}
	return record, nil
}

// GetExperiment loads an experiment by ID.
func (s *Store) GetExperiment(ctx context.Context, id string) (ExperimentRecord, error) {
	if err := s.ensureOpen(); err != nil {
		return ExperimentRecord{}, err
	}
	if strings.TrimSpace(id) == "" {
		return ExperimentRecord{}, invalid("id", "must not be empty")
	}
	record, err := scanExperiment(s.db.QueryRowContext(ctx, experimentSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ExperimentRecord{}, ErrNotFound
	}
	if err != nil {
		return ExperimentRecord{}, fmt.Errorf("store: get experiment: %w", err)
	}
	return record, nil
}

// ListExperiments returns experiment history ordered newest first.
func (s *Store) ListExperiments(ctx context.Context, options ListOptions) ([]ExperimentRecord, error) {
	if err := s.ensureOpen(); err != nil {
		return nil, err
	}
	limit, offset, err := normalizeListOptions(options)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, experimentSelect+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("store: list experiments: %w", err)
	}
	defer rows.Close()

	records := make([]ExperimentRecord, 0)
	for rows.Next() {
		record, err := scanExperiment(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan experiment list: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate experiment list: %w", err)
	}
	return records, nil
}

// UpdateExperiment replaces the mutable data of an existing experiment.
func (s *Store) UpdateExperiment(ctx context.Context, record ExperimentRecord) (ExperimentRecord, error) {
	if err := s.ensureOpen(); err != nil {
		return ExperimentRecord{}, err
	}
	if strings.TrimSpace(record.ID) == "" {
		return ExperimentRecord{}, invalid("id", "must not be empty")
	}
	record.UpdatedAt = time.Now().UTC()
	if err := validateExperiment(&record); err != nil {
		return ExperimentRecord{}, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE experiments SET
		updated_at = ?, name = ?, config_json = ?, results_json = ?, status = ?, core_version = ?, seed = ?, error_message = ?
		WHERE id = ?`,
		dbTime(record.UpdatedAt), record.Name, string(record.Config), string(record.Results), record.Status,
		record.CoreVersion, nullableInt64(record.Seed), nullableString(record.ErrorMessage), record.ID,
	)
	if err != nil {
		return ExperimentRecord{}, fmt.Errorf("store: update experiment: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return ExperimentRecord{}, fmt.Errorf("store: inspect experiment update: %w", err)
	}
	if changed == 0 {
		return ExperimentRecord{}, ErrNotFound
	}
	return s.GetExperiment(ctx, record.ID)
}

// DeleteExperiment removes an experiment. Attached simulations are retained and
// their ExperimentID is set to NULL by the foreign-key rule.
func (s *Store) DeleteExperiment(ctx context.Context, id string) error {
	if err := s.ensureOpen(); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return invalid("id", "must not be empty")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM experiments WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete experiment: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: inspect experiment deletion: %w", err)
	}
	if changed == 0 {
		return ErrNotFound
	}
	return nil
}

const simulationSelect = `SELECT id, experiment_id, created_at, updated_at, model_json, initial_distribution_json,
	algorithm, seed, core_version, trace_json, result_json, status,
	equilibrium_status, optimum_status, poa_status, error_message FROM simulations`

const experimentSelect = `SELECT id, created_at, updated_at, name, config_json, results_json,
	status, core_version, seed, error_message FROM experiments`

type scanner interface {
	Scan(dest ...any) error
}

func scanSimulation(row scanner) (SimulationRecord, error) {
	var record SimulationRecord
	var experimentID, errorMessage sql.NullString
	var seed sql.NullInt64
	var createdAt, updatedAt string
	var model, initialDistribution, trace, result []byte
	err := row.Scan(
		&record.ID, &experimentID, &createdAt, &updatedAt, &model, &initialDistribution,
		&record.Algorithm, &seed, &record.CoreVersion, &trace, &result, &record.Status,
		&record.EquilibriumStatus, &record.OptimumStatus, &record.PoAStatus, &errorMessage,
	)
	if err != nil {
		return SimulationRecord{}, err
	}
	var parseErr error
	record.CreatedAt, parseErr = parseDBTime(createdAt)
	if parseErr != nil {
		return SimulationRecord{}, parseErr
	}
	record.UpdatedAt, parseErr = parseDBTime(updatedAt)
	if parseErr != nil {
		return SimulationRecord{}, parseErr
	}
	record.ExperimentID = fromNullString(experimentID)
	record.Seed = fromNullInt64(seed)
	record.ErrorMessage = fromNullString(errorMessage)
	record.Model = cloneJSON(model)
	record.InitialDistribution = cloneJSON(initialDistribution)
	record.Trace = cloneJSON(trace)
	record.Result = cloneJSON(result)
	return record, nil
}

func scanExperiment(row scanner) (ExperimentRecord, error) {
	var record ExperimentRecord
	var seed sql.NullInt64
	var errorMessage sql.NullString
	var createdAt, updatedAt string
	var config, results []byte
	err := row.Scan(
		&record.ID, &createdAt, &updatedAt, &record.Name, &config, &results,
		&record.Status, &record.CoreVersion, &seed, &errorMessage,
	)
	if err != nil {
		return ExperimentRecord{}, err
	}
	var parseErr error
	record.CreatedAt, parseErr = parseDBTime(createdAt)
	if parseErr != nil {
		return ExperimentRecord{}, parseErr
	}
	record.UpdatedAt, parseErr = parseDBTime(updatedAt)
	if parseErr != nil {
		return ExperimentRecord{}, parseErr
	}
	record.Seed = fromNullInt64(seed)
	record.ErrorMessage = fromNullString(errorMessage)
	record.Config = cloneJSON(config)
	record.Results = cloneJSON(results)
	return record, nil
}

func validateSimulation(record *SimulationRecord) error {
	if strings.TrimSpace(record.ID) == "" {
		return invalid("id", "must not be empty")
	}
	if record.ExperimentID != nil && strings.TrimSpace(*record.ExperimentID) == "" {
		return invalid("experiment_id", "must not be empty when specified")
	}
	if err := normalizeJSON(&record.Model, "model"); err != nil {
		return err
	}
	if err := normalizeJSON(&record.InitialDistribution, "initial_distribution"); err != nil {
		return err
	}
	if strings.TrimSpace(record.Algorithm) == "" {
		return invalid("algorithm", "must not be empty")
	}
	if strings.TrimSpace(record.CoreVersion) == "" {
		return invalid("core_version", "must not be empty")
	}
	if err := normalizeJSON(&record.Trace, "trace"); err != nil {
		return err
	}
	if err := normalizeJSON(&record.Result, "result"); err != nil {
		return err
	}
	if strings.TrimSpace(record.Status) == "" {
		return invalid("status", "must not be empty")
	}
	if strings.TrimSpace(record.EquilibriumStatus) == "" {
		return invalid("equilibrium_status", "must not be empty")
	}
	if strings.TrimSpace(record.OptimumStatus) == "" {
		return invalid("optimum_status", "must not be empty")
	}
	if strings.TrimSpace(record.PoAStatus) == "" {
		return invalid("poa_status", "must not be empty")
	}
	return nil
}

func validateExperiment(record *ExperimentRecord) error {
	if strings.TrimSpace(record.ID) == "" {
		return invalid("id", "must not be empty")
	}
	if err := normalizeJSON(&record.Config, "config"); err != nil {
		return err
	}
	if err := normalizeJSON(&record.Results, "results"); err != nil {
		return err
	}
	if strings.TrimSpace(record.Status) == "" {
		return invalid("status", "must not be empty")
	}
	if strings.TrimSpace(record.CoreVersion) == "" {
		return invalid("core_version", "must not be empty")
	}
	return nil
}

func normalizeConfig(config Config) (Config, error) {
	config.Path = strings.TrimSpace(config.Path)
	if config.Path == "" {
		return Config{}, invalid("path", "must not be empty")
	}
	if config.BusyTimeout < 0 {
		return Config{}, invalid("busy_timeout", "must not be negative")
	}
	if config.BusyTimeout == 0 {
		config.BusyTimeout = 5 * time.Second
	}
	return config, nil
}

func ensureParentDirectory(path string) error {
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("store: create database directory: %w", err)
	}
	return nil
}

func (s *Store) configure(ctx context.Context, busyTimeout time.Duration) error {
	if err := s.ensureOpen(); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA journal_mode = WAL`); err != nil {
		return fmt.Errorf("store: enable WAL: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("store: enable foreign keys: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`PRAGMA busy_timeout = %d`, busyTimeout.Milliseconds())); err != nil {
		return fmt.Errorf("store: set busy timeout: %w", err)
	}
	return nil
}

func (s *Store) ensureOpen() error {
	if s == nil || s.db == nil {
		return errors.New("store: nil store")
	}
	return nil
}

func normalizeListOptions(options ListOptions) (int, int, error) {
	if options.Limit < 0 || options.Limit > maxListLimit {
		return 0, 0, invalid("limit", fmt.Sprintf("must be between 0 and %d", maxListLimit))
	}
	if options.Offset < 0 {
		return 0, 0, invalid("offset", "must not be negative")
	}
	if options.Limit == 0 {
		options.Limit = defaultListLimit
	}
	return options.Limit, options.Offset, nil
}

func normalizeJSON(value *json.RawMessage, field string) error {
	trimmed := bytes.TrimSpace(*value)
	if len(trimmed) == 0 {
		return invalid(field, "must contain JSON")
	}
	if !json.Valid(trimmed) {
		return invalid(field, "must be valid JSON")
	}
	normalized := make([]byte, len(trimmed))
	copy(normalized, trimmed)
	*value = normalized
	return nil
}

func cloneJSON(value []byte) json.RawMessage {
	copyValue := make([]byte, len(value))
	copy(copyValue, value)
	return json.RawMessage(copyValue)
}

func invalid(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}

func newID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("store: generate ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func dbTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseDBTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("store: parse database timestamp: %w", err)
	}
	return parsed.UTC(), nil
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func fromNullString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func fromNullInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func isUniqueConstraint(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

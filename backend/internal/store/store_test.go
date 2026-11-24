package store

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMigrationsAndSimulationRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	var migrationCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != len(migrations) {
		t.Fatalf("migrations = %d, want %d", migrationCount, len(migrations))
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	var foreignKeys int
	if err := store.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys pragma: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
	var busyTimeout int
	if err := store.db.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("read busy_timeout pragma: %v", err)
	}
	if busyTimeout != 1000 {
		t.Fatalf("busy_timeout = %d, want 1000", busyTimeout)
	}

	experiment, err := store.CreateExperiment(ctx, sampleExperiment("experiment-round-trip"))
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}
	simulation, err := store.CreateSimulation(ctx, sampleSimulation("simulation-round-trip", &experiment.ID))
	if err != nil {
		t.Fatalf("create simulation: %v", err)
	}
	if simulation.CreatedAt.IsZero() || simulation.UpdatedAt.IsZero() {
		t.Fatal("store did not assign timestamps")
	}

	loaded, err := store.GetSimulation(ctx, simulation.ID)
	if err != nil {
		t.Fatalf("get simulation: %v", err)
	}
	if loaded.ExperimentID == nil || *loaded.ExperimentID != experiment.ID {
		t.Fatalf("experiment id = %v, want %q", loaded.ExperimentID, experiment.ID)
	}
	if string(loaded.Result) != `{"equilibrium":true}` {
		t.Fatalf("result = %s", loaded.Result)
	}

	loaded.Status = "cancelled"
	loaded.Result = json.RawMessage(`{"reason":"manual cancellation"}`)
	updated, err := store.UpdateSimulation(ctx, loaded)
	if err != nil {
		t.Fatalf("update simulation: %v", err)
	}
	if updated.Status != "cancelled" || updated.CreatedAt != simulation.CreatedAt {
		t.Fatalf("unexpected updated record: %#v", updated)
	}

	records, err := store.ListSimulations(ctx, ListOptions{})
	if err != nil {
		t.Fatalf("list simulations: %v", err)
	}
	if len(records) != 1 || records[0].ID != simulation.ID {
		t.Fatalf("listed records = %#v", records)
	}
	if _, err := store.GetSimulation(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing error = %v, want ErrNotFound", err)
	}
}

func TestDeleteExperimentRetainsSimulation(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	experiment, err := store.CreateExperiment(ctx, sampleExperiment("experiment-delete"))
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}
	simulation, err := store.CreateSimulation(ctx, sampleSimulation("simulation-delete", &experiment.ID))
	if err != nil {
		t.Fatalf("create simulation: %v", err)
	}

	if err := store.DeleteExperiment(ctx, experiment.ID); err != nil {
		t.Fatalf("delete experiment: %v", err)
	}
	loaded, err := store.GetSimulation(ctx, simulation.ID)
	if err != nil {
		t.Fatalf("get retained simulation: %v", err)
	}
	if loaded.ExperimentID != nil {
		t.Fatalf("experiment id after delete = %q, want nil", *loaded.ExperimentID)
	}
	if err := store.DeleteExperiment(ctx, experiment.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete error = %v, want ErrNotFound", err)
	}
}

func TestRecordsPersistAfterReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "persist", "nash-load-lab.db")
	first, err := Open(ctx, Config{Path: path})
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	experiment, err := first.CreateExperiment(ctx, sampleExperiment("experiment-persist"))
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}
	if _, err := first.CreateSimulation(ctx, sampleSimulation("simulation-persist", &experiment.ID)); err != nil {
		t.Fatalf("create simulation: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	second, err := Open(ctx, Config{Path: path})
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	loaded, err := second.GetExperiment(ctx, experiment.ID)
	if err != nil {
		t.Fatalf("get persisted experiment: %v", err)
	}
	if loaded.Name != experiment.Name || string(loaded.Config) != string(experiment.Config) {
		t.Fatalf("persisted experiment = %#v, want %#v", loaded, experiment)
	}
	updated := loaded
	updated.Status = "cancelled"
	updated.Results = json.RawMessage(`[{"run":1,"cancelled":true}]`)
	updated, err = second.UpdateExperiment(ctx, updated)
	if err != nil {
		t.Fatalf("update persisted experiment: %v", err)
	}
	if updated.Status != "cancelled" || updated.CreatedAt != experiment.CreatedAt {
		t.Fatalf("unexpected updated experiment: %#v", updated)
	}
}

func TestExperimentExportsContainFullRecords(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	experiment, err := store.CreateExperiment(ctx, sampleExperiment("experiment-export"))
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}
	if _, err := store.CreateSimulation(ctx, sampleSimulation("simulation-export", &experiment.ID)); err != nil {
		t.Fatalf("create simulation: %v", err)
	}

	jsonExport, err := store.ExportExperimentJSON(ctx, experiment.ID)
	if err != nil {
		t.Fatalf("JSON export: %v", err)
	}
	var document ExperimentExport
	if err := json.Unmarshal(jsonExport, &document); err != nil {
		t.Fatalf("decode JSON export: %v", err)
	}
	if document.FormatVersion != 1 || document.Experiment.ID != experiment.ID || len(document.Simulations) != 1 {
		t.Fatalf("unexpected export: %#v", document)
	}

	csvExport, err := store.ExportExperimentCSV(ctx, experiment.ID)
	if err != nil {
		t.Fatalf("CSV export: %v", err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(csvExport))).ReadAll()
	if err != nil {
		t.Fatalf("decode CSV export: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("CSV rows = %d, want 3", len(rows))
	}
	for index, row := range rows {
		if len(row) != len(csvHeader) {
			t.Fatalf("CSV row %d length = %d, want %d", index, len(row), len(csvHeader))
		}
	}
	if rows[1][0] != "experiment" || rows[2][0] != "simulation" {
		t.Fatalf("CSV record types = %q, %q", rows[1][0], rows[2][0])
	}
	if _, err := store.ExportExperiment(ctx, experiment.ID, "yaml"); err == nil {
		t.Fatal("unsupported export format was accepted")
	}
}

func TestValidationRejectsInvalidJSON(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	record := sampleExperiment("invalid-json")
	record.Config = json.RawMessage(`{"broken"`)
	if _, err := store.CreateExperiment(ctx, record); err == nil {
		t.Fatal("invalid JSON was accepted")
	} else {
		var validationError *ValidationError
		if !errors.As(err, &validationError) {
			t.Fatalf("error = %T %v, want ValidationError", err, err)
		}
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), Config{
		Path:        filepath.Join(t.TempDir(), "data", "nash-load-lab.db"),
		BusyTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return store
}

func sampleExperiment(id string) ExperimentRecord {
	seed := int64(42)
	return ExperimentRecord{
		ID:          id,
		Name:        "Эталонный опыт",
		Config:      json.RawMessage(`{"players":2,"nodes":2}`),
		Results:     json.RawMessage(`[{"run":1,"steps":1}]`),
		Status:      "completed",
		CoreVersion: "test-core",
		Seed:        &seed,
	}
}

func sampleSimulation(id string, experimentID *string) SimulationRecord {
	seed := int64(7)
	return SimulationRecord{
		ID:                  id,
		ExperimentID:        experimentID,
		Model:               json.RawMessage(`{"nodes":[{"id":"A"},{"id":"B"}]}`),
		InitialDistribution: json.RawMessage(`["B","B"]`),
		Algorithm:           "first_improvement",
		Seed:                &seed,
		CoreVersion:         "test-core",
		Trace:               json.RawMessage(`[{"step":0},{"step":1}]`),
		Result:              json.RawMessage(`{"equilibrium":true}`),
		Status:              "completed",
		EquilibriumStatus:   "equilibrium",
		OptimumStatus:       "exact",
		PoAStatus:           "exact",
	}
}

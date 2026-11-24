package store

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ExperimentExport is the self-contained JSON representation of one research
// series. It deliberately includes linked simulations so a downloaded file can
// reproduce both the parameters and every recorded calculation.
type ExperimentExport struct {
	FormatVersion int                `json:"format_version"`
	ExportedAt    time.Time          `json:"exported_at"`
	Experiment    ExperimentRecord   `json:"experiment"`
	Simulations   []SimulationRecord `json:"simulations"`
}

// ExportExperimentJSON returns a complete, reproducible experiment export.
func (s *Store) ExportExperimentJSON(ctx context.Context, id string) ([]byte, error) {
	export, err := s.experimentExport(ctx, id)
	if err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("store: encode experiment JSON export: %w", err)
	}
	return append(encoded, '\n'), nil
}

// ExportExperimentCSV returns the same data as a CSV file. The first row is
// the experiment itself; subsequent rows are linked simulation records. JSON
// columns are intentionally preserved verbatim to avoid losing precision.
func (s *Store) ExportExperimentCSV(ctx context.Context, id string) ([]byte, error) {
	export, err := s.experimentExport(ctx, id)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write(csvHeader); err != nil {
		return nil, fmt.Errorf("store: write CSV header: %w", err)
	}
	if err := writer.Write(experimentCSVRow(export.Experiment)); err != nil {
		return nil, fmt.Errorf("store: write CSV experiment: %w", err)
	}
	for _, simulation := range export.Simulations {
		if err := writer.Write(simulationCSVRow(export.Experiment, simulation)); err != nil {
			return nil, fmt.Errorf("store: write CSV simulation: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("store: finalize CSV export: %w", err)
	}
	return buffer.Bytes(), nil
}

// ExportExperiment selects either the json or csv representation.
func (s *Store) ExportExperiment(ctx context.Context, id, format string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return s.ExportExperimentJSON(ctx, id)
	case "csv":
		return s.ExportExperimentCSV(ctx, id)
	default:
		return nil, invalid("format", "must be csv or json")
	}
}

func (s *Store) experimentExport(ctx context.Context, id string) (ExperimentExport, error) {
	experiment, err := s.GetExperiment(ctx, id)
	if err != nil {
		return ExperimentExport{}, err
	}
	simulations, err := s.listSimulationsForExperiment(ctx, id)
	if err != nil {
		return ExperimentExport{}, err
	}
	return ExperimentExport{
		FormatVersion: 1,
		ExportedAt:    time.Now().UTC(),
		Experiment:    experiment,
		Simulations:   simulations,
	}, nil
}

func (s *Store) listSimulationsForExperiment(ctx context.Context, experimentID string) ([]SimulationRecord, error) {
	if err := s.ensureOpen(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, simulationSelect+` WHERE experiment_id = ? ORDER BY created_at ASC, id ASC`, experimentID)
	if err != nil {
		return nil, fmt.Errorf("store: list experiment simulations: %w", err)
	}
	defer rows.Close()

	records := make([]SimulationRecord, 0)
	for rows.Next() {
		record, err := scanSimulation(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan experiment simulation: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate experiment simulations: %w", err)
	}
	return records, nil
}

var csvHeader = []string{
	"record_type",
	"experiment_id",
	"experiment_name",
	"experiment_created_at",
	"experiment_updated_at",
	"experiment_status",
	"experiment_core_version",
	"experiment_seed",
	"experiment_config_json",
	"experiment_results_json",
	"experiment_error_message",
	"simulation_id",
	"simulation_created_at",
	"simulation_updated_at",
	"simulation_status",
	"simulation_equilibrium_status",
	"simulation_optimum_status",
	"simulation_poa_status",
	"simulation_algorithm",
	"simulation_seed",
	"simulation_core_version",
	"simulation_model_json",
	"simulation_initial_distribution_json",
	"simulation_trace_json",
	"simulation_result_json",
	"simulation_error_message",
}

func experimentCSVRow(experiment ExperimentRecord) []string {
	row := experimentColumns(experiment)
	row[0] = "experiment"
	return append(row, make([]string, len(csvHeader)-len(row))...)
}

func simulationCSVRow(experiment ExperimentRecord, simulation SimulationRecord) []string {
	row := experimentColumns(experiment)
	row[0] = "simulation"
	return append(row,
		simulation.ID,
		dbTime(simulation.CreatedAt),
		dbTime(simulation.UpdatedAt),
		simulation.Status,
		simulation.EquilibriumStatus,
		simulation.OptimumStatus,
		simulation.PoAStatus,
		simulation.Algorithm,
		int64String(simulation.Seed),
		simulation.CoreVersion,
		string(simulation.Model),
		string(simulation.InitialDistribution),
		string(simulation.Trace),
		string(simulation.Result),
		stringValue(simulation.ErrorMessage),
	)
}

func experimentColumns(experiment ExperimentRecord) []string {
	return []string{
		"",
		experiment.ID,
		experiment.Name,
		dbTime(experiment.CreatedAt),
		dbTime(experiment.UpdatedAt),
		experiment.Status,
		experiment.CoreVersion,
		int64String(experiment.Seed),
		string(experiment.Config),
		string(experiment.Results),
		stringValue(experiment.ErrorMessage),
	}
}

func int64String(value *int64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%d", *value)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nash-load-lab/backend/internal/game"
	"nash-load-lab/backend/internal/store"
)

const (
	apiPrefix           = "/api/v1"
	previewTimeout      = 3 * time.Second
	simulationTimeout   = 15 * time.Second
	optimizationTimeout = 8 * time.Second
	experimentTimeout   = 30 * time.Second
)

// Server owns the HTTP boundary and delegates all mathematics to game and all
// persistence to store. It contains no duplicated F/S/NE calculations.
type Server struct {
	store           *store.Store
	version         string
	presentation    presentationResponse
	presentationErr error
}

// New creates an API handler backed by a ready, migrated SQLite store.
//
// The defence deck is deliberately prepared here, before the HTTP server
// accepts requests.  handlePresentation only serializes this immutable
// snapshot, so opening or advancing slides never starts simulation or exact
// optimization work.
func New(storage *store.Store) *Server {
	presentation, presentationErr := preparedPresentation()
	return &Server{
		store:           storage,
		version:         game.Version,
		presentation:    presentation,
		presentationErr: presentationErr,
	}
}

// ServeHTTP provides a compact dependency-free router for the documented API.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "" {
		path = "/"
	}
	switch {
	case path == apiPrefix+"/health":
		s.handleHealth(w, r)
	case path == apiPrefix+"/presets":
		s.handlePresets(w, r)
	case path == apiPrefix+"/presentation":
		s.handlePresentation(w, r)
	case path == apiPrefix+"/preview":
		s.handlePreview(w, r)
	case path == apiPrefix+"/simulations":
		s.handleSimulations(w, r)
	case strings.HasPrefix(path, apiPrefix+"/simulations/"):
		s.handleSimulationByID(w, r, strings.TrimPrefix(path, apiPrefix+"/simulations/"))
	case path == apiPrefix+"/optimize":
		s.handleOptimize(w, r)
	case path == apiPrefix+"/experiments":
		s.handleExperiments(w, r)
	case strings.HasPrefix(path, apiPrefix+"/experiments/"):
		s.handleExperimentByPath(w, r, strings.TrimPrefix(path, apiPrefix+"/experiments/"))
	default:
		writeError(w, http.StatusNotFound, "not_found", "Маршрут API не найден.", nil)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: s.version})
}

func (s *Server) handlePresets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, presetsResponse{Presets: standardPresets()})
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var request PreviewRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	model, err := request.toGame()
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Некорректные параметры модели.", map[string]any{"reason": err.Error()})
		return
	}
	calculationContext, cancel := context.WithTimeout(r.Context(), previewTimeout)
	defer cancel()
	rows := make([]previewRow, 0, request.Players)
	for load := 1; load <= request.Players; load++ {
		if err := calculationContext.Err(); err != nil {
			writeError(w, http.StatusRequestTimeout, "cancelled", "Расчёт предварительного просмотра отменён.", nil)
			return
		}
		values := make([]previewValue, len(model.Nodes))
		for index, node := range model.Nodes {
			value, err := model.Latency(index, load)
			if err != nil {
				writeInternalError(w, err)
				return
			}
			values[index] = previewValue{NodeID: node.ID, Value: value}
		}
		rows = append(rows, previewRow{Load: load, Values: values})
	}
	writeJSON(w, http.StatusOK, previewResponse{Rows: rows})
}

func (s *Server) handleSimulations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var request SimulationRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	model, err := request.validate()
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Некорректные параметры симуляции.", map[string]any{"reason": err.Error()})
		return
	}
	calculationContext, cancel := context.WithTimeout(r.Context(), simulationTimeout)
	defer cancel()
	result, err := game.Simulate(calculationContext, model, request.InitialAssignment, game.SimulationOptions{Policy: request.Algorithm, MaxSteps: request.MaxSteps})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			writeError(w, http.StatusRequestTimeout, "cancelled", "Симуляция отменена.", nil)
			return
		}
		writeInternalError(w, err)
		return
	}
	response := simulationResponseFromResult(request, result)
	record, err := s.persistSimulation(r.Context(), response, nil)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	response.ID = record.ID
	response.CreatedAt = record.CreatedAt
	writeJSON(w, http.StatusCreated, response)
}

func (s *Server) handleSimulationByID(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "not_found", "Симуляция не найдена.", nil)
		return
	}
	record, err := s.store.GetSimulation(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	response, err := responseFromSimulationRecord(record)
	if err != nil {
		writeInternalError(w, fmt.Errorf("восстановить симуляцию %s: %w", id, err))
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleOptimize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var request OptimizeRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	model, err := request.model()
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Некорректные параметры модели.", map[string]any{"reason": err.Error()})
		return
	}
	maxStates := request.MaxStates
	if maxStates == 0 {
		maxStates = defaultExactStates
	}
	if maxStates > maxExactStates {
		writeError(w, http.StatusBadRequest, "validation_error", "Лимит точного перебора слишком велик.", map[string]any{"max_states": maxExactStates})
		return
	}
	policy := request.Algorithm
	steps := request.MaxSteps
	if len(request.InitialAssignment) > 0 {
		if err := model.ValidateAssignment(request.InitialAssignment); err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", "Некорректное распределение для сравнения.", map[string]any{"reason": err.Error()})
			return
		}
		if policy == "" {
			policy = game.FirstImprovement
		}
		if policy != game.FirstImprovement && policy != game.BestResponse {
			writeError(w, http.StatusBadRequest, "validation_error", "Некорректная политика динамики для сравнения.", nil)
			return
		}
		if steps == 0 {
			steps = maxSimulationSteps
		}
		if steps < 1 || steps > maxSimulationSteps {
			writeError(w, http.StatusBadRequest, "validation_error", "Некорректный лимит шагов для сравнения.", nil)
			return
		}
	}
	calculationContext, cancel := context.WithTimeout(r.Context(), optimizationTimeout)
	defer cancel()
	optimized, err := game.Optimize(calculationContext, model, game.OptimizeOptions{MaxStates: maxStates})
	if err != nil {
		writeInternalError(w, err)
		return
	}
	response := responseFromOptimization(optimized)
	if len(request.InitialAssignment) > 0 {
		simulation, err := game.Simulate(calculationContext, model, request.InitialAssignment, game.SimulationOptions{Policy: policy, MaxSteps: steps})
		if err != nil {
			writeInternalError(w, err)
			return
		}
		if simulation.Status == game.SimulationEquilibrium && simulation.NashCheck.IsEquilibrium {
			response.Equilibrium = responseAllocation(&simulation.Final)
			if optimized.Optimum.Status == game.CalculationExact && optimized.Optimum.Summary != nil && !optimized.Optimum.Summary.TotalTime.IsZero() {
				value, err := simulation.Final.TotalTime.Div(optimized.Optimum.Summary.TotalTime)
				if err != nil {
					writeInternalError(w, err)
					return
				}
				response.KRun = &value
			}
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func simulationResponseFromResult(request SimulationRequest, result game.SimulationResult) SimulationResponse {
	return SimulationResponse{
		Request: request, Initial: result.Initial, Trace: result.Trace, Final: result.Final,
		NashCheck: result.NashCheck, Status: result.Status, Message: result.Message,
	}
}

func responseFromOptimization(result game.OptimizationResult) optimizationResponse {
	poa := poaResponse{
		Status: result.PoA.Status, Message: result.PoA.Message, Value: result.PoAValue,
		WorstEquilibrium: responseAllocation(result.WorstEquilibrium),
	}
	return optimizationResponse{Optimum: responseOptimizationValue(result.Optimum), PoA: poa}
}

func (s *Server) persistSimulation(_ context.Context, response SimulationResponse, experimentID *string) (store.SimulationRecord, error) {
	model, err := json.Marshal(response.Request)
	if err != nil {
		return store.SimulationRecord{}, err
	}
	initial, err := json.Marshal(response.Initial)
	if err != nil {
		return store.SimulationRecord{}, err
	}
	trace, err := json.Marshal(response.Trace)
	if err != nil {
		return store.SimulationRecord{}, err
	}
	result, err := json.Marshal(response)
	if err != nil {
		return store.SimulationRecord{}, err
	}
	equilibriumStatus := string(game.CalculationNotComputed)
	if response.Status == game.SimulationEquilibrium && response.NashCheck.IsEquilibrium {
		equilibriumStatus = string(game.CalculationExact)
	}
	if response.Status == game.SimulationCancelled {
		equilibriumStatus = string(game.CalculationCancelled)
	}
	// Persisting a completed/cancelled calculation must not be aborted merely
	// because its HTTP client disconnected; otherwise a reproducibility record
	// would disappear precisely for the runs whose terminal status matters.
	storageContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.store.CreateSimulation(storageContext, store.SimulationRecord{
		ExperimentID: experimentID, Model: model, InitialDistribution: initial, Algorithm: string(response.Request.Algorithm),
		Seed: response.Request.Seed, CoreVersion: s.version, Trace: trace, Result: result,
		Status: string(response.Status), EquilibriumStatus: equilibriumStatus,
		OptimumStatus: string(game.CalculationNotComputed), PoAStatus: string(game.CalculationNotComputed),
	})
}

func responseFromSimulationRecord(record store.SimulationRecord) (SimulationResponse, error) {
	var response SimulationResponse
	if err := json.Unmarshal(record.Result, &response); err != nil {
		return SimulationResponse{}, err
	}
	// Records created by earlier compatible versions may not have stored the
	// request inside Result, so Model remains an authoritative fallback.
	if response.Request.Players == 0 {
		if err := json.Unmarshal(record.Model, &response.Request); err != nil {
			return SimulationResponse{}, err
		}
	}
	response.ID = record.ID
	response.CreatedAt = record.CreatedAt
	return response, nil
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Запись не найдена.", nil)
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "Запись с таким идентификатором уже существует.", nil)
	default:
		var validation *store.ValidationError
		if errors.As(err, &validation) {
			writeError(w, http.StatusBadRequest, "validation_error", "Некорректные данные для хранения.", map[string]any{"field": validation.Field, "reason": validation.Message})
			return
		}
		writeInternalError(w, err)
	}
}

func writeInternalError(w http.ResponseWriter, _ error) {
	// Detailed errors can contain filesystem or SQLite implementation details.
	// The API stays actionable without exposing those internals.
	writeError(w, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера.", nil)
}

func standardPresets() []presetResponse {
	return []presetResponse{
		{
			ID: "A", Name: "A. Два игрока", Description: "A(k)=k, B(k)=2k, старт (B,B). Демонстрирует K=1 при PoA=4/3.",
			Request: SimulationRequest{Players: 2, Nodes: []NodeInput{
				{ID: "A", Name: "Узел A", A: game.MustDecimal("0"), B: game.MustDecimal("1"), Kind: game.CostLinear},
				{ID: "B", Name: "Узел B", A: game.MustDecimal("0"), B: game.MustDecimal("2"), Kind: game.CostLinear},
			}, InitialAssignment: []string{"B", "B"}, Algorithm: game.FirstImprovement, MaxSteps: 100},
		},
		{
			ID: "B", Name: "B. Четыре игрока", Description: "A(k)=k, B(k)=1+0,5k, старт (A,A,A,A). Два строгих улучшения: F 10 → 7,5 → 6,5.",
			Request: SimulationRequest{Players: 4, Nodes: []NodeInput{
				{ID: "A", Name: "Узел A", A: game.MustDecimal("0"), B: game.MustDecimal("1"), Kind: game.CostLinear},
				{ID: "B", Name: "Узел B", A: game.MustDecimal("1"), B: game.MustDecimal("0.5"), Kind: game.CostLinear},
			}, InitialAssignment: []string{"A", "A", "A", "A"}, Algorithm: game.FirstImprovement, MaxSteps: 100},
		},
		{
			ID: "C", Name: "C. Неоптимальное равновесие", Description: "A(k)=k, B(k)=2k, старт (A,A). Распределение устойчиво, но S=4 при оптимуме S=3.",
			Request: SimulationRequest{Players: 2, Nodes: []NodeInput{
				{ID: "A", Name: "Узел A", A: game.MustDecimal("0"), B: game.MustDecimal("1"), Kind: game.CostLinear},
				{ID: "B", Name: "Узел B", A: game.MustDecimal("0"), B: game.MustDecimal("2"), Kind: game.CostLinear},
			}, InitialAssignment: []string{"A", "A"}, Algorithm: game.FirstImprovement, MaxSteps: 100},
		},
	}
}

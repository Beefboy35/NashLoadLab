package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nash-load-lab/backend/internal/game"
	"nash-load-lab/backend/internal/store"
)

const experimentExactStateLimit uint64 = 50_000

func (s *Server) handleExperiments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createExperiment(w, r)
	case http.MethodGet:
		s.listExperiments(w, r)
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

func (s *Server) handleExperimentByPath(w http.ResponseWriter, r *http.Request, remainder string) {
	parts := strings.Split(remainder, "/")
	if len(parts) == 2 && parts[1] == "export" && parts[0] != "" {
		s.exportExperiment(w, r, parts[0])
		return
	}
	if len(parts) != 1 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not_found", "Эксперимент не найден.", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.getExperiment(w, r, parts[0])
	case http.MethodDelete:
		s.deleteExperiment(w, r, parts[0])
	default:
		methodNotAllowed(w, "GET, DELETE")
	}
}

func (s *Server) createExperiment(w http.ResponseWriter, r *http.Request) {
	var request ExperimentRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if err := validateExperimentRequest(request); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Некорректные параметры исследования.", map[string]any{"reason": err.Error()})
		return
	}
	config, err := json.Marshal(request)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	emptyResults := json.RawMessage("[]")
	record, err := s.store.CreateExperiment(r.Context(), store.ExperimentRecord{
		Name: "Воспроизводимое исследование нагрузки", Config: config, Results: emptyResults,
		Status: "running", CoreVersion: s.version, Seed: &request.Seed,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}

	calculationContext, cancel := context.WithTimeout(r.Context(), experimentTimeout)
	defer cancel()
	points, runErr := s.runExperiment(calculationContext, request, record.ID)
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) || r.Context().Err() != nil {
			message := "Исследование отменено до завершения."
			s.persistExperimentTerminal(record, "cancelled", points, &message)
			// A disconnected client has no useful response channel. If it is still
			// connected, make the state explicit instead of reporting success.
			if r.Context().Err() == nil {
				writeError(w, http.StatusRequestTimeout, "cancelled", message, nil)
			}
			return
		}
		message := "Исследование завершилось внутренней ошибкой."
		s.persistExperimentTerminal(record, "error", points, &message)
		writeInternalError(w, runErr)
		return
	}
	updated, err := s.persistExperimentTerminal(record, "complete", points, nil)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, experimentResponseFromRecord(updated, true))
}

func (s *Server) listExperiments(w http.ResponseWriter, r *http.Request) {
	records, err := s.store.ListExperiments(r.Context(), store.ListOptions{Limit: 100})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	response := experimentListResponse{Experiments: make([]experimentSummaryResponse, 0, len(records))}
	for _, record := range records {
		response.Experiments = append(response.Experiments, experimentResponseFromRecord(record, false))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) getExperiment(w http.ResponseWriter, r *http.Request, id string) {
	record, err := s.store.GetExperiment(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, experimentResponseFromRecord(record, true))
}

func (s *Server) deleteExperiment(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.store.DeleteExperiment(r.Context(), id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) exportExperiment(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format != "csv" && format != "json" {
		writeError(w, http.StatusBadRequest, "validation_error", "Формат экспорта должен быть csv или json.", nil)
		return
	}
	data, err := s.store.ExportExperiment(r.Context(), id, format)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=experiment-%s.%s", id, format))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) persistExperimentTerminal(record store.ExperimentRecord, status string, points []experimentPoint, message *string) (store.ExperimentRecord, error) {
	results, err := json.Marshal(points)
	if err != nil {
		return store.ExperimentRecord{}, err
	}
	record.Status = status
	record.Results = results
	record.ErrorMessage = message
	// The HTTP context may already be cancelled. A short background context is
	// intentionally used so cancellation itself is durably visible in history.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.store.UpdateExperiment(ctx, record)
}

func (s *Server) runExperiment(ctx context.Context, request ExperimentRequest, experimentID string) ([]experimentPoint, error) {
	playersValues, err := numberRangeValues(request.Players, 1, maxPlayers)
	if err != nil {
		return nil, err
	}
	nodesValues, err := numberRangeValues(request.Nodes, 1, maxNodes)
	if err != nil {
		return nil, err
	}
	aValues, err := decimalRangeValues(request.A, false)
	if err != nil {
		return nil, err
	}
	bValues, err := decimalRangeValues(request.B, true)
	if err != nil {
		return nil, err
	}
	product := len(playersValues) * len(nodesValues) * len(aValues) * len(bValues)
	if product > maxExperimentRuns {
		return nil, fmt.Errorf("серия содержит %d запусков, максимум %d", product, maxExperimentRuns)
	}

	points := make([]experimentPoint, 0, product)
	runNumber := int64(0)
	for _, players := range playersValues {
		for _, nodes := range nodesValues {
			for _, a := range aValues {
				for _, b := range bValues {
					if ctx.Err() != nil {
						return points, ctx.Err()
					}
					point, err := s.runExperimentPoint(ctx, request, experimentID, players, nodes, a, b, request.Seed+runNumber)
					if err != nil {
						return points, err
					}
					points = append(points, point)
					runNumber++
				}
			}
		}
	}
	return points, nil
}

func (s *Server) runExperimentPoint(ctx context.Context, request ExperimentRequest, experimentID string, players, nodes int, a, b game.Decimal, seed int64) (experimentPoint, error) {
	modelNodes := make([]game.Node, nodes)
	for index := range modelNodes {
		modelNodes[index] = game.Node{
			ID: fmt.Sprintf("N%d", index+1), Name: fmt.Sprintf("Узел %d", index+1),
			A: a, B: b, Kind: game.CostLinear,
		}
	}
	model := game.Game{Players: players, Nodes: modelNodes}
	assignment := seededAssignment(players, nodes, seed)
	simulation, err := game.Simulate(ctx, model, assignment, game.SimulationOptions{Policy: request.Algorithm, MaxSteps: request.MaxSteps})
	if err != nil {
		return experimentPoint{}, err
	}
	point := experimentPoint{Players: players, Nodes: nodes, A: a, B: b, Steps: len(simulation.Trace), Status: game.CalculationNotComputed}
	if simulation.Status != game.SimulationEquilibrium || !simulation.NashCheck.IsEquilibrium {
		point.Message = "Динамика не доказала равновесие в установленном лимите."
		if simulation.Status == game.SimulationCancelled {
			point.Status = game.CalculationCancelled
		}
	} else {
		finalTime := simulation.Final.TotalTime
		point.TotalTimeNE = &finalTime
		optimized, err := game.Optimize(ctx, model, game.OptimizeOptions{MaxStates: experimentExactStateLimit})
		if err != nil {
			return experimentPoint{}, err
		}
		point.Status = optimized.Optimum.Status
		point.Message = optimized.Optimum.Message
		if optimized.Optimum.Status == game.CalculationExact && optimized.Optimum.Summary != nil {
			optimumTime := optimized.Optimum.Summary.TotalTime
			point.TotalTimeOpt = &optimumTime
			if !optimumTime.IsZero() {
				k, err := finalTime.Div(optimumTime)
				if err != nil {
					return experimentPoint{}, err
				}
				point.KRun = &k
			}
		}
	}

	// Persist every run together with its input and trace so the export carries
	// evidence, not merely a chart-ready aggregate.
	simulationRequest := SimulationRequest{
		Players: players, Nodes: nodeInputsFromGame(model), InitialAssignment: assignment,
		Algorithm: request.Algorithm, MaxSteps: request.MaxSteps, Seed: &seed,
	}
	response := simulationResponseFromResult(simulationRequest, simulation)
	if _, err := s.persistSimulation(ctx, response, &experimentID); err != nil {
		return experimentPoint{}, err
	}
	return point, nil
}

func nodeInputsFromGame(model game.Game) []NodeInput {
	values := make([]NodeInput, len(model.Nodes))
	for index, node := range model.Nodes {
		values[index] = NodeInput{
			ID: node.ID, Name: node.Name, A: node.A, B: node.B, Kind: node.Kind,
			TableValues: append([]game.Decimal(nil), node.TableValues...),
		}
	}
	return values
}

func seededAssignment(players, nodes int, seed int64) []string {
	random := rand.New(rand.NewSource(seed)) // deterministic and persisted with every point
	assignment := make([]string, players)
	for player := range assignment {
		assignment[player] = "N" + strconv.Itoa(random.Intn(nodes)+1)
	}
	return assignment
}

func validateExperimentRequest(request ExperimentRequest) error {
	if request.Algorithm != game.FirstImprovement && request.Algorithm != game.BestResponse {
		return fmt.Errorf("политика динамики должна быть first_improvement или best_response")
	}
	if request.MaxSteps < 1 || request.MaxSteps > maxSimulationSteps {
		return fmt.Errorf("лимит шагов должен быть от 1 до %d", maxSimulationSteps)
	}
	for _, decimalInput := range []struct {
		name  string
		value game.Decimal
	}{
		{"a.from", request.A.From}, {"a.to", request.A.To}, {"a.step", request.A.Step},
		{"b.from", request.B.From}, {"b.to", request.B.To}, {"b.step", request.B.Step},
	} {
		if err := validateInputDecimal(decimalInput.name, decimalInput.value); err != nil {
			return err
		}
	}
	players, err := numberRangeValues(request.Players, 1, maxPlayers)
	if err != nil {
		return err
	}
	nodes, err := numberRangeValues(request.Nodes, 1, maxNodes)
	if err != nil {
		return err
	}
	a, err := decimalRangeValues(request.A, false)
	if err != nil {
		return err
	}
	b, err := decimalRangeValues(request.B, true)
	if err != nil {
		return err
	}
	if len(players)*len(nodes)*len(a)*len(b) > maxExperimentRuns {
		return fmt.Errorf("декартово произведение диапазонов превышает %d запусков", maxExperimentRuns)
	}
	// Every point stores an evidence trace. Bound total trace cells
	// (assignments plus per-node loads) before starting the series, rather than
	// accepting a small HTTP request that expands into an unbounded SQLite payload.
	var traceCells int64
	for _, playerCount := range players {
		for _, nodeCount := range nodes {
			traceCells += int64(playerCount+nodeCount) * int64(len(a)) * int64(len(b)) * int64(request.MaxSteps)
		}
	}
	if traceCells > maxExperimentTraceCells {
		return fmt.Errorf("серия может сохранить до %d ячеек трассы, максимум %d", traceCells, maxExperimentTraceCells)
	}
	return nil
}

func numberRangeValues(value NumberRange, minimum, maximum int) ([]int, error) {
	if value.From < minimum || value.To < value.From || value.To > maximum || value.Step < 1 {
		return nil, fmt.Errorf("целочисленный диапазон должен находиться в [%d,%d], быть возрастающим и иметь положительный шаг", minimum, maximum)
	}
	values := make([]int, 0)
	for current := value.From; current <= value.To; current += value.Step {
		values = append(values, current)
		if len(values) > maxExperimentValues {
			return nil, fmt.Errorf("в одном диапазоне допускается не более %d значений", maxExperimentValues)
		}
		if current > value.To-value.Step { // avoids an integer overflow on the increment.
			break
		}
	}
	return values, nil
}

func decimalRangeValues(value DecimalRange, strictlyPositive bool) ([]game.Decimal, error) {
	if value.From.IsNegative() || value.To.Cmp(value.From) < 0 || !value.Step.IsPositive() {
		return nil, fmt.Errorf("десятичный диапазон должен быть неотрицательным, возрастающим и иметь положительный шаг")
	}
	if strictlyPositive && !value.From.IsPositive() {
		return nil, fmt.Errorf("диапазон коэффициента b должен начинаться со значения больше нуля")
	}
	values := make([]game.Decimal, 0)
	for current := value.From; current.Cmp(value.To) <= 0; current = current.Add(value.Step) {
		values = append(values, current)
		if len(values) > maxExperimentValues {
			return nil, fmt.Errorf("в одном диапазоне допускается не более %d значений", maxExperimentValues)
		}
	}
	return values, nil
}

func experimentResponseFromRecord(record store.ExperimentRecord, includePoints bool) experimentSummaryResponse {
	response := experimentSummaryResponse{
		ID: record.ID, Status: record.Status, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
	if record.ErrorMessage != nil {
		response.Message = *record.ErrorMessage
	}
	var request ExperimentRequest
	if json.Unmarshal(record.Config, &request) == nil {
		response.Request = &request
	}
	if includePoints {
		var points []experimentPoint
		if json.Unmarshal(record.Results, &points) == nil {
			response.Points = points
		}
	}
	return response
}

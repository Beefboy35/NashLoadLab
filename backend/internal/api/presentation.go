package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"nash-load-lab/backend/internal/game"
)

const presentationExactStateLimit uint64 = 100_000

const presentationScalingStepLimit = 500

// presentationCache belongs to the process, not to an HTTP request.  The
// source scenarios are fixed, and the result does not use SQLite, therefore a
// single calculation at server initialization is sufficient for every handler
// instance.  sync.Once also keeps parallel test/server initialization safe.
var presentationCache struct {
	once     sync.Once
	response presentationResponse
	err      error
}

// presentationResponse contains a compact legacy block and the complete,
// prepared evidence used by the static defence deck.  No part of this response
// is read from SQLite: every value is formed by the same game engine used by
// simulations and exact optimization.
type presentationResponse struct {
	// Legacy fields are kept for older clients of GET /api/v1/presentation.
	LinearPreview         []presentationLinearPoint      `json:"linear_preview"`
	EquilibriumComparison []presentationEquilibriumPoint `json:"equilibrium_comparison"`
	PotentialTrace        []presentationPotentialPoint   `json:"potential_trace"`

	// Prepared data for the ten-slide defence deck.
	Scenarios       []presentationScenario    `json:"scenarios"`
	ComplexScenario presentationScenario      `json:"complex_scenario"`
	Series          []presentationSeriesPoint `json:"series"`

	// ScalingSetup and ScalingSeries are the controlled experiment for the
	// defence: only n changes, while the four node functions, the all-on-U1
	// initial state, and first_improvement policy stay fixed.
	ScalingSetup  presentationScalingSetup   `json:"scaling_setup"`
	ScalingSeries []presentationScalingPoint `json:"scaling_series"`
}

type presentationLinearPoint struct {
	Load int          `json:"load"`
	A    game.Decimal `json:"a"`
	B    game.Decimal `json:"b"`
}

type presentationEquilibriumPoint struct {
	Profile   string       `json:"profile"`
	TotalTime game.Decimal `json:"total_time"`
}

type presentationPotentialPoint struct {
	Step      int          `json:"step"`
	Potential game.Decimal `json:"potential"`
}

// presentationScenario is intentionally presentation-oriented rather than a
// persistence DTO.  Every decimal serializes as an exact JSON string, while
// trace and allocation structures are direct products of package game.
type presentationScenario struct {
	ID               string                 `json:"id"`
	Title            string                 `json:"title"`
	Players          int                    `json:"players"`
	Nodes            []NodeInput            `json:"nodes"`
	Algorithm        game.Policy            `json:"algorithm"`
	Initial          game.AllocationSummary `json:"initial"`
	Trace            []game.TraceStep       `json:"trace"`
	Final            game.AllocationSummary `json:"final"`
	Nash             game.NashVerification  `json:"nash"`
	Optimum          game.AllocationSummary `json:"optimum"`
	KRun             game.Decimal           `json:"k_run"`
	PoA              game.Decimal           `json:"poa"`
	WorstEquilibrium game.AllocationSummary `json:"worst_equilibrium"`
}

// presentationSeriesPoint is one deterministic prepared experiment.  It is
// deliberately small enough for a defence slide, but each point still carries
// the actual trace length, the reached equilibrium time, and exact optimum.
type presentationSeriesPoint struct {
	Players      int          `json:"players"`
	Nodes        int          `json:"nodes"`
	Steps        int          `json:"steps"`
	KRun         game.Decimal `json:"k_run"`
	TotalTimeNE  game.Decimal `json:"total_time_ne"`
	TotalTimeOpt game.Decimal `json:"total_time_opt"`
}

// presentationScalingSetup exposes the invariant part of the controlled
// series so a client does not have to infer it from hard-coded slide text.
// InitialNodeID means that every task starts on this one node.
type presentationScalingSetup struct {
	Nodes         []NodeInput `json:"nodes"`
	InitialNodeID string      `json:"initial_node_id"`
	Algorithm     game.Policy `json:"algorithm"`
}

// presentationScalingPoint contains only values actually calculated by the
// game core.  SEquilibrium and KRun stay absent rather than pretending that a
// step-limited run is an equilibrium.  SOptimum and KRun are supplied only
// when exact optimization completed; ExactStatus makes that boundary explicit.
type presentationScalingPoint struct {
	N                 int                    `json:"n"`
	M                 int                    `json:"m"`
	H                 int                    `json:"h"`
	EquilibriumStatus game.SimulationStatus  `json:"equilibrium_status"`
	EquilibriumNote   string                 `json:"equilibrium_note,omitempty"`
	SEquilibrium      *game.Decimal          `json:"s_equilibrium,omitempty"`
	SOptimum          *game.Decimal          `json:"s_optimum,omitempty"`
	KRun              *game.Decimal          `json:"k_run,omitempty"`
	ExactStatus       game.CalculationStatus `json:"exact_status"`
	ExactNote         string                 `json:"exact_note,omitempty"`
}

func (s *Server) handlePresentation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.presentationErr != nil {
		writeInternalError(w, s.presentationErr)
		return
	}
	writeJSON(w, http.StatusOK, s.presentation)
}

// preparedPresentation creates the static evidence package exactly once while
// Server is initialized.  It intentionally uses no request context: a client
// disconnect must not cancel a preparation that makes all later slide requests
// read-only cache reads.
func preparedPresentation() (presentationResponse, error) {
	presentationCache.once.Do(func() {
		presentationCache.response, presentationCache.err = calculatePresentation(context.Background())
	})
	return presentationCache.response, presentationCache.err
}

// calculatePresentation forms the static, reproducible evidence package that
// is cached by preparedPresentation.  It is never invoked by an HTTP request.
func calculatePresentation(ctx context.Context) (presentationResponse, error) {
	presetA, err := presentationPresetRequest("A")
	if err != nil {
		return presentationResponse{}, err
	}
	presetB, err := presentationPresetRequest("B")
	if err != nil {
		return presentationResponse{}, err
	}
	presetC, err := presentationPresetRequest("C")
	if err != nil {
		return presentationResponse{}, err
	}

	scenarios := make([]presentationScenario, 0, 3)
	for _, item := range []struct {
		id      string
		title   string
		request SimulationRequest
	}{
		{id: "A", title: "Сценарий А: выгодный переход", request: presetA},
		{id: "B", title: "Сценарий Б: убывание потенциала", request: presetB},
		{id: "C", title: "Сценарий В: равновесие не всегда оптимально", request: presetC},
	} {
		scenario, scenarioErr := calculatePresentationScenario(ctx, item.id, item.title, item.request)
		if scenarioErr != nil {
			return presentationResponse{}, scenarioErr
		}
		scenarios = append(scenarios, scenario)
	}

	previewModel, err := presetA.toGame()
	if err != nil {
		return presentationResponse{}, fmt.Errorf("подготовить линейный пример: %w", err)
	}
	previewGame := game.Game{Players: 4, Nodes: append([]game.Node(nil), previewModel.Nodes...)}
	if err := previewGame.Validate(); err != nil {
		return presentationResponse{}, fmt.Errorf("проверить линейный пример: %w", err)
	}
	linearPreview, err := calculateLinearPreview(previewGame)
	if err != nil {
		return presentationResponse{}, err
	}

	complexScenario, err := calculatePresentationScenario(ctx, "сложный", "Сложный случай: 8 задач и 4 узла", complexPresentationRequest())
	if err != nil {
		return presentationResponse{}, err
	}
	series, err := calculatePresentationSeries(ctx)
	if err != nil {
		return presentationResponse{}, err
	}
	scalingSetup, scalingSeries, err := calculatePresentationScalingSeries(ctx)
	if err != nil {
		return presentationResponse{}, err
	}

	return presentationResponse{
		LinearPreview:         linearPreview,
		EquilibriumComparison: legacyEquilibriumComparison(scenarios),
		PotentialTrace:        legacyPotentialTrace(scenarios),
		Scenarios:             scenarios,
		ComplexScenario:       complexScenario,
		Series:                series,
		ScalingSetup:          scalingSetup,
		ScalingSeries:         scalingSeries,
	}, nil
}

// presentationPresetRequest reuses the public demonstration presets.
// Converting the request through its validator keeps this endpoint compatible
// with every supported latency kind, including table-valued functions.
func presentationPresetRequest(id string) (SimulationRequest, error) {
	for _, preset := range standardPresets() {
		if preset.ID != id {
			continue
		}
		if _, err := preset.Request.validate(); err != nil {
			return SimulationRequest{}, fmt.Errorf("проверить preset %s: %w", id, err)
		}
		return preset.Request, nil
	}
	return SimulationRequest{}, fmt.Errorf("preset %s не найден", id)
}

func calculatePresentationScenario(ctx context.Context, id, title string, request SimulationRequest) (presentationScenario, error) {
	model, err := request.validate()
	if err != nil {
		return presentationScenario{}, fmt.Errorf("проверить сценарий %s: %w", id, err)
	}
	simulation, err := game.Simulate(ctx, model, request.InitialAssignment, game.SimulationOptions{
		Policy: request.Algorithm, MaxSteps: request.MaxSteps,
	})
	if err != nil {
		return presentationScenario{}, fmt.Errorf("рассчитать сценарий %s: %w", id, err)
	}
	if simulation.Status != game.SimulationEquilibrium || !simulation.NashCheck.IsEquilibrium {
		return presentationScenario{}, fmt.Errorf("сценарий %s не завершился доказанным равновесием", id)
	}

	optimized, err := game.Optimize(ctx, model, game.OptimizeOptions{MaxStates: presentationExactStateLimit})
	if err != nil {
		return presentationScenario{}, fmt.Errorf("оптимизировать сценарий %s: %w", id, err)
	}
	if optimized.Optimum.Status != game.CalculationExact || optimized.Optimum.Summary == nil {
		return presentationScenario{}, fmt.Errorf("точный оптимум сценария %s не вычислен: %s", id, optimized.Optimum.Message)
	}
	if optimized.PoA.Status != game.CalculationExact || optimized.PoAValue == nil || optimized.WorstEquilibrium == nil {
		return presentationScenario{}, fmt.Errorf("точная PoA сценария %s не вычислена: %s", id, optimized.PoA.Message)
	}
	if optimized.Optimum.Summary.TotalTime.IsZero() {
		return presentationScenario{}, fmt.Errorf("для сценария %s отношение K не определено: S_opt = 0", id)
	}
	kRun, err := simulation.Final.TotalTime.Div(optimized.Optimum.Summary.TotalTime)
	if err != nil {
		return presentationScenario{}, fmt.Errorf("вычислить K сценария %s: %w", id, err)
	}
	return presentationScenario{
		ID: id, Title: title, Players: request.Players, Nodes: clonePresentationNodes(request.Nodes), Algorithm: request.Algorithm,
		Initial: simulation.Initial, Trace: simulation.Trace, Final: simulation.Final, Nash: simulation.NashCheck,
		Optimum: *optimized.Optimum.Summary, KRun: kRun, PoA: *optimized.PoAValue, WorstEquilibrium: *optimized.WorstEquilibrium,
	}, nil
}

func clonePresentationNodes(nodes []NodeInput) []NodeInput {
	copyNodes := make([]NodeInput, len(nodes))
	copy(copyNodes, nodes)
	for index := range copyNodes {
		copyNodes[index].TableValues = append([]game.Decimal(nil), nodes[index].TableValues...)
	}
	return copyNodes
}

func calculateLinearPreview(model game.Game) ([]presentationLinearPoint, error) {
	if len(model.Nodes) != 2 {
		return nil, fmt.Errorf("линейный пример должен содержать два узла")
	}
	points := make([]presentationLinearPoint, 0, 4)
	for load := 1; load <= 4; load++ {
		a, err := model.Latency(0, load)
		if err != nil {
			return nil, err
		}
		b, err := model.Latency(1, load)
		if err != nil {
			return nil, err
		}
		points = append(points, presentationLinearPoint{Load: load, A: a, B: b})
	}
	return points, nil
}

func legacyEquilibriumComparison(scenarios []presentationScenario) []presentationEquilibriumPoint {
	// Scenario В starts in (A,A); scenario А reaches (A,B).  Keeping these two
	// old chart points preserves the previous public contract.
	if len(scenarios) < 3 {
		return nil
	}
	return []presentationEquilibriumPoint{
		{Profile: formatProfile(scenarios[2].Final.Assignment), TotalTime: scenarios[2].Final.TotalTime},
		{Profile: formatProfile(scenarios[0].Final.Assignment), TotalTime: scenarios[0].Final.TotalTime},
	}
}

func legacyPotentialTrace(scenarios []presentationScenario) []presentationPotentialPoint {
	if len(scenarios) < 2 {
		return nil
	}
	scenario := scenarios[1]
	points := make([]presentationPotentialPoint, 0, len(scenario.Trace)+1)
	points = append(points, presentationPotentialPoint{Step: 0, Potential: scenario.Initial.Potential})
	for _, trace := range scenario.Trace {
		points = append(points, presentationPotentialPoint{Step: trace.Step, Potential: trace.Potential})
	}
	return points
}

func complexPresentationRequest() SimulationRequest {
	return SimulationRequest{
		Players: 8,
		Nodes: []NodeInput{
			{ID: "У₁", Name: "Узел 1 — линейный", A: game.MustDecimal("0"), B: game.MustDecimal("1"), Kind: game.CostLinear},
			{ID: "У₂", Name: "Узел 2 — линейный", A: game.MustDecimal("0.4"), B: game.MustDecimal("0.8"), Kind: game.CostLinear},
			{ID: "У₃", Name: "Узел 3 — квадратичный", A: game.MustDecimal("0.1"), B: game.MustDecimal("0.18"), Kind: game.CostQuadratic},
			{ID: "У₄", Name: "Узел 4 — табличный", Kind: game.CostTable, TableValues: presentationTableValues(8)},
		},
		InitialAssignment: []string{"У₁", "У₁", "У₁", "У₁", "У₁", "У₁", "У₁", "У₁"},
		Algorithm:         game.FirstImprovement,
		MaxSteps:          100,
	}
}

func presentationTableValues(players int) []game.Decimal {
	values := []string{"0.7", "1.1", "1.6", "2.3", "3.5", "5.2", "7.5", "10.5", "14.2", "18.8"}
	if players > len(values) {
		panic("для подготовленной презентации не хватает табличных значений")
	}
	table := make([]game.Decimal, players)
	for index := range table {
		table[index] = game.MustDecimal(values[index])
	}
	return table
}

func calculatePresentationSeries(ctx context.Context) ([]presentationSeriesPoint, error) {
	configurations := []struct {
		players int
		nodes   int
	}{
		{players: 4, nodes: 2},
		{players: 6, nodes: 2},
		{players: 6, nodes: 3},
		{players: 8, nodes: 3},
		{players: 8, nodes: 4},
		{players: 10, nodes: 4},
	}
	points := make([]presentationSeriesPoint, 0, len(configurations))
	for _, configuration := range configurations {
		request := presentationSeriesRequest(configuration.players, configuration.nodes)
		scenario, err := calculatePresentationScenario(ctx, "серия", "", request)
		if err != nil {
			return nil, err
		}
		points = append(points, presentationSeriesPoint{
			Players: configuration.players, Nodes: configuration.nodes, Steps: len(scenario.Trace), KRun: scenario.KRun,
			TotalTimeNE: scenario.Final.TotalTime, TotalTimeOpt: scenario.Optimum.TotalTime,
		})
	}
	return points, nil
}

func presentationSeriesRequest(players, nodes int) SimulationRequest {
	modelNodes := []NodeInput{
		{ID: "У₁", Name: "Узел 1", A: game.MustDecimal("0"), B: game.MustDecimal("1"), Kind: game.CostLinear},
		{ID: "У₂", Name: "Узел 2", A: game.MustDecimal("0.4"), B: game.MustDecimal("0.8"), Kind: game.CostLinear},
		{ID: "У₃", Name: "Узел 3", A: game.MustDecimal("0.1"), B: game.MustDecimal("0.18"), Kind: game.CostQuadratic},
		{ID: "У₄", Name: "Узел 4", Kind: game.CostTable, TableValues: presentationTableValues(players)},
	}
	assignment := make([]string, players)
	for index := range assignment {
		assignment[index] = "У₁"
	}
	return SimulationRequest{
		Players: players, Nodes: clonePresentationNodes(modelNodes[:nodes]), InitialAssignment: assignment,
		Algorithm: game.FirstImprovement, MaxSteps: 100,
	}
}

// calculatePresentationScalingSeries builds the actual scale experiment used
// by the presentation.  It intentionally does not reuse the old compact
// Series: that legacy sequence varies both n and m and includes a table node,
// which would confound a conclusion about the effect of task count.
func calculatePresentationScalingSeries(ctx context.Context) (presentationScalingSetup, []presentationScalingPoint, error) {
	const initialNodeID = "У₁"
	const policy = game.FirstImprovement

	setup := presentationScalingSetup{
		Nodes:         presentationScalingNodes(),
		InitialNodeID: initialNodeID,
		Algorithm:     policy,
	}
	points := make([]presentationScalingPoint, 0, 3)
	for _, players := range []int{8, 16, 32} {
		request := presentationScalingRequest(players)
		model, err := request.validate()
		if err != nil {
			return presentationScalingSetup{}, nil, fmt.Errorf("проверить масштабную точку n=%d: %w", players, err)
		}

		simulation, err := game.Simulate(ctx, model, request.InitialAssignment, game.SimulationOptions{
			Policy: request.Algorithm, MaxSteps: request.MaxSteps,
		})
		if err != nil {
			return presentationScalingSetup{}, nil, fmt.Errorf("рассчитать масштабную точку n=%d: %w", players, err)
		}

		point := presentationScalingPoint{
			N:                 players,
			M:                 len(request.Nodes),
			H:                 len(simulation.Trace),
			EquilibriumStatus: simulation.Status,
			EquilibriumNote:   simulation.Message,
		}
		if simulation.Status == game.SimulationEquilibrium && simulation.NashCheck.IsEquilibrium {
			point.SEquilibrium = decimalReference(simulation.Final.TotalTime)
		}

		optimized, err := game.Optimize(ctx, model, game.OptimizeOptions{MaxStates: presentationExactStateLimit})
		if err != nil {
			return presentationScalingSetup{}, nil, fmt.Errorf("оптимизировать масштабную точку n=%d: %w", players, err)
		}
		point.ExactStatus = optimized.Optimum.Status
		point.ExactNote = optimized.Optimum.Message
		if optimized.Optimum.Status == game.CalculationExact && optimized.Optimum.Summary != nil {
			point.SOptimum = decimalReference(optimized.Optimum.Summary.TotalTime)
			if point.SEquilibrium != nil && !optimized.Optimum.Summary.TotalTime.IsZero() {
				kRun, err := simulation.Final.TotalTime.Div(optimized.Optimum.Summary.TotalTime)
				if err != nil {
					return presentationScalingSetup{}, nil, fmt.Errorf("вычислить K масштабной точки n=%d: %w", players, err)
				}
				point.KRun = decimalReference(kRun)
			}
		}
		points = append(points, point)
	}
	return setup, points, nil
}

func presentationScalingRequest(players int) SimulationRequest {
	assignment := make([]string, players)
	for index := range assignment {
		assignment[index] = "У₁"
	}
	return SimulationRequest{
		Players:           players,
		Nodes:             presentationScalingNodes(),
		InitialAssignment: assignment,
		Algorithm:         game.FirstImprovement,
		MaxSteps:          presentationScalingStepLimit,
	}
}

// presentationScalingNodes contains only formula-defined node functions.
// The mixture is intentionally fixed for every n and has no table-valued
// function: two linear and two quadratic dependencies make the scale effect
// observable without changing the model between points.
func presentationScalingNodes() []NodeInput {
	return []NodeInput{
		{ID: "У₁", Name: "Узел 1 — линейный", A: game.MustDecimal("0"), B: game.MustDecimal("1"), Kind: game.CostLinear},
		{ID: "У₂", Name: "Узел 2 — линейный", A: game.MustDecimal("0.4"), B: game.MustDecimal("0.8"), Kind: game.CostLinear},
		{ID: "У₃", Name: "Узел 3 — квадратичный", A: game.MustDecimal("0.1"), B: game.MustDecimal("0.18"), Kind: game.CostQuadratic},
		{ID: "У₄", Name: "Узел 4 — квадратичный", A: game.MustDecimal("0.2"), B: game.MustDecimal("0.12"), Kind: game.CostQuadratic},
	}
}

func decimalReference(value game.Decimal) *game.Decimal {
	copyValue := value
	return &copyValue
}

func formatProfile(profile []string) string {
	return "(" + strings.Join(profile, ",") + ")"
}

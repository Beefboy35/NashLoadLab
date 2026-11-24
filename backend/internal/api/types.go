package api

import (
	"fmt"
	"strings"
	"time"

	"nash-load-lab/backend/internal/game"
)

const (
	maxPlayers              = 500
	maxNodes                = 40
	maxNodeIDBytes          = 64
	maxNodeNameBytes        = 120
	maxInputExactCharacters = 256
	maxSimulationSteps      = 500
	defaultExactStates      = 100_000
	maxExactStates          = 1_000_000
	// maxExperimentRuns bounds the Cartesian grid itself. The trace-cell budget
	// below remains a separate safeguard for the amount of persisted evidence.
	maxExperimentRuns       = 10_000
	maxExperimentValues     = 20
	maxExperimentTraceCells = int64(300_000)
)

// NodeInput is the public model representation. Decimal accepts a JSON string
// or JSON number on input and always serializes as an exact string on output.
// For kind "table", table_values contains exactly T_j(1)..T_j(n); a and b
// are deliberately not required.
type NodeInput struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	A           game.Decimal          `json:"a"`
	B           game.Decimal          `json:"b"`
	Kind        game.CostFunctionKind `json:"kind"`
	TableValues []game.Decimal        `json:"table_values,omitempty"`
}

// SimulationRequest describes one reproducible dynamics run.
type SimulationRequest struct {
	Players           int         `json:"players"`
	Nodes             []NodeInput `json:"nodes"`
	InitialAssignment []string    `json:"initial_assignment"`
	Algorithm         game.Policy `json:"algorithm"`
	MaxSteps          int         `json:"max_steps"`
	Seed              *int64      `json:"seed,omitempty"`
}

func (r SimulationRequest) toGame() (game.Game, error) {
	if r.Players < 1 || r.Players > maxPlayers {
		return game.Game{}, fmt.Errorf("число заданий должно быть от 1 до %d", maxPlayers)
	}
	if len(r.Nodes) < 1 || len(r.Nodes) > maxNodes {
		return game.Game{}, fmt.Errorf("число вычислительных узлов должно быть от 1 до %d", maxNodes)
	}
	nodes := make([]game.Node, len(r.Nodes))
	for index, node := range r.Nodes {
		id := strings.TrimSpace(node.ID)
		name := strings.TrimSpace(node.Name)
		if len(id) > maxNodeIDBytes {
			return game.Game{}, fmt.Errorf("узел %d: ID длиннее %d байт", index+1, maxNodeIDBytes)
		}
		if len(name) > maxNodeNameBytes {
			return game.Game{}, fmt.Errorf("узел %d: имя длиннее %d байт", index+1, maxNodeNameBytes)
		}
		if node.Kind == game.CostTable {
			for valueIndex, value := range node.TableValues {
				if err := validateInputDecimal(fmt.Sprintf("узел %d, table_values[%d]", index+1, valueIndex), value); err != nil {
					return game.Game{}, err
				}
			}
		} else {
			if err := validateInputDecimal(fmt.Sprintf("узел %d, a", index+1), node.A); err != nil {
				return game.Game{}, err
			}
			if err := validateInputDecimal(fmt.Sprintf("узел %d, b", index+1), node.B); err != nil {
				return game.Game{}, err
			}
		}
		nodes[index] = game.Node{
			ID: id, Name: name, A: node.A, B: node.B, Kind: node.Kind,
			TableValues: append([]game.Decimal(nil), node.TableValues...),
		}
	}
	model := game.Game{Players: r.Players, Nodes: nodes}
	if err := model.Validate(); err != nil {
		return game.Game{}, err
	}
	return model, nil
}

// validateInputDecimal constrains public coefficients below the broader wire
// parser limit. Derived exact values (especially ratios) can therefore safely
// be larger than an input while still round-tripping through stored JSON.
func validateInputDecimal(field string, value game.Decimal) error {
	if strings.Contains(value.String(), "/") {
		return fmt.Errorf("%s должен быть конечной десятичной записью", field)
	}
	if len(value.String()) > maxInputExactCharacters {
		return fmt.Errorf("%s длиннее %d символов", field, maxInputExactCharacters)
	}
	return nil
}

func (r SimulationRequest) validate() (game.Game, error) {
	model, err := r.toGame()
	if err != nil {
		return game.Game{}, err
	}
	if r.Algorithm != game.FirstImprovement && r.Algorithm != game.BestResponse {
		return game.Game{}, fmt.Errorf("политика динамики должна быть first_improvement или best_response")
	}
	if r.MaxSteps < 1 || r.MaxSteps > maxSimulationSteps {
		return game.Game{}, fmt.Errorf("лимит шагов должен быть от 1 до %d", maxSimulationSteps)
	}
	if err := model.ValidateAssignment(r.InitialAssignment); err != nil {
		return game.Game{}, err
	}
	return model, nil
}

// SimulationResponse is persisted verbatim (apart from storage metadata) and
// returned by both POST and GET routes.
type SimulationResponse struct {
	ID        string                 `json:"id,omitempty"`
	Request   SimulationRequest      `json:"request"`
	Initial   game.AllocationSummary `json:"initial"`
	Trace     []game.TraceStep       `json:"trace"`
	Final     game.AllocationSummary `json:"final"`
	NashCheck game.NashVerification  `json:"nash_check"`
	Status    game.SimulationStatus  `json:"status"`
	Message   string                 `json:"message,omitempty"`
	CreatedAt time.Time              `json:"created_at,omitempty"`
}

type presetResponse struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Request     SimulationRequest `json:"request"`
}

type presetsResponse struct {
	Presets []presetResponse `json:"presets"`
}

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// PreviewRequest is intentionally separate from a simulation: it asks the
// server to evaluate the discrete T_j(k) table without inventing a trace.
type PreviewRequest struct {
	Players int         `json:"players"`
	Nodes   []NodeInput `json:"nodes"`
}

func (r PreviewRequest) toGame() (game.Game, error) {
	return SimulationRequest{Players: r.Players, Nodes: r.Nodes}.toGame()
}

type previewValue struct {
	NodeID string       `json:"node_id"`
	Value  game.Decimal `json:"value"`
}

type previewRow struct {
	Load   int            `json:"load"`
	Values []previewValue `json:"values"`
}

type previewResponse struct {
	Rows []previewRow `json:"rows"`
}

// OptimizeRequest performs only server-side enumeration. InitialAssignment is
// optional; when it is supplied, the server also follows the selected policy
// to provide a comparable found equilibrium and K_run.
type OptimizeRequest struct {
	Players           int         `json:"players"`
	Nodes             []NodeInput `json:"nodes"`
	InitialAssignment []string    `json:"initial_assignment,omitempty"`
	Algorithm         game.Policy `json:"algorithm,omitempty"`
	MaxSteps          int         `json:"max_steps,omitempty"`
	MaxStates         uint64      `json:"max_states"`
}

func (r OptimizeRequest) model() (game.Game, error) {
	return SimulationRequest{Players: r.Players, Nodes: r.Nodes}.toGame()
}

type allocationResponse struct {
	Assignment []string        `json:"assignment,omitempty"`
	Loads      []game.NodeLoad `json:"loads,omitempty"`
	Potential  *game.Decimal   `json:"potential,omitempty"`
	TotalTime  *game.Decimal   `json:"total_time,omitempty"`
}

func responseAllocation(summary *game.AllocationSummary) *allocationResponse {
	if summary == nil {
		return nil
	}
	potential := summary.Potential
	total := summary.TotalTime
	return &allocationResponse{
		Assignment: append([]string(nil), summary.Assignment...),
		Loads:      append([]game.NodeLoad(nil), summary.Loads...),
		Potential:  &potential,
		TotalTime:  &total,
	}
}

type optimizationValueResponse struct {
	Status  game.CalculationStatus `json:"status"`
	Message string                 `json:"message,omitempty"`
	*allocationResponse
}

func responseOptimizationValue(value game.OptimizationValue) optimizationValueResponse {
	return optimizationValueResponse{Status: value.Status, Message: value.Message, allocationResponse: responseAllocation(value.Summary)}
}

type poaResponse struct {
	Status           game.CalculationStatus `json:"status"`
	Message          string                 `json:"message,omitempty"`
	Value            *game.Decimal          `json:"value,omitempty"`
	WorstEquilibrium *allocationResponse    `json:"worst_equilibrium,omitempty"`
}

type optimizationResponse struct {
	Optimum     optimizationValueResponse `json:"optimum"`
	Equilibrium *allocationResponse       `json:"equilibrium,omitempty"`
	KRun        *game.Decimal             `json:"k_run,omitempty"`
	PoA         poaResponse               `json:"poa"`
}

// NumberRange and DecimalRange define a finite reproducible Cartesian grid.
type NumberRange struct {
	From int `json:"from"`
	To   int `json:"to"`
	Step int `json:"step"`
}

type DecimalRange struct {
	From game.Decimal `json:"from"`
	To   game.Decimal `json:"to"`
	Step game.Decimal `json:"step"`
}

type ExperimentRequest struct {
	Players   NumberRange  `json:"players"`
	Nodes     NumberRange  `json:"nodes"`
	A         DecimalRange `json:"a"`
	B         DecimalRange `json:"b"`
	Algorithm game.Policy  `json:"algorithm"`
	Seed      int64        `json:"seed"`
	MaxSteps  int          `json:"max_steps"`
}

type experimentPoint struct {
	Players      int                    `json:"players"`
	Nodes        int                    `json:"nodes"`
	A            game.Decimal           `json:"a"`
	B            game.Decimal           `json:"b"`
	Steps        int                    `json:"steps,omitempty"`
	KRun         *game.Decimal          `json:"k_run,omitempty"`
	TotalTimeNE  *game.Decimal          `json:"total_time_ne,omitempty"`
	TotalTimeOpt *game.Decimal          `json:"total_time_opt,omitempty"`
	Status       game.CalculationStatus `json:"status"`
	Message      string                 `json:"message,omitempty"`
}

type experimentSummaryResponse struct {
	ID        string             `json:"id"`
	Status    string             `json:"status"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at,omitempty"`
	Request   *ExperimentRequest `json:"request,omitempty"`
	Points    []experimentPoint  `json:"points,omitempty"`
	Message   string             `json:"message,omitempty"`
}

type experimentListResponse struct {
	Experiments []experimentSummaryResponse `json:"experiments"`
}

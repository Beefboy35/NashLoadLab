package game

import (
	"context"
	"fmt"
	"strings"
)

// Version is persisted alongside every calculation so experiments can be
// reproduced against the mathematical core that created them.
const Version = "1.1.0"

// CostFunctionKind selects how a node latency T_j(k) is specified.
type CostFunctionKind string

const (
	CostLinear    CostFunctionKind = "linear"
	CostQuadratic CostFunctionKind = "quadratic"
	CostTable     CostFunctionKind = "table"
)

// Node is one computing node. Linear and quadratic nodes use A and B, where
// a>=0 and b>0. A table node uses TableValues[load-1] as T_j(load).
type Node struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	A           Decimal          `json:"a"`
	B           Decimal          `json:"b"`
	Kind        CostFunctionKind `json:"kind"`
	TableValues []Decimal        `json:"table_values,omitempty"`
}

// Game is a finite atomic congestion game with identical jobs.
type Game struct {
	Players int    `json:"players"`
	Nodes   []Node `json:"nodes"`
}

// Policy determines which strict improving move is selected at each step.
type Policy string

const (
	FirstImprovement Policy = "first_improvement"
	BestResponse     Policy = "best_response"
)

// SimulationOptions applies transparent safeguards to a dynamics run.
type SimulationOptions struct {
	Policy   Policy `json:"policy"`
	MaxSteps int    `json:"max_steps"`
}

// SimulationStatus distinguishes a proven equilibrium from an interrupted
// search. A step limit is never silently treated as a proof of equilibrium.
type SimulationStatus string

const (
	SimulationEquilibrium SimulationStatus = "equilibrium"
	SimulationStepLimit   SimulationStatus = "step_limit"
	SimulationCancelled   SimulationStatus = "cancelled"
	SimulationError       SimulationStatus = "error"
)

// CalculationStatus is used by exact optimization and PoA enumeration.
type CalculationStatus string

const (
	CalculationExact       CalculationStatus = "exact"
	CalculationNotComputed CalculationStatus = "not_computed"
	CalculationCancelled   CalculationStatus = "cancelled"
	CalculationError       CalculationStatus = "error"
)

// NodeLoad is a server-calculated allocation row. Latency is absent for an
// empty node because T_j(0) is deliberately undefined by the model.
type NodeLoad struct {
	NodeID   string   `json:"node_id"`
	NodeName string   `json:"node_name,omitempty"`
	Load     int      `json:"load"`
	Latency  *Decimal `json:"latency,omitempty"`
}

// AllocationSummary is the exact state shown in a trace, comparison, or
// optimization result.
type AllocationSummary struct {
	Assignment []string   `json:"assignment"`
	Loads      []NodeLoad `json:"loads"`
	Potential  Decimal    `json:"potential"`
	TotalTime  Decimal    `json:"total_time"`
}

// TraceStep describes one actual strict unilateral improvement. Player uses a
// one-based public number so the UI can say "Задание 1" naturally.
type TraceStep struct {
	Step      int               `json:"step"`
	Player    int               `json:"player"`
	From      string            `json:"from"`
	To        string            `json:"to"`
	Previous  Decimal           `json:"previous"`
	Next      Decimal           `json:"next"`
	Delta     Decimal           `json:"delta"`
	Potential Decimal           `json:"potential"`
	TotalTime Decimal           `json:"total_time"`
	Summary   AllocationSummary `json:"summary"`
}

// ImprovingMove is an independently discovered violation of the Nash
// condition. Its presence means the state is not an equilibrium.
type ImprovingMove struct {
	Player    int     `json:"player"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	Current   Decimal `json:"current"`
	Candidate Decimal `json:"candidate"`
}

// NashVerification records a fresh, exhaustive check of all legal unilateral
// moves rather than relying only on how the dynamics happened to stop.
type NashVerification struct {
	IsEquilibrium bool            `json:"is_equilibrium"`
	CheckedMoves  int             `json:"checked_moves"`
	Violations    []ImprovingMove `json:"violations,omitempty"`
	Explanation   string          `json:"explanation"`
}

// SimulationResult contains a trace beginning at step zero (Initial) and the
// independent terminal verification.
type SimulationResult struct {
	Initial   AllocationSummary `json:"initial"`
	Trace     []TraceStep       `json:"trace"`
	Final     AllocationSummary `json:"final"`
	NashCheck NashVerification  `json:"nash_check"`
	Status    SimulationStatus  `json:"status"`
	Message   string            `json:"message,omitempty"`
}

// OptimizeOptions bounds an exhaustive enumeration of load vectors.
type OptimizeOptions struct {
	MaxStates uint64 `json:"max_states"`
}

// OptimizationValue is either an exact result or an explicitly non-computed
// outcome; a missing result is never represented by zero.
type OptimizationValue struct {
	Status  CalculationStatus  `json:"status"`
	Summary *AllocationSummary `json:"summary,omitempty"`
	Message string             `json:"message,omitempty"`
}

// OptimizationResult keeps optimum and PoA calculation states distinct. PoA
// is only present as exact after every bounded candidate load vector has been
// checked for the Nash condition.
type OptimizationResult struct {
	Optimum          OptimizationValue  `json:"optimum"`
	PoA              OptimizationValue  `json:"poa"`
	PoAValue         *Decimal           `json:"poa_value,omitempty"`
	WorstEquilibrium *AllocationSummary `json:"worst_equilibrium,omitempty"`
}

// Validate verifies game invariants before any calculation.
func (g Game) Validate() error {
	if g.Players < 1 {
		return fmt.Errorf("число заданий должно быть не меньше 1")
	}
	if len(g.Nodes) < 1 {
		return fmt.Errorf("нужен хотя бы один вычислительный узел")
	}
	ids := make(map[string]struct{}, len(g.Nodes))
	for index, node := range g.Nodes {
		id := strings.TrimSpace(node.ID)
		if id == "" {
			return fmt.Errorf("узел %d: ID не должен быть пустым", index+1)
		}
		if _, exists := ids[id]; exists {
			return fmt.Errorf("ID узла %q повторяется", id)
		}
		ids[id] = struct{}{}
		switch node.Kind {
		case CostLinear, CostQuadratic:
			if node.A.IsNegative() {
				return fmt.Errorf("узел %q: коэффициент a должен быть неотрицательным", id)
			}
			if !node.B.IsPositive() {
				return fmt.Errorf("узел %q: коэффициент b должен быть больше нуля", id)
			}
		case CostTable:
			if len(node.TableValues) != g.Players {
				return fmt.Errorf("узел %q: таблица значений должна содержать ровно %d значений T_j(1)..T_j(%d)", id, g.Players, g.Players)
			}
			for valueIndex, value := range node.TableValues {
				if value.IsNegative() {
					return fmt.Errorf("узел %q: значение таблицы T_j(%d) не должно быть отрицательным", id, valueIndex+1)
				}
				if valueIndex > 0 && value.Cmp(node.TableValues[valueIndex-1]) < 0 {
					return fmt.Errorf("узел %q: таблица значений должна быть неубывающей (T_j(%d) < T_j(%d))", id, valueIndex+1, valueIndex)
				}
			}
		default:
			return fmt.Errorf("узел %q: неизвестный вид функции %q", id, node.Kind)
		}
	}
	return nil
}

// ValidateAssignment confirms that every player selects one existing node.
func (g Game) ValidateAssignment(assignment []string) error {
	if err := g.Validate(); err != nil {
		return err
	}
	if len(assignment) != g.Players {
		return fmt.Errorf("длина начального распределения (%d) должна совпадать с числом заданий (%d)", len(assignment), g.Players)
	}
	known := make(map[string]struct{}, len(g.Nodes))
	for _, node := range g.Nodes {
		known[node.ID] = struct{}{}
	}
	for player, nodeID := range assignment {
		if _, ok := known[nodeID]; !ok {
			return fmt.Errorf("задание %d ссылается на неизвестный узел %q", player+1, nodeID)
		}
	}
	return nil
}

func (g Game) nodeIndex(id string) (int, bool) {
	for index, node := range g.Nodes {
		if node.ID == id {
			return index, true
		}
	}
	return 0, false
}

// Time returns T_j(load) for load>=1. It is the single latency evaluator used
// by every algorithm, independently of how the node function was specified.
func (node Node) Time(load int) (Decimal, error) {
	if load < 1 {
		return Decimal{}, fmt.Errorf("нагрузка должна быть не меньше 1")
	}
	switch node.Kind {
	case CostLinear, CostQuadratic:
		loadValue := DecimalFromInt(load)
		if node.Kind == CostQuadratic {
			loadValue = loadValue.Mul(loadValue)
		}
		return node.A.Add(node.B.Mul(loadValue)), nil
	case CostTable:
		if load > len(node.TableValues) {
			return Decimal{}, fmt.Errorf("таблица узла %q не содержит значения T_j(%d)", node.ID, load)
		}
		return node.TableValues[load-1], nil
	default:
		return Decimal{}, fmt.Errorf("узел %q: неизвестный вид функции %q", node.ID, node.Kind)
	}
}

// Latency preserves the game-level API while delegating all latency logic to
// Node.Time. Mathematical algorithms therefore never need to branch on Kind.
func (g Game) Latency(nodeIndex, load int) (Decimal, error) {
	if nodeIndex < 0 || nodeIndex >= len(g.Nodes) {
		return Decimal{}, fmt.Errorf("индекс узла %d вне диапазона", nodeIndex)
	}
	return g.Nodes[nodeIndex].Time(load)
}

func contextCancelled(ctx context.Context) bool {
	return ctx != nil && ctx.Err() != nil
}

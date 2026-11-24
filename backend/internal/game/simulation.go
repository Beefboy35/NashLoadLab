package game

import (
	"context"
	"fmt"
)

// Potential evaluates Rosenthal's exact potential
// F(s)=sum_j sum_(q=1)^n_j T_j(q).
func (g Game) Potential(assignment []string) (Decimal, error) {
	loads, err := g.loadCounts(assignment)
	if err != nil {
		return Decimal{}, err
	}
	return g.potentialForLoads(loads)
}

// TotalTime evaluates S(s)=sum_j n_j*T_j(n_j), which is intentionally a
// different quantity from the potential function.
func (g Game) TotalTime(assignment []string) (Decimal, error) {
	loads, err := g.loadCounts(assignment)
	if err != nil {
		return Decimal{}, err
	}
	return g.totalForLoads(loads)
}

// Summary constructs the complete immutable presentation state for an
// assignment, including per-node loads and exact F/S values.
func (g Game) Summary(assignment []string) (AllocationSummary, error) {
	loads, err := g.loadCounts(assignment)
	if err != nil {
		return AllocationSummary{}, err
	}
	return g.summaryForLoads(assignment, loads)
}

func (g Game) loadCounts(assignment []string) ([]int, error) {
	if err := g.ValidateAssignment(assignment); err != nil {
		return nil, err
	}
	loads := make([]int, len(g.Nodes))
	for _, nodeID := range assignment {
		index, _ := g.nodeIndex(nodeID)
		loads[index]++
	}
	return loads, nil
}

func (g Game) potentialForLoads(loads []int) (Decimal, error) {
	if len(loads) != len(g.Nodes) {
		return Decimal{}, fmt.Errorf("длина вектора нагрузок не совпадает с числом узлов")
	}
	total := Zero()
	for nodeIndex, load := range loads {
		if load < 0 {
			return Decimal{}, fmt.Errorf("нагрузка узла %d не может быть отрицательной", nodeIndex+1)
		}
		for level := 1; level <= load; level++ {
			latency, err := g.Latency(nodeIndex, level)
			if err != nil {
				return Decimal{}, err
			}
			total = total.Add(latency)
		}
	}
	return total, nil
}

func (g Game) totalForLoads(loads []int) (Decimal, error) {
	if len(loads) != len(g.Nodes) {
		return Decimal{}, fmt.Errorf("длина вектора нагрузок не совпадает с числом узлов")
	}
	total := Zero()
	for nodeIndex, load := range loads {
		if load < 0 {
			return Decimal{}, fmt.Errorf("нагрузка узла %d не может быть отрицательной", nodeIndex+1)
		}
		if load == 0 {
			continue
		}
		latency, err := g.Latency(nodeIndex, load)
		if err != nil {
			return Decimal{}, err
		}
		total = total.Add(latency.MulInt(load))
	}
	return total, nil
}

func (g Game) summaryForLoads(assignment []string, loads []int) (AllocationSummary, error) {
	potential, err := g.potentialForLoads(loads)
	if err != nil {
		return AllocationSummary{}, err
	}
	total, err := g.totalForLoads(loads)
	if err != nil {
		return AllocationSummary{}, err
	}
	rows := make([]NodeLoad, len(g.Nodes))
	for index, node := range g.Nodes {
		name := node.Name
		if name == "" {
			name = node.ID
		}
		rows[index] = NodeLoad{NodeID: node.ID, NodeName: name, Load: loads[index]}
		if loads[index] > 0 {
			latency, err := g.Latency(index, loads[index])
			if err != nil {
				return AllocationSummary{}, err
			}
			rows[index].Latency = &latency
		}
	}
	assignmentCopy := append([]string(nil), assignment...)
	return AllocationSummary{Assignment: assignmentCopy, Loads: rows, Potential: potential, TotalTime: total}, nil
}

// VerifyNash independently evaluates every player's legal unilateral move.
func VerifyNash(ctx context.Context, g Game, assignment []string) (NashVerification, error) {
	loads, err := g.loadCounts(assignment)
	if err != nil {
		return NashVerification{}, err
	}
	verification := NashVerification{Violations: make([]ImprovingMove, 0)}
	for player, sourceID := range assignment {
		if contextCancelled(ctx) {
			return NashVerification{}, ctx.Err()
		}
		source, _ := g.nodeIndex(sourceID)
		current, err := g.Latency(source, loads[source])
		if err != nil {
			return NashVerification{}, err
		}
		for target := range g.Nodes {
			if target == source {
				continue
			}
			verification.CheckedMoves++
			candidate, err := g.Latency(target, loads[target]+1)
			if err != nil {
				return NashVerification{}, err
			}
			if candidate.Cmp(current) < 0 {
				verification.Violations = append(verification.Violations, ImprovingMove{
					Player: player + 1, From: g.Nodes[source].ID, To: g.Nodes[target].ID,
					Current: current, Candidate: candidate,
				})
			}
		}
	}
	verification.IsEquilibrium = len(verification.Violations) == 0
	if verification.IsEquilibrium {
		verification.Explanation = "Независимая проверка не нашла ни одного выгодного одностороннего перехода."
	} else {
		verification.Explanation = fmt.Sprintf("Независимая проверка нашла %d выгодных односторонних переходов.", len(verification.Violations))
	}
	return verification, nil
}

type moveChoice struct {
	player         int
	from, to       int
	previous, next Decimal
}

func (g Game) findImprovement(ctx context.Context, assignment []string, loads []int, policy Policy) (*moveChoice, error) {
	for player, sourceID := range assignment {
		if contextCancelled(ctx) {
			return nil, ctx.Err()
		}
		source, _ := g.nodeIndex(sourceID)
		current, err := g.Latency(source, loads[source])
		if err != nil {
			return nil, err
		}
		var best *moveChoice
		for target := range g.Nodes {
			if target == source {
				continue
			}
			candidate, err := g.Latency(target, loads[target]+1)
			if err != nil {
				return nil, err
			}
			if candidate.Cmp(current) >= 0 {
				continue
			}
			choice := &moveChoice{player: player, from: source, to: target, previous: current, next: candidate}
			if policy == FirstImprovement {
				return choice, nil
			}
			// Equal best responses are resolved by the stable node ID, not by an
			// incidental map or goroutine order.
			if best == nil || candidate.Cmp(best.next) < 0 || (candidate.Cmp(best.next) == 0 && g.Nodes[target].ID < g.Nodes[best.to].ID) {
				best = choice
			}
		}
		if policy == BestResponse && best != nil {
			return best, nil
		}
	}
	return nil, nil
}

// Simulate follows deterministic strict improvements until a state is proven
// stable, the explicitly requested step cap is reached, or its context ends.
func Simulate(ctx context.Context, g Game, initial []string, options SimulationOptions) (SimulationResult, error) {
	if err := g.ValidateAssignment(initial); err != nil {
		return SimulationResult{}, err
	}
	if options.Policy == "" {
		options.Policy = FirstImprovement
	}
	if options.Policy != FirstImprovement && options.Policy != BestResponse {
		return SimulationResult{}, fmt.Errorf("неизвестная политика динамики %q", options.Policy)
	}
	if options.MaxSteps < 1 {
		return SimulationResult{}, fmt.Errorf("лимит шагов должен быть не меньше 1")
	}

	assignment := append([]string(nil), initial...)
	loads, err := g.loadCounts(assignment)
	if err != nil {
		return SimulationResult{}, err
	}
	initialSummary, err := g.summaryForLoads(assignment, loads)
	if err != nil {
		return SimulationResult{}, err
	}
	result := SimulationResult{Initial: initialSummary, Final: initialSummary, Trace: make([]TraceStep, 0)}

	for step := 1; step <= options.MaxSteps; step++ {
		if contextCancelled(ctx) {
			result.Status = SimulationCancelled
			result.Message = "Расчёт остановлен отменой запроса."
			return result, nil
		}
		choice, err := g.findImprovement(ctx, assignment, loads, options.Policy)
		if err != nil {
			if contextCancelled(ctx) {
				result.Status = SimulationCancelled
				result.Message = "Расчёт остановлен отменой запроса."
				return result, nil
			}
			return SimulationResult{}, err
		}
		if choice == nil {
			verification, err := VerifyNash(ctx, g, assignment)
			if err != nil {
				if contextCancelled(ctx) {
					result.Status = SimulationCancelled
					result.Message = "Расчёт остановлен отменой запроса."
					return result, nil
				}
				return SimulationResult{}, err
			}
			result.NashCheck = verification
			result.Status = SimulationEquilibrium
			result.Message = "Все строгие улучшения исчерпаны; распределение является равновесием Нэша."
			return result, nil
		}

		beforePotential := result.Final.Potential
		assignment[choice.player] = g.Nodes[choice.to].ID
		loads[choice.from]--
		loads[choice.to]++
		summary, err := g.summaryForLoads(assignment, loads)
		if err != nil {
			return SimulationResult{}, err
		}
		delta := choice.next.Sub(choice.previous)
		if summary.Potential.Sub(beforePotential).Cmp(delta) != 0 {
			return SimulationResult{}, fmt.Errorf("нарушено тождество ΔF=Δt на шаге %d", step)
		}
		result.Trace = append(result.Trace, TraceStep{
			Step: step, Player: choice.player + 1, From: g.Nodes[choice.from].ID, To: g.Nodes[choice.to].ID,
			Previous: choice.previous, Next: choice.next, Delta: delta,
			Potential: summary.Potential, TotalTime: summary.TotalTime, Summary: compactTraceSummary(summary),
		})
		result.Final = summary
	}

	verification, err := VerifyNash(ctx, g, assignment)
	if err != nil {
		if contextCancelled(ctx) {
			result.Status = SimulationCancelled
			result.Message = "Расчёт остановлен отменой запроса."
			return result, nil
		}
		return SimulationResult{}, err
	}
	result.NashCheck = verification
	result.Status = SimulationStepLimit
	if verification.IsEquilibrium {
		result.Message = "Достигнут лимит шагов; независимая проверка уже подтверждает равновесие, но остановка не была выведена из самой динамики."
	} else {
		result.Message = "Достигнут лимит шагов до равновесия; это не является доказательством устойчивости."
	}
	return result, nil
}

// compactTraceSummary preserves every server-calculated state needed by the
// step player while omitting node labels repeated in every snapshot. Clients
// already receive labels with the immutable model request, so repeating them
// turns a long trace into avoidable storage and response amplification.
func compactTraceSummary(summary AllocationSummary) AllocationSummary {
	loads := make([]NodeLoad, len(summary.Loads))
	copy(loads, summary.Loads)
	for index := range loads {
		loads[index].NodeName = ""
	}
	return AllocationSummary{
		Assignment: append([]string(nil), summary.Assignment...),
		Loads:      loads,
		Potential:  summary.Potential,
		TotalTime:  summary.TotalTime,
	}
}

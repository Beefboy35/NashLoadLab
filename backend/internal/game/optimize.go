package game

import (
	"context"
	"fmt"
	"math/big"
)

// Optimize performs an exact enumeration of integer load vectors
// n_1+...+n_m=n. It never substitutes a partial search for an exact optimum or
// for PoA: if the configured state cap would be exceeded, both values are
// explicitly marked not_computed.
func Optimize(ctx context.Context, g Game, options OptimizeOptions) (OptimizationResult, error) {
	if err := g.Validate(); err != nil {
		return OptimizationResult{}, err
	}
	if options.MaxStates == 0 {
		return notComputedOptimization("Лимит точного перебора равен нулю."), nil
	}
	states, exceeds := compositionCountAtMost(g.Players, len(g.Nodes), options.MaxStates)
	if exceeds {
		return notComputedOptimization(fmt.Sprintf("Точный перебор требует более %d состояний и не был запущен.", options.MaxStates)), nil
	}

	loads := make([]int, len(g.Nodes))
	var optimum *AllocationSummary
	var worstEquilibrium *AllocationSummary
	var enumerate func(position, remaining int) error
	enumerate = func(position, remaining int) error {
		if contextCancelled(ctx) {
			return ctx.Err()
		}
		if position == len(loads)-1 {
			loads[position] = remaining
			return evaluateLoadVector(g, loads, &optimum, &worstEquilibrium)
		}
		for value := 0; value <= remaining; value++ {
			loads[position] = value
			if err := enumerate(position+1, remaining-value); err != nil {
				return err
			}
		}
		return nil
	}
	if err := enumerate(0, g.Players); err != nil {
		if contextCancelled(ctx) {
			message := "Точный перебор отменён до завершения."
			return OptimizationResult{
				Optimum: OptimizationValue{Status: CalculationCancelled, Message: message},
				PoA:     OptimizationValue{Status: CalculationCancelled, Message: message},
			}, nil
		}
		return OptimizationResult{}, err
	}
	if optimum == nil || worstEquilibrium == nil {
		// A finite exact potential game always has a pure NE, so this would be
		// an internal defect rather than a legitimate no-result condition.
		return OptimizationResult{}, fmt.Errorf("точный перебор %d состояний не нашёл оптимум или равновесие", states)
	}

	result := OptimizationResult{
		Optimum:          OptimizationValue{Status: CalculationExact, Summary: optimum, Message: fmt.Sprintf("Точно перебрано состояний: %d.", states)},
		PoA:              OptimizationValue{Status: CalculationExact, Summary: worstEquilibrium, Message: fmt.Sprintf("Точно проверено состояний: %d.", states)},
		WorstEquilibrium: worstEquilibrium,
	}
	if optimum.TotalTime.IsZero() {
		result.PoA.Message = "Точно перебрано, но PoA не определена: S_opt = 0."
		return result, nil
	}
	poa, err := worstEquilibrium.TotalTime.Div(optimum.TotalTime)
	if err != nil {
		return OptimizationResult{}, err
	}
	result.PoAValue = &poa
	return result, nil
}

func notComputedOptimization(message string) OptimizationResult {
	return OptimizationResult{
		Optimum: OptimizationValue{Status: CalculationNotComputed, Message: message},
		PoA:     OptimizationValue{Status: CalculationNotComputed, Message: message},
	}
}

func evaluateLoadVector(g Game, loads []int, optimum, worstEquilibrium **AllocationSummary) error {
	assignment := g.assignmentForLoads(loads)
	summary, err := g.summaryForLoads(assignment, loads)
	if err != nil {
		return err
	}
	if *optimum == nil || summary.TotalTime.Cmp((*optimum).TotalTime) < 0 {
		copySummary := summary
		*optimum = &copySummary
	}
	isNash, err := g.loadsAreNash(loads)
	if err != nil {
		return err
	}
	if isNash && (*worstEquilibrium == nil || summary.TotalTime.Cmp((*worstEquilibrium).TotalTime) > 0) {
		copySummary := summary
		*worstEquilibrium = &copySummary
	}
	return nil
}

func (g Game) assignmentForLoads(loads []int) []string {
	assignment := make([]string, 0, g.Players)
	for nodeIndex, load := range loads {
		for number := 0; number < load; number++ {
			assignment = append(assignment, g.Nodes[nodeIndex].ID)
		}
	}
	return assignment
}

func (g Game) loadsAreNash(loads []int) (bool, error) {
	for source, sourceLoad := range loads {
		if sourceLoad == 0 {
			continue
		}
		current, err := g.Latency(source, sourceLoad)
		if err != nil {
			return false, err
		}
		for target := range loads {
			if target == source {
				continue
			}
			candidate, err := g.Latency(target, loads[target]+1)
			if err != nil {
				return false, err
			}
			if candidate.Cmp(current) < 0 {
				return false, nil
			}
		}
	}
	return true, nil
}

// compositionCountAtMost returns C(players+nodes-1,nodes-1) exactly until it
// passes cap. Big integers avoid overflow while still making a bounded API
// decision without enumerating any states.
func compositionCountAtMost(players, nodes int, cap uint64) (uint64, bool) {
	if players < 0 || nodes < 1 {
		return 0, true
	}
	result := big.NewInt(1)
	limit := new(big.Int).SetUint64(cap)
	for k := 1; k <= nodes-1; k++ {
		result.Mul(result, big.NewInt(int64(players+k)))
		result.Quo(result, big.NewInt(int64(k)))
		if result.Cmp(limit) > 0 {
			return cap, true
		}
	}
	return result.Uint64(), false
}

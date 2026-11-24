package game

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func linearNode(id, a, b string) Node {
	return Node{ID: id, Name: id, A: MustDecimal(a), B: MustDecimal(b), Kind: CostLinear}
}

func tableNode(id string, values ...string) Node {
	table := make([]Decimal, len(values))
	for index, value := range values {
		table[index] = MustDecimal(value)
	}
	return Node{ID: id, Name: id, Kind: CostTable, TableValues: table}
}

func TestPresetATraceAndPoA(t *testing.T) {
	t.Parallel()
	game := Game{Players: 2, Nodes: []Node{linearNode("A", "0", "1"), linearNode("B", "0", "2")}}
	result, err := Simulate(context.Background(), game, []string{"B", "B"}, SimulationOptions{Policy: FirstImprovement, MaxSteps: 10})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if got := result.Initial.Potential.String(); got != "6" {
		t.Fatalf("initial F = %s, want 6", got)
	}
	if got := result.Initial.TotalTime.String(); got != "8" {
		t.Fatalf("initial S = %s, want 8", got)
	}
	if len(result.Trace) != 1 {
		t.Fatalf("trace length = %d, want 1", len(result.Trace))
	}
	move := result.Trace[0]
	if move.Player != 1 || move.From != "B" || move.To != "A" {
		t.Fatalf("unexpected move: %#v", move)
	}
	if move.Previous.String() != "4" || move.Next.String() != "1" || move.Delta.String() != "-3" {
		t.Fatalf("unexpected move values: previous=%s next=%s delta=%s", move.Previous, move.Next, move.Delta)
	}
	if move.Summary.TotalTime.String() != "3" || len(move.Summary.Loads) != 2 || move.Summary.Assignment[0] != "A" {
		t.Fatalf("server step summary is incomplete: %#v", move.Summary)
	}
	if move.Summary.Loads[0].NodeName != "" || result.Final.Loads[0].NodeName == "" {
		t.Fatalf("trace should compact repeated node labels without changing final summary: %#v / %#v", move.Summary.Loads, result.Final.Loads)
	}
	if got := result.Final.TotalTime.String(); got != "3" {
		t.Fatalf("final S = %s, want 3", got)
	}
	if result.Status != SimulationEquilibrium || !result.NashCheck.IsEquilibrium {
		t.Fatalf("simulation did not prove equilibrium: %#v", result)
	}

	optimized, err := Optimize(context.Background(), game, OptimizeOptions{MaxStates: 100})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if optimized.Optimum.Status != CalculationExact || optimized.Optimum.Summary == nil {
		t.Fatalf("optimum status = %#v", optimized.Optimum)
	}
	if got := optimized.Optimum.Summary.TotalTime.String(); got != "3" {
		t.Fatalf("optimum S = %s, want 3", got)
	}
	if optimized.PoAValue == nil || optimized.PoAValue.String() != "4/3" {
		t.Fatalf("PoA = %v, want 4/3", optimized.PoAValue)
	}
}

func TestPresetBPotentialSteps(t *testing.T) {
	t.Parallel()
	game := Game{Players: 4, Nodes: []Node{
		linearNode("A", "0", "1"),
		linearNode("B", "1", "0.5"),
	}}
	result, err := Simulate(context.Background(), game, []string{"A", "A", "A", "A"}, SimulationOptions{Policy: FirstImprovement, MaxSteps: 10})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if len(result.Trace) != 2 {
		t.Fatalf("trace length = %d, want 2", len(result.Trace))
	}
	wantPotential := []string{"7.5", "6.5"}
	wantTotal := []string{"10.5", "8"}
	wantDelta := []string{"-2.5", "-1"}
	for index, step := range result.Trace {
		if step.Potential.String() != wantPotential[index] || step.TotalTime.String() != wantTotal[index] || step.Delta.String() != wantDelta[index] {
			t.Fatalf("step %d = F %s S %s delta %s", index+1, step.Potential, step.TotalTime, step.Delta)
		}
		before := result.Initial.Potential
		if index > 0 {
			before = result.Trace[index-1].Potential
		}
		if step.Potential.Sub(before).Cmp(step.Delta) != 0 {
			t.Fatalf("step %d violates delta F = delta t", index+1)
		}
	}
	if !result.NashCheck.IsEquilibrium || result.Final.TotalTime.String() != "8" {
		t.Fatalf("final = %#v", result.Final)
	}
}

func TestPresetCStableButNotOptimal(t *testing.T) {
	t.Parallel()
	game := Game{Players: 2, Nodes: []Node{linearNode("A", "0", "1"), linearNode("B", "0", "2")}}
	result, err := Simulate(context.Background(), game, []string{"A", "A"}, SimulationOptions{Policy: BestResponse, MaxSteps: 5})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if len(result.Trace) != 0 || !result.NashCheck.IsEquilibrium || result.Final.TotalTime.String() != "4" {
		t.Fatalf("expected stable S=4, got %#v", result)
	}
	optimized, err := Optimize(context.Background(), game, OptimizeOptions{MaxStates: 10})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	k, err := result.Final.TotalTime.Div(optimized.Optimum.Summary.TotalTime)
	if err != nil || k.String() != "4/3" {
		t.Fatalf("K_run = %v, %v", k, err)
	}
}

func TestEqualLatencyDoesNotMove(t *testing.T) {
	t.Parallel()
	game := Game{Players: 1, Nodes: []Node{linearNode("A", "0", "1"), linearNode("B", "0", "1")}}
	result, err := Simulate(context.Background(), game, []string{"A"}, SimulationOptions{Policy: FirstImprovement, MaxSteps: 2})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if len(result.Trace) != 0 || !result.NashCheck.IsEquilibrium {
		t.Fatalf("equality must not be a strict improvement: %#v", result)
	}
}

func TestQuadraticAndStateLimit(t *testing.T) {
	t.Parallel()
	game := Game{Players: 3, Nodes: []Node{
		{ID: "A", A: MustDecimal("0"), B: MustDecimal("1"), Kind: CostQuadratic},
		linearNode("B", "0", "1"),
	}}
	latency, err := game.Latency(0, 3)
	if err != nil || latency.String() != "9" {
		t.Fatalf("quadratic latency = %v, %v", latency, err)
	}
	optimized, err := Optimize(context.Background(), game, OptimizeOptions{MaxStates: 3})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if optimized.Optimum.Status != CalculationNotComputed || optimized.PoA.Status != CalculationNotComputed {
		t.Fatalf("state cap was silently ignored: %#v", optimized)
	}
}

func TestTableFunctionNashExample(t *testing.T) {
	t.Parallel()
	game := Game{Players: 4, Nodes: []Node{
		tableNode("A", "1", "2", "5", "9"),
		tableNode("B", "2", "3", "4", "6"),
	}}
	if latency, err := game.Nodes[0].Time(3); err != nil || latency.String() != "5" {
		t.Fatalf("A(3) = %v, %v; want 5", latency, err)
	}
	if _, err := game.Nodes[1].Time(5); err == nil {
		t.Fatal("table lookup beyond the supplied values was accepted")
	}

	result, err := Simulate(context.Background(), game, []string{"A", "A", "B", "B"}, SimulationOptions{Policy: FirstImprovement, MaxSteps: 10})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if len(result.Trace) != 0 || result.Status != SimulationEquilibrium || !result.NashCheck.IsEquilibrium {
		t.Fatalf("A=2, B=2 must be an equilibrium: %#v", result)
	}
	if result.Final.Potential.String() != "8" || result.Final.TotalTime.String() != "10" {
		t.Fatalf("table summary: F=%s, S=%s; want F=8, S=10", result.Final.Potential, result.Final.TotalTime)
	}

	optimized, err := Optimize(context.Background(), game, OptimizeOptions{MaxStates: 100})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if optimized.Optimum.Status != CalculationExact || optimized.Optimum.Summary == nil || optimized.Optimum.Summary.TotalTime.String() != "10" {
		t.Fatalf("table optimum = %#v; want exact S=10", optimized.Optimum)
	}
}

func TestTableFunctionStrictMovesPreservePotentialIdentity(t *testing.T) {
	t.Parallel()
	game := Game{Players: 4, Nodes: []Node{
		tableNode("A", "1", "2", "5", "9"),
		tableNode("B", "2", "3", "4", "6"),
	}}
	result, err := Simulate(context.Background(), game, []string{"A", "A", "A", "A"}, SimulationOptions{Policy: FirstImprovement, MaxSteps: 10})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if len(result.Trace) != 2 || result.Status != SimulationEquilibrium || !result.NashCheck.IsEquilibrium {
		t.Fatalf("table dynamics did not reach the expected equilibrium: %#v", result)
	}
	wantDeltas := []string{"-7", "-2"}
	for index, step := range result.Trace {
		beforePotential := result.Initial.Potential
		if index > 0 {
			beforePotential = result.Trace[index-1].Potential
		}
		if step.Delta.String() != wantDeltas[index] || step.Potential.Sub(beforePotential).Cmp(step.Delta) != 0 {
			t.Fatalf("table step %d violates ΔF=Δt: before F=%s, after F=%s, Δt=%s", index+1, beforePotential, step.Potential, step.Delta)
		}
	}
	if result.Final.Potential.String() != "8" || result.Final.TotalTime.String() != "10" {
		t.Fatalf("table dynamics final summary: %#v", result.Final)
	}
}

func TestTableValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		node Node
	}{
		{name: "wrong_length", node: tableNode("A", "1", "2")},
		{name: "negative_value", node: tableNode("A", "1", "-1", "2")},
		{name: "decreasing_value", node: tableNode("A", "1", "3", "2")},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			model := Game{Players: 3, Nodes: []Node{test.node}}
			if err := model.Validate(); err == nil {
				t.Fatalf("invalid table %q was accepted", test.name)
			}
		})
	}
}

func TestDecimalExactRendering(t *testing.T) {
	t.Parallel()
	value := MustDecimal("0.5").Add(MustDecimal("0.25"))
	if value.String() != "0.75" {
		t.Fatalf("0.5+0.25 = %s", value)
	}
	third, err := MustDecimal("1").Div(MustDecimal("3"))
	if err != nil || third.String() != "1/3" {
		t.Fatalf("one third = %v, %v", third, err)
	}
	if _, err := ParseDecimal("NaN"); err == nil {
		t.Fatal("NaN was accepted")
	}
	encoded, err := json.Marshal(third)
	if err != nil {
		t.Fatal(err)
	}
	var restored Decimal
	if err := json.Unmarshal(encoded, &restored); err != nil || restored.String() != "1/3" {
		t.Fatalf("fraction did not round-trip: %s, %v", restored, err)
	}
	if _, err := ParseDecimal(strings.Repeat("9", maxExactLiteralLength+1)); err == nil {
		t.Fatal("overlong exact value was accepted")
	}
}

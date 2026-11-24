package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"nash-load-lab/backend/internal/game"
)

func TestPresentationReturnsPreparedEngineEvidenceWithoutStore(t *testing.T) {
	t.Parallel()

	// The deck must remain read-only.  A nil store proves that obtaining the
	// prepared slides neither needs nor attempts SQLite persistence.
	response := performJSON(t, New(nil), http.MethodGet, "/api/v1/presentation", "")
	if response.Code != http.StatusOK {
		t.Fatalf("presentation status = %d: %s", response.Code, response.Body.String())
	}

	var keys map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &keys); err != nil {
		t.Fatalf("decode presentation keys: %v", err)
	}
	for _, key := range []string{
		"linear_preview", "equilibrium_comparison", "potential_trace",
		"scenarios", "complex_scenario", "series", "scaling_setup", "scaling_series",
	} {
		if keys[key] == nil {
			t.Fatalf("presentation response misses key %q: %s", key, response.Body.String())
		}
	}

	var presentation presentationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &presentation); err != nil {
		t.Fatalf("decode presentation: %v", err)
	}
	assertLegacyPresentationContract(t, presentation)
	assertPreparedPresetScenarios(t, presentation.Scenarios)
	assertPreparedComplexScenario(t, presentation.ComplexScenario)
	assertPreparedSeries(t, presentation.Series)
	assertPreparedScalingSeries(t, presentation.ScalingSetup, presentation.ScalingSeries)

	// The non-terminating ratio must stay an exact JSON string, not a floating
	// point number rounded by a client or the HTTP boundary.
	var complexRaw map[string]json.RawMessage
	if err := json.Unmarshal(keys["complex_scenario"], &complexRaw); err != nil {
		t.Fatalf("decode complex scenario raw JSON: %v", err)
	}
	if got := string(complexRaw["k_run"]); got != `"152/143"` {
		t.Fatalf("complex k_run JSON = %s, want exact string 152/143", got)
	}
}

func TestPresentationHandlerServesItsPreparedSnapshot(t *testing.T) {
	t.Parallel()

	// Do not construct this server through New: the sentinel proves that the
	// handler serializes an already prepared response instead of calling
	// calculatePresentation (and therefore Simulate/Optimize) for this GET.
	server := &Server{
		version: game.Version,
		presentation: presentationResponse{
			LinearPreview: []presentationLinearPoint{{
				Load: 99,
				A:    game.MustDecimal("101"),
				B:    game.MustDecimal("202"),
			}},
		},
	}

	response := performJSON(t, server, http.MethodGet, "/api/v1/presentation", "")
	if response.Code != http.StatusOK {
		t.Fatalf("presentation status = %d: %s", response.Code, response.Body.String())
	}
	var payload presentationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode cached presentation: %v", err)
	}
	if len(payload.LinearPreview) != 1 || payload.LinearPreview[0].Load != 99 ||
		payload.LinearPreview[0].A.String() != "101" || payload.LinearPreview[0].B.String() != "202" {
		t.Fatalf("handler did not return the prepared snapshot: %+v", payload.LinearPreview)
	}
}

func assertLegacyPresentationContract(t *testing.T, presentation presentationResponse) {
	t.Helper()
	wantPreview := [][2]string{{"1", "2"}, {"2", "4"}, {"3", "6"}, {"4", "8"}}
	if len(presentation.LinearPreview) != len(wantPreview) {
		t.Fatalf("linear preview length = %d, want %d", len(presentation.LinearPreview), len(wantPreview))
	}
	for index, want := range wantPreview {
		got := presentation.LinearPreview[index]
		if got.Load != index+1 || got.A.String() != want[0] || got.B.String() != want[1] {
			t.Fatalf("linear preview[%d] = %+v, want load=%d a=%s b=%s", index, got, index+1, want[0], want[1])
		}
	}

	wantEquilibria := []struct {
		profile string
		total   string
	}{
		{profile: "(A,A)", total: "4"},
		{profile: "(A,B)", total: "3"},
	}
	if len(presentation.EquilibriumComparison) != len(wantEquilibria) {
		t.Fatalf("equilibrium comparison length = %d, want %d", len(presentation.EquilibriumComparison), len(wantEquilibria))
	}
	for index, want := range wantEquilibria {
		got := presentation.EquilibriumComparison[index]
		if got.Profile != want.profile || got.TotalTime.String() != want.total {
			t.Fatalf("equilibrium comparison[%d] = %+v, want profile=%s total_time=%s", index, got, want.profile, want.total)
		}
	}

	wantTrace := []struct {
		step      int
		potential string
	}{
		{step: 0, potential: "10"},
		{step: 1, potential: "7.5"},
		{step: 2, potential: "6.5"},
	}
	if len(presentation.PotentialTrace) != len(wantTrace) {
		t.Fatalf("potential trace length = %d, want %d", len(presentation.PotentialTrace), len(wantTrace))
	}
	for index, want := range wantTrace {
		got := presentation.PotentialTrace[index]
		if got.Step != want.step || got.Potential.String() != want.potential {
			t.Fatalf("potential trace[%d] = %+v, want step=%d potential=%s", index, got, want.step, want.potential)
		}
	}
}

func assertPreparedPresetScenarios(t *testing.T, scenarios []presentationScenario) {
	t.Helper()
	want := []struct {
		id          string
		steps       int
		finalTime   string
		optimumTime string
		kRun        string
		poa         string
	}{
		{id: "A", steps: 1, finalTime: "3", optimumTime: "3", kRun: "1", poa: "4/3"},
		{id: "B", steps: 2, finalTime: "8", optimumTime: "8", kRun: "1", poa: "1"},
		{id: "C", steps: 0, finalTime: "4", optimumTime: "3", kRun: "4/3", poa: "4/3"},
	}
	if len(scenarios) != len(want) {
		t.Fatalf("preset scenarios = %d, want %d", len(scenarios), len(want))
	}
	for index, expected := range want {
		scenario := scenarios[index]
		if scenario.ID != expected.id || scenario.Players < 1 || len(scenario.Nodes) < 1 {
			t.Fatalf("scenario[%d] identity/model = %+v", index, scenario)
		}
		if len(scenario.Trace) != expected.steps || scenario.Final.TotalTime.String() != expected.finalTime ||
			scenario.Optimum.TotalTime.String() != expected.optimumTime || scenario.KRun.String() != expected.kRun || scenario.PoA.String() != expected.poa {
			t.Fatalf("scenario %s has unexpected prepared values: %+v", expected.id, scenario)
		}
		assertExactScenarioEvidence(t, scenario)
	}
}

func assertPreparedComplexScenario(t *testing.T, scenario presentationScenario) {
	t.Helper()
	if scenario.ID != "сложный" || scenario.Players != 8 || len(scenario.Nodes) != 4 {
		t.Fatalf("complex scenario model = %+v", scenario)
	}
	if len(scenario.Trace) != 14 || scenario.Initial.TotalTime.String() != "64" || scenario.Final.TotalTime.String() != "12.16" ||
		scenario.Optimum.TotalTime.String() != "11.44" || scenario.KRun.String() != "152/143" || scenario.PoA.String() != "152/143" {
		t.Fatalf("complex scenario has unexpected prepared values: %+v", scenario)
	}
	kinds := map[game.CostFunctionKind]bool{}
	for _, node := range scenario.Nodes {
		kinds[node.Kind] = true
	}
	for _, kind := range []game.CostFunctionKind{game.CostLinear, game.CostQuadratic, game.CostTable} {
		if !kinds[kind] {
			t.Fatalf("complex scenario does not include node kind %q", kind)
		}
	}
	assertExactScenarioEvidence(t, scenario)
}

func assertExactScenarioEvidence(t *testing.T, scenario presentationScenario) {
	t.Helper()
	if !scenario.Nash.IsEquilibrium || scenario.Nash.CheckedMoves == 0 {
		t.Fatalf("scenario %s has no independent Nash proof: %+v", scenario.ID, scenario.Nash)
	}
	if scenario.Optimum.TotalTime.IsZero() || scenario.KRun.IsZero() || scenario.PoA.IsZero() {
		t.Fatalf("scenario %s has an undefined exact comparison", scenario.ID)
	}
	previousPotential := scenario.Initial.Potential
	for _, step := range scenario.Trace {
		if !step.Delta.IsNegative() {
			t.Fatalf("scenario %s step %d delta = %s, want strict improvement", scenario.ID, step.Step, step.Delta.String())
		}
		if step.Potential.Cmp(previousPotential) >= 0 {
			t.Fatalf("scenario %s step %d potential = %s, previous = %s", scenario.ID, step.Step, step.Potential.String(), previousPotential.String())
		}
		previousPotential = step.Potential
	}
	if scenario.Final.Potential.Cmp(previousPotential) != 0 {
		t.Fatalf("scenario %s final potential = %s, trace ends at %s", scenario.ID, scenario.Final.Potential.String(), previousPotential.String())
	}
}

func assertPreparedSeries(t *testing.T, points []presentationSeriesPoint) {
	t.Helper()
	want := []struct {
		players int
		nodes   int
		steps   int
		kRun    string
		ne      string
		optimum string
	}{
		{players: 4, nodes: 2, steps: 2, kRun: "1", ne: "8", optimum: "8"},
		{players: 6, nodes: 2, steps: 3, kRun: "1", ne: "17.4", optimum: "17.4"},
		{players: 6, nodes: 3, steps: 7, kRun: "259/241", ne: "10.36", optimum: "9.64"},
		{players: 8, nodes: 3, steps: 9, kRun: "1", ne: "17.56", optimum: "17.56"},
		{players: 8, nodes: 4, steps: 14, kRun: "152/143", ne: "12.16", optimum: "11.44"},
		{players: 10, nodes: 4, steps: 16, kRun: "1", ne: "17.96", optimum: "17.96"},
	}
	if len(points) != len(want) {
		t.Fatalf("presentation series points = %d, want %d", len(points), len(want))
	}
	for index, expected := range want {
		point := points[index]
		if point.Players != expected.players || point.Nodes != expected.nodes || point.Steps != expected.steps ||
			point.KRun.String() != expected.kRun || point.TotalTimeNE.String() != expected.ne || point.TotalTimeOpt.String() != expected.optimum {
			t.Fatalf("series[%d] = %+v, want %+v", index, point, expected)
		}
		if point.TotalTimeNE.Cmp(point.TotalTimeOpt) < 0 {
			t.Fatalf("series[%d] has S_NE < S_opt", index)
		}
	}
}

func assertPreparedScalingSeries(t *testing.T, setup presentationScalingSetup, points []presentationScalingPoint) {
	t.Helper()
	if setup.InitialNodeID != "У₁" || setup.Algorithm != game.FirstImprovement {
		t.Fatalf("scaling setup start/policy = %+v, want all tasks on У₁ with first_improvement", setup)
	}
	if len(setup.Nodes) != 4 {
		t.Fatalf("scaling setup nodes = %d, want 4", len(setup.Nodes))
	}
	wantNodes := []struct {
		kind game.CostFunctionKind
		a    string
		b    string
	}{
		{kind: game.CostLinear, a: "0", b: "1"},
		{kind: game.CostLinear, a: "0.4", b: "0.8"},
		{kind: game.CostQuadratic, a: "0.1", b: "0.18"},
		{kind: game.CostQuadratic, a: "0.2", b: "0.12"},
	}
	for index, expected := range wantNodes {
		node := setup.Nodes[index]
		if node.Kind != expected.kind || node.A.String() != expected.a || node.B.String() != expected.b || len(node.TableValues) != 0 {
			t.Fatalf("scaling node[%d] = %+v, want kind=%s a=%s b=%s and no table", index, node, expected.kind, expected.a, expected.b)
		}
	}

	want := []struct {
		n           int
		h           int
		equilibrium string
		optimum     string
		kRun        string
	}{
		{n: 8, h: 15, equilibrium: "11.2", optimum: "10.48", kRun: "140/131"},
		{n: 16, h: 25, equilibrium: "51.32", optimum: "50.8", kRun: "1283/1270"},
		{n: 32, h: 43, equilibrium: "250.52", optimum: "247.04", kRun: "6263/6176"},
	}
	if len(points) != len(want) {
		t.Fatalf("scaling points = %d, want %d", len(points), len(want))
	}
	for index, expected := range want {
		point := points[index]
		if point.N != expected.n || point.M != 4 || point.H != expected.h {
			t.Fatalf("scaling point[%d] dimensions/steps = %+v, want n=%d m=4 h=%d", index, point, expected.n, expected.h)
		}
		if point.H >= presentationScalingStepLimit {
			t.Fatalf("scaling point n=%d reached the step limit (%d), so H cannot be presented as a completed run", point.N, presentationScalingStepLimit)
		}
		if point.EquilibriumStatus != game.SimulationEquilibrium || point.SEquilibrium == nil {
			t.Fatalf("scaling point n=%d is not a proven equilibrium: %+v", point.N, point)
		}
		if point.ExactStatus != game.CalculationExact || point.SOptimum == nil || point.KRun == nil {
			t.Fatalf("scaling point n=%d has no exact comparison: %+v", point.N, point)
		}
		if point.SEquilibrium.String() != expected.equilibrium || point.SOptimum.String() != expected.optimum || point.KRun.String() != expected.kRun {
			t.Fatalf("scaling point n=%d values = %+v, want S_eq=%s S_opt=%s K=%s", point.N, point, expected.equilibrium, expected.optimum, expected.kRun)
		}
		if point.SEquilibrium.Cmp(*point.SOptimum) < 0 {
			t.Fatalf("scaling point n=%d has S_eq < S_opt: %+v", point.N, point)
		}
	}
}

func TestPresentationRejectsNonGET(t *testing.T) {
	t.Parallel()

	response := performJSON(t, New(nil), http.MethodPost, "/api/v1/presentation", "{}")
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("presentation POST status = %d: %s", response.Code, response.Body.String())
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want %q", allow, http.MethodGet)
	}
}

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nash-load-lab/backend/internal/game"
	"nash-load-lab/backend/internal/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	database, err := store.Open(context.Background(), store.Config{Path: ":memory:", BusyTimeout: time.Second})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return New(database)
}

func performJSON(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestSimulationRoundTripAndPresets(t *testing.T) {
	t.Parallel()
	server := newTestServer(t)

	presets := performJSON(t, server, http.MethodGet, "/api/v1/presets", "")
	if presets.Code != http.StatusOK {
		t.Fatalf("presets status = %d: %s", presets.Code, presets.Body.String())
	}
	var presetBody presetsResponse
	if err := json.Unmarshal(presets.Body.Bytes(), &presetBody); err != nil {
		t.Fatal(err)
	}
	if len(presetBody.Presets) != 3 {
		t.Fatalf("presets = %d, want 3", len(presetBody.Presets))
	}

	body := `{
      "players":2,
      "nodes":[
        {"id":"A","name":"Узел A","a":"0","b":"1","kind":"linear"},
        {"id":"B","name":"Узел B","a":"0","b":"2","kind":"linear"}
      ],
      "initial_assignment":["B","B"],
      "algorithm":"first_improvement",
      "max_steps":10
    }`
	created := performJSON(t, server, http.MethodPost, "/api/v1/simulations", body)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	var simulation SimulationResponse
	if err := json.Unmarshal(created.Body.Bytes(), &simulation); err != nil {
		t.Fatal(err)
	}
	if simulation.ID == "" || simulation.Initial.Potential.String() != "6" || simulation.Final.TotalTime.String() != "3" {
		t.Fatalf("unexpected simulation: %#v", simulation)
	}
	loaded := performJSON(t, server, http.MethodGet, "/api/v1/simulations/"+simulation.ID, "")
	if loaded.Code != http.StatusOK {
		t.Fatalf("load status = %d: %s", loaded.Code, loaded.Body.String())
	}
	var restored SimulationResponse
	if err := json.Unmarshal(loaded.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ID != simulation.ID || len(restored.Trace) != 1 || !restored.NashCheck.IsEquilibrium {
		t.Fatalf("round-trip changed result: %#v", restored)
	}
}

func TestPreviewAndOptimizationPublicShape(t *testing.T) {
	t.Parallel()
	server := newTestServer(t)
	nodes := `[{"id":"A","name":"A","a":"0","b":"1","kind":"linear"},{"id":"B","name":"B","a":"0","b":"2","kind":"linear"}]`
	preview := performJSON(t, server, http.MethodPost, "/api/v1/preview", `{"players":2,"nodes":`+nodes+`}`)
	if preview.Code != http.StatusOK {
		t.Fatalf("preview status = %d: %s", preview.Code, preview.Body.String())
	}
	var previewBody previewResponse
	if err := json.Unmarshal(preview.Body.Bytes(), &previewBody); err != nil {
		t.Fatal(err)
	}
	if len(previewBody.Rows) != 2 || previewBody.Rows[1].Values[1].Value.String() != "4" {
		t.Fatalf("unexpected preview: %#v", previewBody)
	}

	optimized := performJSON(t, server, http.MethodPost, "/api/v1/optimize", `{
      "players":2,"nodes":`+nodes+`,"initial_assignment":["B","B"],
      "algorithm":"first_improvement","max_steps":10,"max_states":100
    }`)
	if optimized.Code != http.StatusOK {
		t.Fatalf("optimize status = %d: %s", optimized.Code, optimized.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(optimized.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	optimum := response["optimum"].(map[string]any)
	if optimum["status"] != "exact" || optimum["total_time"] != "3" {
		t.Fatalf("unexpected optimum response: %#v", optimum)
	}
	poa := response["poa"].(map[string]any)
	if poa["value"] != "4/3" {
		t.Fatalf("unexpected PoA response: %#v", poa)
	}
	if response["k_run"] != "1" {
		t.Fatalf("K_run = %#v, want 1", response["k_run"])
	}
}

func TestTablePreviewSimulationAndValidation(t *testing.T) {
	t.Parallel()
	server := newTestServer(t)
	nodes := `[
      {"id":"A","name":"A","kind":"table","table_values":["1","2","5","9"]},
      {"id":"B","name":"B","kind":"table","table_values":["2","3","4","6"]}
    ]`

	preview := performJSON(t, server, http.MethodPost, "/api/v1/preview", `{"players":4,"nodes":`+nodes+`}`)
	if preview.Code != http.StatusOK {
		t.Fatalf("table preview status = %d: %s", preview.Code, preview.Body.String())
	}
	var previewBody previewResponse
	if err := json.Unmarshal(preview.Body.Bytes(), &previewBody); err != nil {
		t.Fatal(err)
	}
	if len(previewBody.Rows) != 4 || previewBody.Rows[2].Values[0].Value.String() != "5" || previewBody.Rows[3].Values[1].Value.String() != "6" {
		t.Fatalf("unexpected table preview: %#v", previewBody)
	}

	created := performJSON(t, server, http.MethodPost, "/api/v1/simulations", `{
      "players":4,"nodes":`+nodes+`,"initial_assignment":["A","A","B","B"],
      "algorithm":"first_improvement","max_steps":10
    }`)
	if created.Code != http.StatusCreated {
		t.Fatalf("table simulation status = %d: %s", created.Code, created.Body.String())
	}
	var simulation SimulationResponse
	if err := json.Unmarshal(created.Body.Bytes(), &simulation); err != nil {
		t.Fatal(err)
	}
	if simulation.Status != game.SimulationEquilibrium || !simulation.NashCheck.IsEquilibrium || simulation.Final.TotalTime.String() != "10" {
		t.Fatalf("unexpected table simulation: %#v", simulation)
	}
	loaded := performJSON(t, server, http.MethodGet, "/api/v1/simulations/"+simulation.ID, "")
	if loaded.Code != http.StatusOK {
		t.Fatalf("table simulation reload status = %d: %s", loaded.Code, loaded.Body.String())
	}
	var restored SimulationResponse
	if err := json.Unmarshal(loaded.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Request.Nodes) != 2 || restored.Request.Nodes[0].Kind != game.CostTable || len(restored.Request.Nodes[0].TableValues) != 4 || restored.Request.Nodes[0].TableValues[2].String() != "5" || restored.Request.Nodes[1].TableValues[3].String() != "6" {
		t.Fatalf("table model did not survive the simulation round-trip: %#v", restored.Request.Nodes)
	}

	invalidTables := []string{
		`["1","2","5"]`,
		`["1","3","2","4"]`,
		`["1","2","-1","4"]`,
	}
	for _, values := range invalidTables {
		response := performJSON(t, server, http.MethodPost, "/api/v1/preview", `{
          "players":4,
          "nodes":[{"id":"A","name":"A","kind":"table","table_values":`+values+`}]
        }`)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid table %s status = %d: %s", values, response.Code, response.Body.String())
		}
	}
}

func TestOptimizeDoesNotExposeStepLimitedRunAsEquilibrium(t *testing.T) {
	t.Parallel()
	server := newTestServer(t)
	nodes := `[{"id":"A","name":"A","a":"0","b":"1","kind":"linear"},{"id":"B","name":"B","a":"0","b":"2","kind":"linear"}]`
	optimized := performJSON(t, server, http.MethodPost, "/api/v1/optimize", `{
      "players":2,"nodes":`+nodes+`,"initial_assignment":["B","B"],
      "algorithm":"first_improvement","max_steps":1,"max_states":100
    }`)
	if optimized.Code != http.StatusOK {
		t.Fatalf("optimize status = %d: %s", optimized.Code, optimized.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(optimized.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if _, ok := response["equilibrium"]; ok {
		t.Fatalf("step-limited run must not be published as an equilibrium: %#v", response)
	}
	if _, ok := response["k_run"]; ok {
		t.Fatalf("step-limited run must not receive K_run: %#v", response)
	}
}

func TestStepLimitedSimulationKeepsItsTerminalStatusAfterReload(t *testing.T) {
	t.Parallel()
	server := newTestServer(t)
	created := performJSON(t, server, http.MethodPost, "/api/v1/simulations", `{
      "players":2,
      "nodes":[
        {"id":"A","name":"A","a":"0","b":"1","kind":"linear"},
        {"id":"B","name":"B","a":"0","b":"2","kind":"linear"}
      ],
      "initial_assignment":["B","B"],
      "algorithm":"first_improvement",
      "max_steps":1
    }`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	var response SimulationResponse
	if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "step_limit" || !response.NashCheck.IsEquilibrium {
		t.Fatalf("unexpected terminal state: %#v", response)
	}
	loaded := performJSON(t, server, http.MethodGet, "/api/v1/simulations/"+response.ID, "")
	if loaded.Code != http.StatusOK {
		t.Fatalf("load status = %d: %s", loaded.Code, loaded.Body.String())
	}
	var restored SimulationResponse
	if err := json.Unmarshal(loaded.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Status != "step_limit" {
		t.Fatalf("restored status = %q, want step_limit", restored.Status)
	}
}

func TestPublicExactInputLimitAndWideResultRoundTrip(t *testing.T) {
	t.Parallel()
	server := newTestServer(t)
	coefficient := strings.Repeat("9", maxInputExactCharacters)
	body := fmt.Sprintf(`{
      "players":2,
      "nodes":[{"id":"A","name":"A","a":%q,"b":"1","kind":"linear"}],
      "initial_assignment":["A","A"],
      "algorithm":"first_improvement",
      "max_steps":1
    }`, coefficient)
	created := performJSON(t, server, http.MethodPost, "/api/v1/simulations", body)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	var response SimulationResponse
	if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	loaded := performJSON(t, server, http.MethodGet, "/api/v1/simulations/"+response.ID, "")
	if loaded.Code != http.StatusOK {
		t.Fatalf("load status = %d: %s", loaded.Code, loaded.Body.String())
	}
	var restored SimulationResponse
	if err := json.Unmarshal(loaded.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Final.TotalTime.String()) <= maxInputExactCharacters {
		t.Fatalf("derived exact value was not wider than input: %s", restored.Final.TotalTime)
	}

	overlong := strings.Repeat("9", maxInputExactCharacters+1)
	invalid := performJSON(t, server, http.MethodPost, "/api/v1/preview", fmt.Sprintf(`{
      "players":1,
      "nodes":[{"id":"A","name":"A","a":%q,"b":"1","kind":"linear"}]
    }`, overlong))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("overlong coefficient status = %d: %s", invalid.Code, invalid.Body.String())
	}
	fraction := performJSON(t, server, http.MethodPost, "/api/v1/preview", `{
      "players":1,
      "nodes":[{"id":"A","name":"A","a":"1/3","b":"1","kind":"linear"}]
    }`)
	if fraction.Code != http.StatusBadRequest {
		t.Fatalf("fraction coefficient status = %d: %s", fraction.Code, fraction.Body.String())
	}
}

func TestExperimentTraceBudgetIsValidatedBeforeExecution(t *testing.T) {
	t.Parallel()
	request := ExperimentRequest{
		Players:   NumberRange{From: 499, To: 500, Step: 1},
		Nodes:     NumberRange{From: 40, To: 40, Step: 1},
		A:         DecimalRange{From: game.MustDecimal("0"), To: game.MustDecimal("0"), Step: game.MustDecimal("1")},
		B:         DecimalRange{From: game.MustDecimal("1"), To: game.MustDecimal("1"), Step: game.MustDecimal("1")},
		Algorithm: game.FirstImprovement,
		Seed:      1,
		MaxSteps:  500,
	}
	if err := validateExperimentRequest(request); err == nil {
		t.Fatal("oversized experiment trace budget was accepted")
	}
}

func TestExperimentRunLimitAllowsTenThousandRuns(t *testing.T) {
	t.Parallel()
	request := ExperimentRequest{
		Players:   NumberRange{From: 1, To: 10, Step: 1},
		Nodes:     NumberRange{From: 1, To: 10, Step: 1},
		A:         DecimalRange{From: game.MustDecimal("0"), To: game.MustDecimal("9"), Step: game.MustDecimal("1")},
		B:         DecimalRange{From: game.MustDecimal("1"), To: game.MustDecimal("10"), Step: game.MustDecimal("1")},
		Algorithm: game.FirstImprovement,
		Seed:      1,
		MaxSteps:  1,
	}
	if err := validateExperimentRequest(request); err != nil {
		t.Fatalf("exactly %d experiment runs were rejected: %v", maxExperimentRuns, err)
	}
}

func TestExperimentRunLimitRejectsMoreThanTenThousandRuns(t *testing.T) {
	t.Parallel()
	request := ExperimentRequest{
		Players:   NumberRange{From: 1, To: 11, Step: 1},
		Nodes:     NumberRange{From: 1, To: 10, Step: 1},
		A:         DecimalRange{From: game.MustDecimal("0"), To: game.MustDecimal("9"), Step: game.MustDecimal("1")},
		B:         DecimalRange{From: game.MustDecimal("1"), To: game.MustDecimal("10"), Step: game.MustDecimal("1")},
		Algorithm: game.FirstImprovement,
		Seed:      1,
		MaxSteps:  1,
	}
	err := validateExperimentRequest(request)
	if err == nil || !strings.Contains(err.Error(), "10000") {
		t.Fatalf("more than %d experiment runs were accepted or reported unclearly: %v", maxExperimentRuns, err)
	}
}

func TestExperimentLifecycleAndExport(t *testing.T) {
	t.Parallel()
	server := newTestServer(t)
	created := performJSON(t, server, http.MethodPost, "/api/v1/experiments", `{
      "players":{"from":2,"to":2,"step":1},
      "nodes":{"from":2,"to":2,"step":1},
      "a":{"from":"0","to":"0","step":"1"},
      "b":{"from":"1","to":"1","step":"1"},
      "algorithm":"first_improvement","seed":7,"max_steps":10
    }`)
	if created.Code != http.StatusCreated {
		t.Fatalf("experiment create = %d: %s", created.Code, created.Body.String())
	}
	var experiment experimentSummaryResponse
	if err := json.Unmarshal(created.Body.Bytes(), &experiment); err != nil {
		t.Fatal(err)
	}
	if experiment.Status != "complete" || len(experiment.Points) != 1 || experiment.Points[0].Status != "exact" {
		t.Fatalf("unexpected experiment: %#v", experiment)
	}

	export := performJSON(t, server, http.MethodGet, "/api/v1/experiments/"+experiment.ID+"/export?format=json", "")
	if export.Code != http.StatusOK || !bytes.Contains(export.Body.Bytes(), []byte(`"format_version"`)) {
		t.Fatalf("export = %d: %s", export.Code, export.Body.String())
	}
	deleted := performJSON(t, server, http.MethodDelete, "/api/v1/experiments/"+experiment.ID, "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete = %d: %s", deleted.Code, deleted.Body.String())
	}
}

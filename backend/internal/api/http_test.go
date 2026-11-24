package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"known":"ok","extra":1}`))
	var payload struct {
		Known string `json:"known"`
	}

	if decodeJSON(recorder, request, &payload) {
		t.Fatal("decodeJSON accepted an unknown field")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestDecodeJSONAcceptsOneDocument(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"known":"ok"}`))
	var payload struct {
		Known string `json:"known"`
	}

	if !decodeJSON(recorder, request, &payload) {
		t.Fatalf("decodeJSON rejected valid data: %s", recorder.Body.String())
	}
	if payload.Known != "ok" {
		t.Fatalf("known = %q, want ok", payload.Known)
	}
}

func TestDecodeJSONRejectsMultipleDocuments(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{} {}`))
	var payload map[string]any

	if decodeJSON(recorder, request, &payload) {
		t.Fatal("decodeJSON accepted multiple documents")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

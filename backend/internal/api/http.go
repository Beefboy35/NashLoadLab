// Package api contains the HTTP boundary for Nash Load Lab.  It deliberately
// keeps JSON validation and error formatting here, leaving all game arithmetic
// in the game package.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const maxJSONBodyBytes int64 = 1 << 20 // 1 MiB is ample for bounded traces.

// ErrorResponse is the stable, Russian-language error envelope returned by
// every endpoint. Details is optional and is intended for field-level errors.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		// Headers and the status have already been sent. There is no safe HTTP
		// recovery at this point, and the connection will be closed by net/http.
		return
	}
}

func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, ErrorResponse{Error: code, Message: message, Details: details})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "empty_body", "Тело запроса не должно быть пустым.", nil)
			return false
		}
		var syntaxError *json.SyntaxError
		var typeError *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntaxError):
			writeError(w, http.StatusBadRequest, "invalid_json", "Некорректный JSON в запросе.", map[string]any{"offset": syntaxError.Offset})
		case errors.As(err, &typeError):
			writeError(w, http.StatusBadRequest, "invalid_field", "Некорректное значение поля запроса.", map[string]any{"field": typeError.Field})
		default:
			writeError(w, http.StatusBadRequest, "invalid_request", "Не удалось прочитать запрос.", map[string]any{"reason": err.Error()})
		}
		return false
	}

	// Exactly one JSON document prevents accidental concatenated requests.
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_json", "Запрос должен содержать один JSON-объект.", nil)
		return false
	}
	return true
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", fmt.Sprintf("Допустимый метод: %s.", allowed), nil)
}

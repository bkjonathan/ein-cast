package httpapi

import (
	"encoding/json"
	"net/http"
)

// errorBody is the one error shape every endpoint uses:
// {"error":{"code":"not_found","message":"..."}}.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error codes. Stable strings clients can switch on.
const (
	codeNotFound = "not_found"
	codeInternal = "internal"
)

// writeJSON marshals first, so a marshalling failure becomes a clean 500
// instead of a half-written 200.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "could not encode response")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	body, _ := json.Marshal(errorBody{Error: errorDetail{Code: code, Message: message}})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

package httpx

import (
	"encoding/json"
	"net/http"
)

type ErrorEnvelope struct {
	Error     ErrorBody `json:"error"`
	RequestID string    `json:"request_id"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details"`
}

func WriteJSON(w http.ResponseWriter, statusCode int, data any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(data)
}

func WriteError(w http.ResponseWriter, r *http.Request, statusCode int, code string, message string, details any) {
	reqID := GetRequestID(r.Context())
	if reqID == "" {
		reqID = r.Header.Get(HeaderRequestID)
	}

	if details == nil {
		details = []any{}
	}

	envelope := ErrorEnvelope{
		Error: ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
		RequestID: reqID,
	}

	_ = WriteJSON(w, statusCode, envelope)
}

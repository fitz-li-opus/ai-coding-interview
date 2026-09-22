package middleware

import (
	"encoding/json"
	"net/http"
)

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func WriteError(writer http.ResponseWriter, request *http.Request, status int, code, message string) {
	requestID := ""
	if state := State(request); state != nil {
		requestID = state.RequestID
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(errorEnvelope{Error: errorBody{
		Code:      code,
		Message:   message,
		RequestID: requestID,
	}}); err != nil {
		panic(err)
	}
}

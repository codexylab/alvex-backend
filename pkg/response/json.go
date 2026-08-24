package response

import (
	"encoding/json"
	"net/http"
)

// APIResponse is the standard envelope for all ALVEX API responses.
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

// JSON writes a JSON response with the given status code and payload.
func JSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

// Success writes a 200 OK response with data.
func Success(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusOK, APIResponse{Success: true, Data: data})
}

// Created writes a 201 Created response with data.
func Created(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusCreated, APIResponse{Success: true, Data: data})
}

// NoContent writes a 204 No Content response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Message writes a 200 OK response with only a message string.
func Message(w http.ResponseWriter, msg string) {
	JSON(w, http.StatusOK, APIResponse{Success: true, Message: msg})
}

// BadRequest writes a 400 error response.
func BadRequest(w http.ResponseWriter, err string) {
	JSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err})
}

// Unauthorized writes a 401 error response.
func Unauthorized(w http.ResponseWriter) {
	JSON(w, http.StatusUnauthorized, APIResponse{Success: false, Error: "Unauthorized — valid JWT token required"})
}

// Forbidden writes a 403 error response.
func Forbidden(w http.ResponseWriter) {
	JSON(w, http.StatusForbidden, APIResponse{Success: false, Error: "Forbidden — insufficient permissions"})
}

// NotFound writes a 404 error response.
func NotFound(w http.ResponseWriter, resource string) {
	JSON(w, http.StatusNotFound, APIResponse{Success: false, Error: resource + " not found"})
}

// Conflict writes a 409 error response.
func Conflict(w http.ResponseWriter, err string) {
	JSON(w, http.StatusConflict, APIResponse{Success: false, Error: err})
}

// InternalError writes a 500 error response.
func InternalError(w http.ResponseWriter) {
	JSON(w, http.StatusInternalServerError, APIResponse{Success: false, Error: "Internal server error"})
}

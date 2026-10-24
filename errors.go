package main

// Error de la API: se serializa como `{ "error": "mensaje" }`, que es lo que espera el front
// (`mensajeDeError` en `src/services/api.js` lee `response.data.error`).

import (
	"errors"
	"log"
	"net/http"
)

type apiError struct {
	status  int
	message string
}

func (e *apiError) Error() string { return e.message }

func badRequest(msg string) error    { return &apiError{http.StatusBadRequest, msg} }
func unauthorized(msg string) error  { return &apiError{http.StatusUnauthorized, msg} }
func forbidden(msg string) error     { return &apiError{http.StatusForbidden, msg} }
func notFound(msg string) error      { return &apiError{http.StatusNotFound, msg} }
func conflict(msg string) error      { return &apiError{http.StatusConflict, msg} }
func internalError(msg string) error { return &apiError{http.StatusInternalServerError, msg} }

func dbError(err error) error {
	return internalError("Error de base de datos: " + err.Error())
}

func serializationError(err error) error {
	return internalError("Error de serialización: " + err.Error())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := marshalJSON(v)
	if err != nil {
		writeError(w, serializationError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}

func writeError(w http.ResponseWriter, err error) {
	var ae *apiError
	if !errors.As(err, &ae) {
		ae = &apiError{http.StatusInternalServerError, err.Error()}
	}
	if ae.status >= 500 {
		log.Printf("ERROR %s", ae.message)
	}
	writeJSON(w, ae.status, M{"error": ae.message})
}

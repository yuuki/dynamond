package web

import (
	"encoding/json"
	"net/http"
)

func renderJSON(w http.ResponseWriter, status int, v interface{}) {
	res, err := json.Marshal(v)
	if err != nil {
		serverError(w, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(res)
}

func ok(w http.ResponseWriter, msg string) {
	var data struct {
		Msg string `json:"message"`
	}
	data.Msg = msg
	renderJSON(w, http.StatusOK, data)
	return
}

func badRequest(w http.ResponseWriter, msg string) {
	var data struct {
		Error string `json:"error"`
	}
	data.Error = msg
	renderJSON(w, http.StatusBadRequest, data)
	return
}

func notFound(w http.ResponseWriter) {
	http.Error(w, "404 Not Found", http.StatusNotFound)
}

func serverError(w http.ResponseWriter, msg string) {
	var data struct {
		Error string `json:"error"`
	}
	data.Error = msg
	renderJSON(w, http.StatusInternalServerError, data)
	return
}

func unavaliableError(w http.ResponseWriter, msg string) {
	var data struct {
		Error string `json:"error"`
	}
	data.Error = msg
	renderJSON(w, http.StatusServiceUnavailable, data)
	return
}
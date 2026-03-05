package http

import (
	"net/http"
)

func MapRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("GET /health", h.HealthCheck)
}

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/volantvm/volant/internal/drift/controller"
	"github.com/volantvm/volant/internal/drift/routes"
)

// Handler wires HTTP endpoints for Drift route management.
type Handler struct {
	controller *controller.Controller
}

// New constructs a router backed by the provided Controller.
func New(ctrl *controller.Controller) http.Handler {
	h := &Handler{controller: ctrl}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", h.handleHealth)
	r.Group(func(r chi.Router) {
		r.Get("/routes", h.handleListRoutes)
		r.Post("/routes", h.handleUpsertRoute)
		r.Delete("/routes/{protocol}/{port}", h.handleDeleteRoute)
	})

	return r
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) handleListRoutes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := h.controller.List(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleUpsertRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var route routes.Route
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	updated, err := h.controller.Upsert(ctx, route)
	if err != nil {
		status := http.StatusInternalServerError
		var validationErr controller.ValidationError
		if errors.As(err, &validationErr) {
			status = http.StatusBadRequest
		} else {
			var unavailable controller.RuntimeUnavailableError
			if errors.As(err, &unavailable) {
				status = http.StatusServiceUnavailable
			}
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, updated)
}

func (h *Handler) handleDeleteRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	protocol := chi.URLParam(r, "protocol")
	portStr := chi.URLParam(r, "port")
	if protocol == "" || portStr == "" {
		writeError(w, http.StatusBadRequest, "missing protocol or port")
		return
	}
	port64, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid port")
		return
	}
	if err := h.controller.Delete(ctx, uint16(port64), protocol); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, routes.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

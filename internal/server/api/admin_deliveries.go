package api

import (
	"net/http"
	"strconv"
)

func (s *Server) handleListDeliveries(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	deliveries, err := s.Store.ListDeliveries(r.Context(), limit)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deliveries)
}

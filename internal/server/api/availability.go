package api

import (
	"net/http"
	"net/url"
	"time"
)

func (s *Server) handleAvailability(w http.ResponseWriter, r *http.Request, admin bool) {
	v, ok := s.lookup(w, r, admin)
	if !ok {
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_range")
		return
	}
	rng := "7d"
	for key, values := range q {
		if key != "range" || len(values) != 1 || (values[0] != "7d" && values[0] != "30d" && values[0] != "90d") {
			writeError(w, http.StatusBadRequest, "invalid_range")
			return
		}
		rng = values[0]
	}
	now := time.Now()
	summary, queryErr := s.Store.QueryAvailability(r.Context(), v.Node.ID, now.Add(-ranges[rng]).Unix(), now.Unix())
	if queryErr != nil {
		s.Log.Error("query availability", "err", queryErr)
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

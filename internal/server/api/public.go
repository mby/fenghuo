package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mby/fenghuo/internal/proto"
	"github.com/mby/fenghuo/internal/server/auth"
	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/store"
	"github.com/mby/fenghuo/internal/version"
)

func (s *Server) mountPublic(mux *http.ServeMux, adminMounted bool) {
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, r *http.Request) {
		st := s.Store.Settings()
		ver := ""
		if st.PublicSystem || (adminMounted && s.sessionOK(r)) {
			ver = version.Version
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"site_name":        st.SiteName,
			"version":          ver,
			"public_enabled":   st.PublicEnabled,
			"admin":            adminMounted,
			"keep_minute_days": int(s.KeepMinute.Hours() / 24),
			"keep_hour_days":   int(s.KeepHour.Hours() / 24),
		})
	})
	mux.HandleFunc("GET /api/v1/nodes", s.viewer(adminMounted, s.handleNodes))
	mux.HandleFunc("GET /api/v1/nodes/{uuid}/metrics", s.viewer(adminMounted, s.handleMetrics))
	mux.HandleFunc("GET /api/v1/nodes/{uuid}/ping", s.viewer(adminMounted, s.handlePing))
	mux.HandleFunc("GET /api/v1/nodes/{uuid}/availability", s.viewer(adminMounted, s.handleAvailability))
}

type viewerFunc func(w http.ResponseWriter, r *http.Request, admin bool)

// viewer resolves who is looking. Admins (only where the admin API is
// mounted) also see hidden nodes and can view even when the public page is off.
func (s *Server) viewer(adminMounted bool, h viewerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := adminMounted && s.sessionOK(r)
		if !admin && !s.Store.Settings().PublicEnabled {
			writeError(w, http.StatusNotFound, "public_disabled")
			return
		}
		s.Hub.MarkViewed(time.Now())
		h(w, r, admin)
	}
}

func (s *Server) handleNodes(w http.ResponseWriter, _ *http.Request, admin bool) {
	views := s.Hub.Snapshot(time.Now())
	out := make([]publicNode, 0, len(views))
	for _, v := range views {
		if v.Node.Hidden && !admin {
			continue
		}
		out = append(out, s.nodeForViewer(v, admin))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) lookup(w http.ResponseWriter, r *http.Request, admin bool) (hub.View, bool) {
	v, ok := s.Hub.ByUUID(r.PathValue("uuid"), time.Now())
	if !ok || (v.Node.Hidden && !admin) {
		writeError(w, http.StatusNotFound, "not_found")
		return v, false
	}
	return v, true
}

var ranges = map[string]time.Duration{
	"1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour,
	"7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour, "90d": 90 * 24 * time.Hour,
}

const maxPoints = 360

// plan picks the table and bucket size so that a range yields at most ~360 points.
func (s *Server) plan(d time.Duration) (store.Resolution, int64) {
	res, unit := store.Hour, int64(3600)
	if d <= s.KeepMinute {
		res, unit = store.Minute, 60
	}
	step := int64(d.Seconds()) / maxPoints
	step = max(unit, (step+unit-1)/unit*unit)
	return res, step
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request, admin bool) {
	v, ok := s.lookup(w, r, admin)
	if !ok {
		return
	}
	rng := r.URL.Query().Get("range")
	if rng == "" || rng == "live" {
		writeJSON(w, http.StatusOK, s.Hub.Live(v.Node.ID))
		return
	}
	d, ok := ranges[rng]
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_range")
		return
	}
	res, step := s.plan(d)
	points, err := s.Store.QueryMetrics(r.Context(), v.Node.ID, res, time.Now().Add(-d).Unix(), step)
	if err != nil {
		s.Log.Error("query metrics", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, points)
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request, admin bool) {
	if !admin && !s.Store.Settings().PublicPing {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	v, ok := s.lookup(w, r, admin)
	if !ok {
		return
	}
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = "24h"
	}
	d, ok := ranges[rng]
	if !ok || (d > s.KeepMinute && d > s.KeepHour) {
		writeError(w, http.StatusBadRequest, "invalid_range")
		return
	}
	_, step := s.plan(d)
	query := s.Store.QueryPing
	if d > s.KeepMinute {
		query = s.Store.QueryPingHours
	}
	series, err := query(r.Context(), v.Node.ID, time.Now().Add(-d).Unix(), step)
	if err != nil {
		s.Log.Error("query ping", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, series)
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, proto.MaxBodyBytes)
	var rep proto.Report
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rep); err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := rep.Validate(); err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid_report", err.Error())
		return
	}
	watched, err := s.Hub.Ingest(auth.HashToken(token), &rep, s.clientIP(r), time.Now())
	switch {
	case err == nil && watched:
		w.WriteHeader(proto.StatusWatched)
	case err == nil:
		w.WriteHeader(proto.StatusIdle)
	case errors.Is(err, hub.ErrUnknownToken):
		writeError(w, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, hub.ErrTooFrequent):
		writeError(w, http.StatusTooManyRequests, "too_frequent")
	default:
		s.Log.Error("ingest", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error")
	}
}

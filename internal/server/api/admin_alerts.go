package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mby/fenghuo/internal/server/notify"
	"github.com/mby/fenghuo/internal/server/store"
)

func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.Store.ListRules(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (s *Server) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	var rule store.Rule
	if !decodeJSON(w, r, &rule, smallBody) {
		return
	}
	if err := validateRule(&rule); err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid_rule", err.Error())
		return
	}
	if err := s.Store.CreateRule(r.Context(), &rule); err != nil {
		s.storeError(w, err)
		return
	}
	s.reloadAlerts(w, r, http.StatusCreated, rule)
}

func (s *Server) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var rule store.Rule
	if !decodeJSON(w, r, &rule, smallBody) {
		return
	}
	rule.ID = id
	if err := validateRule(&rule); err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid_rule", err.Error())
		return
	}
	if err := s.Store.UpdateRule(r.Context(), rule); err != nil {
		s.storeError(w, err)
		return
	}
	s.reloadAlerts(w, r, http.StatusOK, rule)
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteRule(r.Context(), id); err != nil {
		s.storeError(w, err)
		return
	}
	s.reloadAlerts(w, r, http.StatusNoContent, nil)
}

func (s *Server) reloadAlerts(w http.ResponseWriter, r *http.Request, status int, body any) {
	if err := s.Alerts.Reload(r.Context()); err != nil {
		s.storeError(w, err)
		return
	}
	if body == nil {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, body)
}

type channelInput struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Config  json.RawMessage `json:"config"`
	Enabled bool            `json:"enabled"`
}

func (in *channelInput) toChannel() (store.Channel, error) {
	in.Name = strings.TrimSpace(in.Name)
	if err := checkText("name", in.Name, 1, 64); err != nil {
		return store.Channel{}, err
	}
	cfg, err := notify.ValidateConfig(in.Type, in.Config)
	if err != nil {
		return store.Channel{}, err
	}
	return store.Channel{Name: in.Name, Type: in.Type, Config: cfg, Enabled: in.Enabled}, nil
}

func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	chans, err := s.Store.ListChannels(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, chans)
}

func (s *Server) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	var in channelInput
	if !decodeJSON(w, r, &in, 16<<10) {
		return
	}
	ch, err := in.toChannel()
	if err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid_channel", err.Error())
		return
	}
	if err := s.Store.CreateChannel(r.Context(), &ch); err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ch)
}

func (s *Server) handleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in channelInput
	if !decodeJSON(w, r, &in, 16<<10) {
		return
	}
	ch, err := in.toChannel()
	if err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid_channel", err.Error())
		return
	}
	ch.ID = id
	if err := s.Store.UpdateChannel(r.Context(), ch); err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ch)
}

func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteChannel(r.Context(), id); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	chans, err := s.Store.ListChannels(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	for _, ch := range chans {
		if ch.ID != id {
			continue
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		site := s.Store.Settings().SiteName
		err := s.Notify.Send(ctx, ch, notify.Message{
			Title: "[" + site + "] Test", Text: "This is a test notification from " + site + ".",
			Kind: "test", Firing: true, Time: time.Now().Unix(),
		})
		if err != nil {
			writeErrorMsg(w, http.StatusBadGateway, "send_failed", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeError(w, http.StatusNotFound, "not_found")
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	events, err := s.Store.ListEvents(r.Context(), limit)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/mby/fenghuo/internal/server/auth"
	"github.com/mby/fenghuo/internal/server/store"
)

const tokenPrefix = "fh_"

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return 0, false
	}
	return id, true
}

func (s *Server) storeError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	s.Log.Error("store", "err", err)
	writeError(w, http.StatusInternalServerError, "internal_error")
}

// reloadNodes pushes node configuration changes into the hub and alert engine.
func (s *Server) reloadNodes(r *http.Request) error {
	nodes, err := s.Store.ListNodes(r.Context())
	if err != nil {
		return err
	}
	s.Hub.Reload(nodes, nil, time.Now())
	return s.Alerts.Reload(r.Context())
}

func (s *Server) nodeByID(id int64) (adminNode, bool) {
	for _, v := range s.Hub.Snapshot(time.Now()) {
		if v.Node.ID == id {
			return toAdmin(v), true
		}
	}
	return adminNode{}, false
}

func (s *Server) handleListNodes(w http.ResponseWriter, _ *http.Request) {
	s.Hub.MarkViewed(time.Now())
	views := s.Hub.Snapshot(time.Now())
	out := make([]adminNode, 0, len(views))
	for _, v := range views {
		out = append(out, toAdmin(v))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	var in nodeInput
	if !decodeJSON(w, r, &in, smallBody) {
		return
	}
	if err := in.validate(); err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid_node", err.Error())
		return
	}
	token := auth.NewToken(tokenPrefix)
	n := store.Node{TokenHash: auth.HashToken(token)}
	in.apply(&n)
	if err := s.Store.CreateNode(r.Context(), &n); err != nil {
		s.storeError(w, err)
		return
	}
	if err := s.reloadNodes(r); err != nil {
		s.storeError(w, err)
		return
	}
	s.Log.Info("node created", "id", n.ID, "name", n.Name)
	node, _ := s.nodeByID(n.ID)
	// The token is only ever returned here and on rotation; the server keeps a hash.
	writeJSON(w, http.StatusCreated, map[string]any{"node": node, "token": token})
}

func (s *Server) handleUpdateNode(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in nodeInput
	if !decodeJSON(w, r, &in, smallBody) {
		return
	}
	if err := in.validate(); err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid_node", err.Error())
		return
	}
	n := store.Node{ID: id}
	in.apply(&n)
	if err := s.Store.UpdateNodeWithPublicTags(r.Context(), n, in.PublicTags); err != nil {
		s.storeError(w, err)
		return
	}
	if err := s.reloadNodes(r); err != nil {
		s.storeError(w, err)
		return
	}
	node, _ := s.nodeByID(id)
	writeJSON(w, http.StatusOK, node)
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteNode(r.Context(), id); err != nil {
		s.storeError(w, err)
		return
	}
	if err := s.reloadNodes(r); err != nil {
		s.storeError(w, err)
		return
	}
	s.Log.Info("node deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRotateToken(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	token := auth.NewToken(tokenPrefix)
	if err := s.Store.SetNodeToken(r.Context(), id, auth.HashToken(token)); err != nil {
		s.storeError(w, err)
		return
	}
	if err := s.reloadNodes(r); err != nil {
		s.storeError(w, err)
		return
	}
	s.Log.Info("node token rotated", "id", id)
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

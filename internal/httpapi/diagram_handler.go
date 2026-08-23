package httpapi

import (
	"net/http"

	"task187-phaseaudit/internal/phasediagram"
)

// handleCreateDiagram POST /api/diagrams — 创建相图版本。
func (s *Server) handleCreateDiagram(w http.ResponseWriter, r *http.Request) {
	var in phasediagram.CreateInput
	if err := decodeBody(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	d, err := s.app.Diagram.Create(in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

// handleListDiagrams GET /api/diagrams — 相图版本列表。
func (s *Server) handleListDiagrams(w http.ResponseWriter, r *http.Request) {
	ds, err := s.app.Diagram.List()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ds)
}

// handleGetDiagram GET /api/diagrams/{id} — 相图详情。
func (s *Server) handleGetDiagram(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	d, err := s.app.Diagram.Get(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// handlePublishDiagram POST /api/diagrams/{id}/publish — 发布相图版本。
func (s *Server) handlePublishDiagram(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	d, err := s.app.Diagram.Publish(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

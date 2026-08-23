package httpapi

import (
	"net/http"

	"task187-phaseaudit/internal/arbitration"
)

// handleOpenArbitration POST /api/arbitrations — 打开仲裁。
func (s *Server) handleOpenArbitration(w http.ResponseWriter, r *http.Request) {
	var in arbitration.OpenInput
	if err := decodeBody(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	a, err := s.app.Arbitration.Open(in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// handleListArbitrations GET /api/batches/{id}/arbitrations — 批次待仲裁任务。
func (s *Server) handleListArbitrations(w http.ResponseWriter, r *http.Request) {
	batchID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	as, err := s.app.Arbitration.ListOpen(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, as)
}

// handleGetArbitration GET /api/arbitrations/{id} — 仲裁详情。
func (s *Server) handleGetArbitration(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	a, err := s.app.Arbitration.Get(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// handleDecideArbitration POST /api/arbitrations/{id}/decide — 执行仲裁决定。
func (s *Server) handleDecideArbitration(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	var in arbitration.DecideInput
	if err := decodeBody(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	a, err := s.app.Arbitration.Decide(id, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

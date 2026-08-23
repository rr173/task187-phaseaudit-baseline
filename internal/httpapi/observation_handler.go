package httpapi

import (
	"net/http"

	"task187-phaseaudit/internal/observation"
)

// handleCreateObservation POST /api/batches/{id}/observations — 登记显微观察。
func (s *Server) handleCreateObservation(w http.ResponseWriter, r *http.Request) {
	batchID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	var in observation.CreateInput
	if err := decodeBody(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	in.BatchID = batchID
	o, err := s.app.Observation.Create(in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

// handleListObservations GET /api/batches/{id}/observations — 批次观察列表。
func (s *Server) handleListObservations(w http.ResponseWriter, r *http.Request) {
	batchID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	obs, err := s.app.Observation.ListByBatch(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, obs)
}

// handleGetObservation GET /api/observations/{id} — 观察详情。
func (s *Server) handleGetObservation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	o, err := s.app.Observation.Get(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// handleExcludeObservation POST /api/observations/{id}/exclude — 排除观察。
func (s *Server) handleExcludeObservation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.app.Observation.Exclude(id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "excluded"})
}

// handleResolveStatus POST /api/batches/{id}/observations/resolve — 重算观察状态（分歧检测）。
func (s *Server) handleResolveStatus(w http.ResponseWriter, r *http.Request) {
	batchID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.app.Observation.ResolveStatus(batchID); err != nil {
		writeErr(w, err)
		return
	}
	div, err := s.app.Observation.Divergence(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"batch_id": batchID, "divergence": div})
}

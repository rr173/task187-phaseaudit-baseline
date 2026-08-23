package httpapi

import (
	"net/http"
)

// handleHealth GET /api/health — 自检。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "task187-phaseaudit",
	})
}

// handleStats GET /api/stats — 全局统计。
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	batches, err := s.app.Batch.List(500, 0)
	if err != nil {
		writeErr(w, err)
		return
	}
	reports, err := s.app.Report.List(500, 0)
	if err != nil {
		writeErr(w, err)
		return
	}
	diagrams, err := s.app.Diagram.List()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"batch_count":   len(batches),
		"report_count":  len(reports),
		"diagram_count": len(diagrams),
	})
}

// handleBatchHistory GET /api/batches/{id}/history — 批次全量历史：观察、候选、仲裁、报告。
func (s *Server) handleBatchHistory(w http.ResponseWriter, r *http.Request) {
	batchID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := s.app.Batch.Get(batchID); err != nil {
		writeErr(w, err)
		return
	}
	obs, err := s.app.Observation.ListByBatch(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	cands, err := s.app.Inference.ListCandidates(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	arbs, err := s.app.Arbitration.ListOpen(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	reps, err := s.app.Report.ListByBatch(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"batch_id":       batchID,
		"observations":   obs,
		"candidates":     cands,
		"open_arbitrations": arbs,
		"reports":        reps,
	})
}

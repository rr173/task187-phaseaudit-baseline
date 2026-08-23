package httpapi

import (
	"net/http"

	"task187-phaseaudit/internal/inference"
)

// handleInfer POST /api/batches/{id}/infer — 执行候选推断。
func (s *Server) handleInfer(w http.ResponseWriter, r *http.Request) {
	batchID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	var in inference.InferInput
	_ = decodeBody(r, &in)
	in.BatchID = batchID
	b, err := s.app.Batch.Get(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	res, err := s.app.Inference.Infer(b, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleListCandidates GET /api/batches/{id}/candidates — 候选列表。
func (s *Server) handleListCandidates(w http.ResponseWriter, r *http.Request) {
	batchID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	cands, err := s.app.Inference.ListCandidates(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cands)
}

// handleGetCandidate GET /api/candidates/{id} — 候选详情。
func (s *Server) handleGetCandidate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	c, err := s.app.Inference.GetCandidate(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// handleConfirmCandidate POST /api/candidates/{id}/confirm — 复核者确认候选。
func (s *Server) handleConfirmCandidate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	c, err := s.app.Inference.ConfirmAcceptable(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// handleRejectCandidate POST /api/candidates/{id}/reject — 复核者拒绝候选。
func (s *Server) handleRejectCandidate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	_ = decodeBody(r, &in)
	c, err := s.app.Inference.RejectCandidate(id, in.Reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// handleFractionSum GET /api/batches/{id}/fractionsum — 比例和检查。
func (s *Server) handleFractionSum(w http.ResponseWriter, r *http.Request) {
	batchID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	res, err := s.app.Inference.CheckFractionSum(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

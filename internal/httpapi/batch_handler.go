package httpapi

import (
	"net/http"

	"task187-phaseaudit/internal/batch"
)

// handleCreateBatch POST /api/batches — 登记批次。
func (s *Server) handleCreateBatch(w http.ResponseWriter, r *http.Request) {
	var in batch.CreateInput
	if err := decodeBody(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	b, err := s.app.Batch.Create(in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// handleListBatches GET /api/batches — 列出批次。
func (s *Server) handleListBatches(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	bs, err := s.app.Batch.List(limit, offset)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bs)
}

// handleGetBatch GET /api/batches/{id} — 批次详情。
func (s *Server) handleGetBatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	b, err := s.app.Batch.Get(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// handleSealBatch POST /api/batches/{id}/seal — 封存批次。
func (s *Server) handleSealBatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.app.Batch.Seal(id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "sealed"})
}

// handleMoveToReview POST /api/batches/{id}/review — 置为待复核。
func (s *Server) handleMoveToReview(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.app.Batch.MoveToReview(id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "pending_review"})
}

// handleMarkInsufficient POST /api/batches/{id}/insufficient — 标记证据不足。
func (s *Server) handleMarkInsufficient(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	_ = decodeBody(r, &in)
	if err := s.app.Batch.MarkInsufficient(id, in.Reason); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "insufficient"})
}

// handleConfirmComposition POST /api/batches/{id}/fixed — 相组成已定。
func (s *Server) handleConfirmComposition(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.app.Batch.ConfirmComposition(id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "composition_fixed"})
}

// pagination 解析 limit/offset 查询参数。
func pagination(r *http.Request) (int, int) {
	limit := 100
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := parseAtoi(v); err == nil {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := parseAtoi(v); err == nil {
			offset = n
		}
	}
	return limit, offset
}

// parseAtoi 安全解析整数（panic-free）。
func parseAtoi(v string) (int, error) {
	n := 0
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return 0, errNotNumber
		}
		n = n*10 + int(ch-'0')
	}
	return n, nil
}

var errNotNumber = errParse{msg: "not a number"}

type errParse struct{ msg string }

func (e errParse) Error() string { return e.msg }

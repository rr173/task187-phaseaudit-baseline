// Package httpapi 提供 HTTP API 层：路由 /api 前缀，映射领域错误到 HTTP 状态码。
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/service"
)

// Server HTTP 服务。
type Server struct {
	app *service.App
	mux *http.ServeMux
}

// New 构造 HTTP 服务。
func New(app *service.App) *Server {
	s := &Server{app: app, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler 返回 http.Handler。
func (s *Server) Handler() http.Handler { return s.mux }

// routes 注册全部路由（统一 /api 前缀）。
func (s *Server) routes() {
	// 批次
	s.mux.HandleFunc("POST /api/batches", s.handleCreateBatch)
	s.mux.HandleFunc("GET /api/batches", s.handleListBatches)
	s.mux.HandleFunc("GET /api/batches/{id}", s.handleGetBatch)
	s.mux.HandleFunc("POST /api/batches/{id}/seal", s.handleSealBatch)
	s.mux.HandleFunc("POST /api/batches/{id}/review", s.handleMoveToReview)
	s.mux.HandleFunc("POST /api/batches/{id}/insufficient", s.handleMarkInsufficient)
	s.mux.HandleFunc("POST /api/batches/{id}/fixed", s.handleConfirmComposition)

	// 观察
	s.mux.HandleFunc("POST /api/batches/{id}/observations", s.handleCreateObservation)
	s.mux.HandleFunc("GET /api/batches/{id}/observations", s.handleListObservations)
	s.mux.HandleFunc("GET /api/observations/{id}", s.handleGetObservation)
	s.mux.HandleFunc("POST /api/observations/{id}/exclude", s.handleExcludeObservation)
	s.mux.HandleFunc("POST /api/batches/{id}/observations/resolve", s.handleResolveStatus)

	// 相图
	s.mux.HandleFunc("POST /api/diagrams", s.handleCreateDiagram)
	s.mux.HandleFunc("GET /api/diagrams", s.handleListDiagrams)
	s.mux.HandleFunc("GET /api/diagrams/{id}", s.handleGetDiagram)
	s.mux.HandleFunc("POST /api/diagrams/{id}/publish", s.handlePublishDiagram)

	// 候选推断
	s.mux.HandleFunc("POST /api/batches/{id}/infer", s.handleInfer)
	s.mux.HandleFunc("GET /api/batches/{id}/candidates", s.handleListCandidates)
	s.mux.HandleFunc("GET /api/candidates/{id}", s.handleGetCandidate)
	s.mux.HandleFunc("POST /api/candidates/{id}/confirm", s.handleConfirmCandidate)
	s.mux.HandleFunc("POST /api/candidates/{id}/reject", s.handleRejectCandidate)
	s.mux.HandleFunc("GET /api/batches/{id}/fractionsum", s.handleFractionSum)

	// 仲裁
	s.mux.HandleFunc("POST /api/arbitrations", s.handleOpenArbitration)
	s.mux.HandleFunc("GET /api/batches/{id}/arbitrations", s.handleListArbitrations)
	s.mux.HandleFunc("GET /api/arbitrations/{id}", s.handleGetArbitration)
	s.mux.HandleFunc("POST /api/arbitrations/{id}/decide", s.handleDecideArbitration)

	// 报告
	s.mux.HandleFunc("POST /api/batches/{id}/reports", s.handlePublishReport)
	s.mux.HandleFunc("GET /api/reports", s.handleListReports)
	s.mux.HandleFunc("GET /api/batches/{id}/reports", s.handleListBatchReports)
	s.mux.HandleFunc("GET /api/reports/{id}", s.handleGetReport)
	s.mux.HandleFunc("POST /api/reports/{id}/signoff", s.handleSignOffReport)
	s.mux.HandleFunc("POST /api/reports/{id}/revise", s.handleReviseReport)
	s.mux.HandleFunc("GET /api/reports/{id}/revisions", s.handleReportRevisions)
	s.mux.HandleFunc("GET /api/reports/compare", s.handleCompareReports)

	// 历史比较与自检
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)
	s.mux.HandleFunc("GET /api/batches/{id}/history", s.handleBatchHistory)
}

// --- 通用工具 ---

// writeJSON 写 JSON 响应。
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr 写错误响应。
func writeErr(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, model.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, model.ErrBatchSealed), errors.Is(err, model.ErrInvalidState),
		errors.Is(err, model.ErrConflict), errors.Is(err, model.ErrObservationConflict),
		errors.Is(err, model.ErrReportVersionDrift), errors.Is(err, model.ErrReportSuperseded),
		errors.Is(err, model.ErrDiagramNotPublished):
		code = http.StatusConflict
	case errors.Is(err, model.ErrFractionSumExceedsOne), errors.Is(err, model.ErrCompositionOutOfRange),
		errors.Is(err, model.ErrCrossBatchImage):
		code = http.StatusBadRequest
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

// pathID 解析路径参数为 int64。
func pathID(r *http.Request, name string) (int64, error) {
	v := r.PathValue(name)
	return strconv.ParseInt(v, 10, 64)
}

// decodeBody 解析 JSON 请求体。
func decodeBody(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

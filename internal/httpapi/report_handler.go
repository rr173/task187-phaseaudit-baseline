package httpapi

import (
	"net/http"

	"task187-phaseaudit/internal/report"
)

// handlePublishReport POST /api/batches/{id}/reports — 创建（草案）组织报告。
func (s *Server) handlePublishReport(w http.ResponseWriter, r *http.Request) {
	batchID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	var in report.PublishInput
	if err := decodeBody(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	in.BatchID = batchID
	rep, err := s.app.Report.Publish(in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rep)
}

// handleListReports GET /api/reports — 全部报告。
func (s *Server) handleListReports(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	reps, err := s.app.Report.List(limit, offset)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reps)
}

// handleListBatchReports GET /api/batches/{id}/reports — 批次报告列表。
func (s *Server) handleListBatchReports(w http.ResponseWriter, r *http.Request) {
	batchID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	reps, err := s.app.Report.ListByBatch(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reps)
}

// handleGetReport GET /api/reports/{id} — 报告详情。
func (s *Server) handleGetReport(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	rep, err := s.app.Report.Get(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// handleSignOffReport POST /api/reports/{id}/signoff — 签核发布。
func (s *Server) handleSignOffReport(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	rep, err := s.app.Report.SignOff(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// handleReviseReport POST /api/reports/{id}/revise — 补测修订。
func (s *Server) handleReviseReport(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Title      string `json:"title"`
		Conclusion string `json:"conclusion"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	rep, err := s.app.Report.Revise(id, in.Title, in.Conclusion)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rep)
}

// handleReportRevisions GET /api/reports/{id}/revisions — 修订历史。
func (s *Server) handleReportRevisions(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	revs, err := s.app.Report.Revisions(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, revs)
}

// handleCompareReports GET /api/reports/compare?from=&to= — 历史比较。
func (s *Server) handleCompareReports(w http.ResponseWriter, r *http.Request) {
	from, err1 := parseAtoi(r.URL.Query().Get("from"))
	to, err2 := parseAtoi(r.URL.Query().Get("to"))
	if err1 != nil || err2 != nil {
		writeErr(w, errNotNumber)
		return
	}
	res, err := s.app.Report.Compare(int64(from), int64(to))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

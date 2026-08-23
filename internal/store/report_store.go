package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"task187-phaseaudit/internal/model"
)

// ReportStore 组织报告的持久化读写。
type ReportStore struct{ db *DB }

// NewReportStore 构造 ReportStore。
func NewReportStore(db *DB) *ReportStore { return &ReportStore{db: db} }

// Create 创建报告（草案）。
func (s *ReportStore) Create(r *model.MicroReport) (*model.MicroReport, error) {
	ivJSON, err := json.Marshal(r.InputVersion)
	if err != nil {
		return nil, fmt.Errorf("marshal input version: %w", err)
	}
	now := Now()
	res, err := s.db.SQL().Exec(
		`INSERT INTO micro_reports(batch_id, title, conclusion, status, input_version, snapshot, created_at)
		 VALUES (?,?,?,?,?,?,?)`,
		r.BatchID, r.Title, r.Conclusion, r.Status, string(ivJSON), r.Snapshot, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert report: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	r.ID = id
	r.CreatedAt = now
	return r, nil
}

// Get 按 ID 查询报告。
func (s *ReportStore) Get(id int64) (*model.MicroReport, error) {
	row := s.db.SQL().QueryRow(
		`SELECT id,batch_id,title,conclusion,status,input_version,snapshot,created_at,COALESCE(published_at,''),COALESCE(superseded_at,''),COALESCE(superseded_by,0)
		 FROM micro_reports WHERE id=?`, id)
	return scanReport(row)
}

// ListByBatch 列出批次全部报告（按 ID 倒序）。
func (s *ReportStore) ListByBatch(batchID int64) ([]*model.MicroReport, error) {
	rows, err := s.db.SQL().Query(
		`SELECT id,batch_id,title,conclusion,status,input_version,snapshot,created_at,COALESCE(published_at,''),COALESCE(superseded_at,''),COALESCE(superseded_by,0)
		 FROM micro_reports WHERE batch_id=? ORDER BY id DESC`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.MicroReport
	for rows.Next() {
		r, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// List 列出全部报告（按 ID 倒序）。
func (s *ReportStore) List(limit, offset int) ([]*model.MicroReport, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.SQL().Query(
		`SELECT id,batch_id,title,conclusion,status,input_version,snapshot,created_at,COALESCE(published_at,''),COALESCE(superseded_at,''),COALESCE(superseded_by,0)
		 FROM micro_reports ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.MicroReport
	for rows.Next() {
		r, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Publish 发布报告：仅草案可发布；已替代的报告不可发布。
func (s *ReportStore) Publish(id int64) (*model.MicroReport, error) {
	now := Now()
	res, err := s.db.SQL().Exec(
		`UPDATE micro_reports SET status=?, published_at=? WHERE id=? AND status=?`,
		model.ReportPublished, now, id, model.ReportDraft)
	if err != nil {
		return nil, fmt.Errorf("publish report: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, model.ErrInvalidState
	}
	return s.Get(id)
}

// Supersede 将旧报告标记为已替代并记录替代者（版本漂移守卫的落盘侧）。
func (s *ReportStore) Supersede(oldID, newID int64, reason string) error {
	tx, err := s.db.SQL().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := Now()
	if _, err := tx.Exec(
		`UPDATE micro_reports SET status=?, superseded_at=?, superseded_by=? WHERE id=? AND status=?`,
		model.ReportSuperseded, now, newID, oldID, model.ReportPublished); err != nil {
		return fmt.Errorf("supersede report: %w", err)
	}
	// 追加修订记录。
	var rev int
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(rev_no),0) FROM report_revisions WHERE report_id=?`, oldID).Scan(&rev); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO report_revisions(report_id, rev_no, reason, supersedes_at, created_at) VALUES (?,?,?,?,?)`,
		oldID, rev+1, reason, now, now); err != nil {
		return fmt.Errorf("insert revision: %w", err)
	}
	return tx.Commit()
}

// Revisions 列出报告的修订历史。
func (s *ReportStore) Revisions(reportID int64) ([]Revision, error) {
	rows, err := s.db.SQL().Query(
		`SELECT rev_no, reason, supersedes_at, created_at FROM report_revisions WHERE report_id=? ORDER BY rev_no`, reportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Revision
	for rows.Next() {
		var r Revision
		if err := rows.Scan(&r.RevNo, &r.Reason, &r.SupersedesAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Revision 修订记录。
type Revision struct {
	RevNo        int    `json:"rev_no"`
	Reason       string `json:"reason"`
	SupersedesAt string `json:"supersedes_at"`
	CreatedAt    string `json:"created_at"`
}

type reportScanner interface {
	Scan(dest ...any) error
}

func scanReport(row reportScanner) (*model.MicroReport, error) {
	var r model.MicroReport
	var ivJSON string
	if err := row.Scan(&r.ID, &r.BatchID, &r.Title, &r.Conclusion, &r.Status, &ivJSON,
		&r.Snapshot, &r.CreatedAt, &r.PublishedAt, &r.SupersededAt, &r.SupersededBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(ivJSON), &r.InputVersion); err != nil {
		return nil, fmt.Errorf("unmarshal input version: %w", err)
	}
	return &r, nil
}

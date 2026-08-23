package store

import (
	"database/sql"
	"errors"
	"fmt"

	"task187-phaseaudit/internal/model"
)

// ArbitrationStore 仲裁记录的持久化读写。
type ArbitrationStore struct{ db *DB }

// NewArbitrationStore 构造 ArbitrationStore。
func NewArbitrationStore(db *DB) *ArbitrationStore { return &ArbitrationStore{db: db} }

// Create 创建仲裁记录。
func (s *ArbitrationStore) Create(a *model.Arbitration) (*model.Arbitration, error) {
	now := Now()
	res, err := s.db.SQL().Exec(
		`INSERT INTO arbitrations(batch_id, candidate_id, kind, reason, status, created_at)
		 VALUES (?,?,?,?,?,?)`,
		a.BatchID, a.CandidateID, a.Kind, a.Reason, model.ArbOpen, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert arbitration: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	a.ID = id
	a.Status = model.ArbOpen
	a.CreatedAt = now
	return a, nil
}

// Get 按 ID 查询仲裁。
func (s *ArbitrationStore) Get(id int64) (*model.Arbitration, error) {
	row := s.db.SQL().QueryRow(
		`SELECT id,batch_id,candidate_id,kind,reason,status,COALESCE(decision,''),COALESCE(note,''),created_at,COALESCE(decided_at,'')
		 FROM arbitrations WHERE id=?`, id)
	return scanArbitration(row)
}

// ListOpen 列出某批次打开的仲裁（重启后恢复待仲裁任务）。
func (s *ArbitrationStore) ListOpen(batchID int64) ([]*model.Arbitration, error) {
	rows, err := s.db.SQL().Query(
		`SELECT id,batch_id,candidate_id,kind,reason,status,COALESCE(decision,''),COALESCE(note,''),created_at,COALESCE(decided_at,'')
		 FROM arbitrations WHERE batch_id=? AND status=? ORDER BY id`, batchID, model.ArbOpen)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Arbitration
	for rows.Next() {
		a, err := scanArbitration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// HasOpenForCandidate 判断候选是否已有打开的仲裁（防止重复仲裁）。
func (s *ArbitrationStore) HasOpenForCandidate(candidateID int64) (bool, error) {
	var n int
	err := s.db.SQL().QueryRow(
		`SELECT COUNT(*) FROM arbitrations WHERE candidate_id=? AND status=?`,
		candidateID, model.ArbOpen).Scan(&n)
	return n > 0, err
}

// Decide 裁决：关闭仲裁并写入决定。
func (s *ArbitrationStore) Decide(id int64, decision, note string) error {
	res, err := s.db.SQL().Exec(
		`UPDATE arbitrations SET status=?, decision=?, note=?, decided_at=? WHERE id=? AND status=?`,
		model.ArbDecided, decision, note, Now(), id, model.ArbOpen)
	if err != nil {
		return fmt.Errorf("decide arbitration: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrInvalidState
	}
	return nil
}

// Dismiss 驳回仲裁（无需仲裁）。
func (s *ArbitrationStore) Dismiss(id int64, note string) error {
	_, err := s.db.SQL().Exec(
		`UPDATE arbitrations SET status=?, note=?, decided_at=? WHERE id=? AND status=?`,
		model.ArbDismissed, note, Now(), id, model.ArbOpen)
	if err != nil {
		return fmt.Errorf("dismiss arbitration: %w", err)
	}
	return nil
}

type arbitrationScanner interface {
	Scan(dest ...any) error
}

func scanArbitration(row arbitrationScanner) (*model.Arbitration, error) {
	var a model.Arbitration
	if err := row.Scan(&a.ID, &a.BatchID, &a.CandidateID, &a.Kind, &a.Reason, &a.Status,
		&a.Decision, &a.Note, &a.CreatedAt, &a.DecidedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

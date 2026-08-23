package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"task187-phaseaudit/internal/model"
)

// CandidateStore 相候选的持久化读写。
type CandidateStore struct{ db *DB }

// NewCandidateStore 构造 CandidateStore。
func NewCandidateStore(db *DB) *CandidateStore { return &CandidateStore{db: db} }

// Upsert 插入或更新候选（batch+diagram+phase 唯一）。返回最终记录。
func (s *CandidateStore) Upsert(c *model.PhaseCandidate) (*model.PhaseCandidate, error) {
	consJSON, err := json.Marshal(c.Conservation)
	if err != nil {
		return nil, fmt.Errorf("marshal conservation: %w", err)
	}
	now := Now()
	_, err = s.db.SQL().Exec(
		`INSERT INTO phase_candidates(batch_id, diagram_id, phase, fraction, fraction_low, fraction_high, status, conservation, evidence, inferred_at, confirmed_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(batch_id, diagram_id, phase) DO UPDATE SET
		   fraction=excluded.fraction, fraction_low=excluded.fraction_low, fraction_high=excluded.fraction_high,
		   status=excluded.status, conservation=excluded.conservation, evidence=excluded.evidence,
		   inferred_at=excluded.inferred_at, confirmed_at=excluded.confirmed_at`,
		c.BatchID, c.DiagramID, c.Phase, c.Fraction, c.FractionLow, c.FractionHigh, c.Status,
		string(consJSON), c.Evidence, now, c.ConfirmedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert candidate: %w", err)
	}
	row := s.db.SQL().QueryRow(
		`SELECT id,batch_id,diagram_id,phase,fraction,fraction_low,fraction_high,status,conservation,evidence,inferred_at,COALESCE(confirmed_at,'')
		 FROM phase_candidates WHERE batch_id=? AND diagram_id=? AND phase=?`,
		c.BatchID, c.DiagramID, c.Phase)
	return scanCandidate(row)
}

// Get 按 ID 查询候选。
func (s *CandidateStore) Get(id int64) (*model.PhaseCandidate, error) {
	row := s.db.SQL().QueryRow(
		`SELECT id,batch_id,diagram_id,phase,fraction,fraction_low,fraction_high,status,conservation,evidence,inferred_at,COALESCE(confirmed_at,'')
		 FROM phase_candidates WHERE id=?`, id)
	return scanCandidate(row)
}

// ListByBatch 列出批次全部候选。
func (s *CandidateStore) ListByBatch(batchID int64) ([]*model.PhaseCandidate, error) {
	rows, err := s.db.SQL().Query(
		`SELECT id,batch_id,diagram_id,phase,fraction,fraction_low,fraction_high,status,conservation,evidence,inferred_at,COALESCE(confirmed_at,'')
		 FROM phase_candidates WHERE batch_id=? ORDER BY id`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.PhaseCandidate
	for rows.Next() {
		c, err := scanCandidate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateStatus 更新候选状态。
func (s *CandidateStore) UpdateStatus(id int64, status string) error {
	_, err := s.db.SQL().Exec(`UPDATE phase_candidates SET status=? WHERE id=?`, model.CandAcceptable, id)
	if err != nil {
		return fmt.Errorf("update candidate status: %w", err)
	}
	return nil
}

// Confirm 确认候选（写入确认时间，状态置 confirmed）。
func (s *CandidateStore) Confirm(id int64) error {
	res, err := s.db.SQL().Exec(
		`UPDATE phase_candidates SET status=?, confirmed_at=? WHERE id=?`, model.CandConfirmed, Now(), id)
	if err != nil {
		return fmt.Errorf("confirm candidate: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// CountConfirmed 统计批次已确认候选数。
func (s *CandidateStore) CountConfirmed(batchID int64) (int, error) {
	var n int
	err := s.db.SQL().QueryRow(
		`SELECT COUNT(*) FROM phase_candidates WHERE batch_id=? AND status=?`,
		batchID, model.CandConfirmed).Scan(&n)
	return n, err
}

// SumFraction 统计批次候选比例和。
func (s *CandidateStore) SumFraction(batchID int64, excludeID int64) (float64, error) {
	var sum float64
	err := s.db.SQL().QueryRow(
		`SELECT COALESCE(SUM(fraction),0) FROM phase_candidates WHERE batch_id=? AND id<>?`,
		batchID, excludeID).Scan(&sum)
	return sum, err
}

type candidateScanner interface {
	Scan(dest ...any) error
}

func scanCandidate(row candidateScanner) (*model.PhaseCandidate, error) {
	var c model.PhaseCandidate
	var consJSON string
	if err := row.Scan(&c.ID, &c.BatchID, &c.DiagramID, &c.Phase, &c.Fraction, &c.FractionLow,
		&c.FractionHigh, &c.Status, &consJSON, &c.Evidence, &c.InferredAt, &c.ConfirmedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(consJSON), &c.Conservation); err != nil {
		return nil, fmt.Errorf("unmarshal conservation: %w", err)
	}
	return &c, nil
}

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
	return s.UpsertTx(s.db.SQL(), c)
}

// UpsertTx 在指定执行器（*sql.DB 或事务内 *sql.Tx）上插入或更新候选。
// 在事务内执行时读回最终记录也使用同一事务，保证写入与读回一致可见。
func (s *CandidateStore) UpsertTx(tx DBTX, c *model.PhaseCandidate) (*model.PhaseCandidate, error) {
	consJSON, err := json.Marshal(c.Conservation)
	if err != nil {
		return nil, fmt.Errorf("marshal conservation: %w", err)
	}
	now := Now()
	if _, err := tx.Exec(
		`INSERT INTO phase_candidates(batch_id, diagram_id, phase, fraction, fraction_low, fraction_high, status, conservation, evidence, inferred_at, confirmed_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(batch_id, diagram_id, phase) DO UPDATE SET
		   fraction=excluded.fraction, fraction_low=excluded.fraction_low, fraction_high=excluded.fraction_high,
		   status=excluded.status, conservation=excluded.conservation, evidence=excluded.evidence,
		   inferred_at=excluded.inferred_at, confirmed_at=excluded.confirmed_at`,
		c.BatchID, c.DiagramID, c.Phase, c.Fraction, c.FractionLow, c.FractionHigh, c.Status,
		string(consJSON), c.Evidence, now, c.ConfirmedAt,
	); err != nil {
		return nil, fmt.Errorf("upsert candidate: %w", err)
	}
	row := tx.QueryRow(
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

// UpdateStatus 更新候选状态（落盘传入的 status，不再硬编码）。
func (s *CandidateStore) UpdateStatus(id int64, status string) error {
	return s.UpdateStatusTx(s.db.SQL(), id, status)
}

// UpdateStatusTx 在指定执行器（*sql.DB 或事务内 *sql.Tx）上更新候选状态，
// 供跨表原子写复用：仲裁决定需在同一事务中驱动候选与批次状态流转。
func (s *CandidateStore) UpdateStatusTx(tx DBTX, id int64, status string) error {
	res, err := tx.Exec(`UPDATE phase_candidates SET status=? WHERE id=?`, status, id)
	if err != nil {
		return fmt.Errorf("update candidate status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// Confirm 确认候选（写入确认时间，状态置 confirmed）。
func (s *CandidateStore) Confirm(id int64) error {
	return s.ConfirmTx(s.db.SQL(), id)
}

// ConfirmTx 在指定执行器（*sql.DB 或事务内 *sql.Tx）上确认候选，
// 供仲裁确认决定在同一事务内与关闭仲裁一并提交。
func (s *CandidateStore) ConfirmTx(tx DBTX, id int64) error {
	res, err := tx.Exec(
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

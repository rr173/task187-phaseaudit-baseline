package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"task187-phaseaudit/internal/model"
)

// BatchStore 材料批次的持久化读写。
type BatchStore struct{ db *DB }

// NewBatchStore 构造 BatchStore。
func NewBatchStore(db *DB) *BatchStore { return &BatchStore{db: db} }

// Create 插入新批次；相同指纹返回既有记录（幂等）。
func (s *BatchStore) Create(b *model.MaterialBatch) (*model.MaterialBatch, error) {
	compJSON, err := json.Marshal(b.Composition)
	if err != nil {
		return nil, fmt.Errorf("marshal composition: %w", err)
	}
	hhJSON, err := json.Marshal(b.HeatHistory)
	if err != nil {
		return nil, fmt.Errorf("marshal heat history: %w", err)
	}
	now := Now()
	res, err := s.db.SQL().Exec(
		`INSERT INTO material_batches(name, alloy, composition, heat_history, status, fingerprint, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		b.Name, b.Alloy, string(compJSON), string(hhJSON), b.Status, b.Fingerprint, now, now,
	)
	if err != nil {
		if isUniqueViolation(err) {
			existing, gerr := s.ByFingerprint(b.Fingerprint)
			if gerr == nil {
				return existing, nil
			}
			return nil, fmt.Errorf("duplicate batch fingerprint: %w", gerr)
		}
		return nil, fmt.Errorf("insert batch: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	b.ID = id
	b.CreatedAt, b.UpdatedAt = now, now
	return b, nil
}

// ByFingerprint 按指纹查询批次。
func (s *BatchStore) ByFingerprint(fp string) (*model.MaterialBatch, error) {
	row := s.db.SQL().QueryRow(
		`SELECT id,name,alloy,composition,heat_history,status,fingerprint,created_at,updated_at,COALESCE(sealed_at,'')
		 FROM material_batches WHERE fingerprint=?`, fp)
	return scanBatch(row)
}

// Get 按 ID 查询批次。
func (s *BatchStore) Get(id int64) (*model.MaterialBatch, error) {
	row := s.db.SQL().QueryRow(
		`SELECT id,name,alloy,composition,heat_history,status,fingerprint,created_at,updated_at,COALESCE(sealed_at,'')
		 FROM material_batches WHERE id=?`, id)
	return scanBatch(row)
}

// List 列出批次（按创建时间倒序）。
func (s *BatchStore) List(limit, offset int) ([]*model.MaterialBatch, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.SQL().Query(
		`SELECT id,name,alloy,composition,heat_history,status,fingerprint,created_at,updated_at,COALESCE(sealed_at,'')
		 FROM material_batches ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.MaterialBatch
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// UpdateStatus 更新批次状态（原子单行更新）。
func (s *BatchStore) UpdateStatus(id int64, status string) error {
	_, err := s.db.SQL().Exec(
		`UPDATE material_batches SET status=?, updated_at=? WHERE id=?`, model.BatchPendingReview, Now(), id)
	if err != nil {
		return fmt.Errorf("update batch status: %w", err)
	}
	return nil
}

// Seal 封存批次：写入封存时间并锁定状态。
func (s *BatchStore) Seal(id int64) error {
	res, err := s.db.SQL().Exec(
		`UPDATE material_batches SET status=?, sealed_at=?, updated_at=? WHERE id=?`,
		model.BatchSealed, Now(), Now(), id)
	if err != nil {
		return fmt.Errorf("seal batch: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

type batchScanner interface {
	Scan(dest ...any) error
}

func scanBatch(row batchScanner) (*model.MaterialBatch, error) {
	var b model.MaterialBatch
	var compJSON, hhJSON string
	if err := row.Scan(&b.ID, &b.Name, &b.Alloy, &compJSON, &hhJSON, &b.Status, &b.Fingerprint,
		&b.CreatedAt, &b.UpdatedAt, &b.SealedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(compJSON), &b.Composition); err != nil {
		return nil, fmt.Errorf("unmarshal composition: %w", err)
	}
	if err := json.Unmarshal([]byte(hhJSON), &b.HeatHistory); err != nil {
		return nil, fmt.Errorf("unmarshal heat history: %w", err)
	}
	return &b, nil
}

package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"task187-phaseaudit/internal/model"
)

// ObservationStore 显微观察的持久化读写。
type ObservationStore struct{ db *DB }

// NewObservationStore 构造 ObservationStore。
func NewObservationStore(db *DB) *ObservationStore { return &ObservationStore{db: db} }

// Create 插入观察；相同指纹（图像+观察者+特征）幂等返回既有记录。
func (s *ObservationStore) Create(o *model.Observation) (*model.Observation, error) {
	estJSON, err := json.Marshal(o.PhaseEstimate)
	if err != nil {
		return nil, fmt.Errorf("marshal phase estimate: %w", err)
	}
	now := Now()
	res, err := s.db.SQL().Exec(
		`INSERT INTO observations(batch_id, observer, image_ref, feature_notes, grain_size_um, inclusion_level, phase_estimate, status, fingerprint, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		o.BatchID, o.Observer, o.ImageRef, o.FeatureNotes, o.GrainSizeUM, o.InclusionLevel,
		string(estJSON), o.Status, o.Fingerprint, now,
	)
	if err != nil {
		if isUniqueViolation(err) {
			existing, gerr := s.ByFingerprint(o.Fingerprint)
			if gerr == nil {
				return existing, nil
			}
			return nil, fmt.Errorf("duplicate observation: %w", gerr)
		}
		return nil, fmt.Errorf("insert observation: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	o.ID = id
	o.CreatedAt = now
	return o, nil
}

// ByFingerprint 按指纹查询观察。
func (s *ObservationStore) ByFingerprint(fp string) (*model.Observation, error) {
	row := s.db.SQL().QueryRow(
		`SELECT id,batch_id,observer,image_ref,feature_notes,grain_size_um,inclusion_level,phase_estimate,status,fingerprint,created_at
		 FROM observations WHERE fingerprint=?`, fp)
	return scanObservation(row)
}

// Get 按 ID 查询观察。
func (s *ObservationStore) Get(id int64) (*model.Observation, error) {
	row := s.db.SQL().QueryRow(
		`SELECT id,batch_id,observer,image_ref,feature_notes,grain_size_um,inclusion_level,phase_estimate,status,fingerprint,created_at
		 FROM observations WHERE id=?`, id)
	return scanObservation(row)
}

// ListByBatch 列出某批次全部观察。
func (s *ObservationStore) ListByBatch(batchID int64) ([]*model.Observation, error) {
	rows, err := s.db.SQL().Query(
		`SELECT id,batch_id,observer,image_ref,feature_notes,grain_size_um,inclusion_level,phase_estimate,status,fingerprint,created_at
		 FROM observations WHERE batch_id=? ORDER BY id`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Observation
	for rows.Next() {
		o, err := scanObservation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// CountSupporting 统计某批次处于支持/待处理状态的观察数。
func (s *ObservationStore) CountSupporting(batchID int64) (int, error) {
	var n int
	err := s.db.SQL().QueryRow(
		`SELECT COUNT(*) FROM observations WHERE batch_id=? AND status IN (?,?)`,
		batchID, model.ObsSupport, model.ObsPending).Scan(&n)
	return n, err
}

// UpdateStatus 更新观察状态。
func (s *ObservationStore) UpdateStatus(id int64, status string) error {
	_, err := s.db.SQL().Exec(`UPDATE observations SET status=? WHERE id=?`, status, id)
	if err != nil {
		return fmt.Errorf("update observation status: %w", err)
	}
	return nil
}

// HasImageInOtherBatch 检查图像是否已被其他批次引用（跨批次引用守卫）。
func (s *ObservationStore) HasImageInOtherBatch(imageRef string, excludeBatchID int64) (bool, error) {
	var n int
	err := s.db.SQL().QueryRow(
		`SELECT COUNT(*) FROM observations WHERE image_ref=? AND batch_id<>?`, imageRef, excludeBatchID).Scan(&n)
	return n > 0, err
}

type observationScanner interface {
	Scan(dest ...any) error
}

func scanObservation(row observationScanner) (*model.Observation, error) {
	var o model.Observation
	var estJSON string
	if err := row.Scan(&o.ID, &o.BatchID, &o.Observer, &o.ImageRef, &o.FeatureNotes,
		&o.GrainSizeUM, &o.InclusionLevel, &estJSON, &o.Status, &o.Fingerprint, &o.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(estJSON), &o.PhaseEstimate); err != nil {
		return nil, fmt.Errorf("unmarshal phase estimate: %w", err)
	}
	return &o, nil
}

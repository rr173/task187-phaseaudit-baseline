// Package observation 实现显微观察模块：记录特征证据、估计相比例、分歧检测与排除。
package observation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/store"
)

// Service 观察模块服务。
type Service struct {
	store *store.ObservationStore
	batches *store.BatchStore
}

// New 构造观察服务。
func New(store *store.ObservationStore, batches *store.BatchStore) *Service {
	return &Service{store: store, batches: batches}
}

// CreateInput 创建观察入参。
type CreateInput struct {
	BatchID        int64              `json:"batch_id"`
	Observer       string             `json:"observer"`
	ImageRef       string             `json:"image_ref"`
	FeatureNotes   string             `json:"feature_notes"`
	GrainSizeUM    float64            `json:"grain_size_um"`
	InclusionLevel float64            `json:"inclusion_level"`
	PhaseEstimate  map[string]float64 `json:"phase_estimate"`
}

// Create 登记观察证据。约束：
//   - 批次不能已封存；
//   - 图像不能已被其他批次引用（跨批次图像引用守卫）；
//   - 相同图像+观察者+特征指纹幂等。
func (s *Service) Create(in CreateInput) (*model.Observation, error) {
	batch, err := s.batches.Get(in.BatchID)
	if err != nil {
		return nil, err
	}
	if batch.Status == model.BatchSealed {
		return nil, model.ErrBatchSealed
	}
	if strings.TrimSpace(in.Observer) == "" || strings.TrimSpace(in.ImageRef) == "" {
		return nil, fmt.Errorf("观察者与图像标识不能为空")
	}
	if in.InclusionLevel < 0 || in.InclusionLevel > 5 {
		return nil, fmt.Errorf("夹杂物等级必须在 0-5 之间")
	}
	if err := model.VerifyConservation(in.PhaseEstimate, 100); err != nil {
		return nil, err
	}
	used, err := s.store.HasImageInOtherBatch(in.ImageRef, in.BatchID)
	if err != nil {
		return nil, err
	}
	if used {
		return nil, model.ErrCrossBatchImage
	}
	o := &model.Observation{
		BatchID:        in.BatchID,
		Observer:       in.Observer,
		ImageRef:       in.ImageRef,
		FeatureNotes:   in.FeatureNotes,
		GrainSizeUM:    in.GrainSizeUM,
		InclusionLevel: in.InclusionLevel,
		PhaseEstimate:  in.PhaseEstimate,
		Status:         model.ObsPending,
		Fingerprint:    Fingerprint(in.BatchID, in.Observer, in.ImageRef, in.FeatureNotes, in.PhaseEstimate),
	}
	return s.store.Create(o)
}

// Get 查询观察。
func (s *Service) Get(id int64) (*model.Observation, error) { return s.store.Get(id) }

// ListByBatch 列出批次全部观察。
func (s *Service) ListByBatch(batchID int64) ([]*model.Observation, error) {
	return s.store.ListByBatch(batchID)
}

// ResolveStatus 根据观察者分歧情况更新观察状态：
// 同一批次的观察者估计不一致 → 标记 conflict；一致 → support。
func (s *Service) ResolveStatus(batchID int64) error {
	obs, err := s.store.ListByBatch(batchID)
	if err != nil {
		return err
	}
	// 取第一个未排除观察的比例估计作为基准。
	var baseline map[string]float64
	for _, o := range obs {
		if o.Status == model.ObsExcluded {
			continue
		}
		baseline = o.PhaseEstimate
		break
	}
	if baseline == nil {
		return nil
	}
	for _, o := range obs {
		if o.Status == model.ObsExcluded || o.Status == model.ObsSuperseded {
			continue
		}
		if estimatesDiverge(o.PhaseEstimate, baseline) {
			if err := s.store.UpdateStatus(o.ID, model.ObsConflict); err != nil {
				return err
			}
		} else if o.Status == model.ObsPending {
			if err := s.store.UpdateStatus(o.ID, model.ObsSupport); err != nil {
				return err
			}
		}
	}
	return nil
}

// Exclude 排除观察（重复/无效证据），仅待处理或支持状态可排除。
func (s *Service) Exclude(id int64) error {
	o, err := s.store.Get(id)
	if err != nil {
		return err
	}
	if o.Status == model.ObsExcluded {
		return nil
	}
	if o.Status == model.ObsConflict {
		return fmt.Errorf("冲突观察不可直接排除，需先仲裁")
	}
	return s.store.UpdateStatus(id, model.ObsExcluded)
}

// Divergence 检查批次内观察者分歧是否存在（供推断模块与仲裁模块使用）。
func (s *Service) Divergence(batchID int64) (bool, error) {
	obs, err := s.store.ListByBatch(batchID)
	if err != nil {
		return false, err
	}
	var active []*model.Observation
	for _, o := range obs {
		if o.Status != model.ObsExcluded && o.Status != model.ObsSuperseded {
			active = append(active, o)
		}
	}
	if len(active) < 2 {
		return false, nil
	}
	baseline := active[0].PhaseEstimate
	for _, o := range active[1:] {
		if estimatesDiverge(o.PhaseEstimate, baseline) {
			return true, nil
		}
	}
	return false, nil
}

// estimatesDiverge 判断两组相比例估计是否分歧（任一相比例差超过 5 个百分点）。
func estimatesDiverge(a, b map[string]float64) bool {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	for k := range keys {
		diff := a[k] - b[k]
		if diff > 5 || diff < -5 {
			return true
		}
	}
	return false
}

// Fingerprint 观察指纹：批次+观察者+图像+特征+比例估计。
func Fingerprint(batchID int64, observer, imageRef, feature string, est map[string]float64) string {
	keys := make([]string, 0, len(est))
	for k := range est {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d|%s|%s|%s", batchID, observer, imageRef, feature)
	for _, k := range keys {
		fmt.Fprintf(&sb, "|%s=%.4f", k, est[k])
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

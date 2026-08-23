// Package batch 实现材料批次模块：登记合金成分与热历史、状态流转与封存。
package batch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/store"
)

// Service 批次模块服务。
type Service struct {
	store *store.BatchStore
	obs   *store.ObservationStore
}

// New 构造批次服务。
func New(store *store.BatchStore, obs *store.ObservationStore) *Service {
	return &Service{store: store, obs: obs}
}

// CreateInput 创建批次入参。
type CreateInput struct {
	Name        string             `json:"name"`
	Alloy       string             `json:"alloy"`
	Composition model.Composition  `json:"composition"`
	HeatHistory model.HeatHistory  `json:"heat_history"`
}

// Create 登记新批次。相同成分+热历史指纹幂等返回既有批次。
func (s *Service) Create(in CreateInput) (*model.MaterialBatch, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("批次名称不能为空")
	}
	if len(in.Composition) == 0 {
		return nil, fmt.Errorf("成分不能为空")
	}
	norm, _ := model.NormalizeComposition(in.Composition)
	for el, v := range norm {
		if v < 0 {
			return nil, fmt.Errorf("%w: %s=%v", model.ErrNegativeFraction, el, v)
		}
	}
	fp := Fingerprint(norm, in.HeatHistory)
	b := &model.MaterialBatch{
		Name:        in.Name,
		Alloy:       in.Alloy,
		Composition: norm,
		HeatHistory: in.HeatHistory,
		Status:      model.BatchPendingObservation,
		Fingerprint: fp,
	}
	return s.store.Create(b)
}

// Get 查询批次详情。
func (s *Service) Get(id int64) (*model.MaterialBatch, error) { return s.store.Get(id) }

// List 列出批次。
func (s *Service) List(limit, offset int) ([]*model.MaterialBatch, error) {
	return s.store.List(limit, offset)
}

// MoveToReview 将批次置为待复核（观察齐备、候选推断完成后的入口）。
func (s *Service) MoveToReview(id int64) error {
	b, err := s.store.Get(id)
	if err != nil {
		return err
	}
	if b.Status == model.BatchSealed {
		return model.ErrBatchSealed
	}
	if b.Status != model.BatchPendingObservation {
		return model.ErrInvalidState
	}
	return s.store.UpdateStatus(id, model.BatchPendingReview)
}

// MarkInsufficient 标记证据不足（需要补测）。
func (s *Service) MarkInsufficient(id int64, reason string) error {
	b, err := s.store.Get(id)
	if err != nil {
		return err
	}
	if b.Status == model.BatchSealed {
		return model.ErrBatchSealed
	}
	return s.store.UpdateStatus(id, model.BatchInsufficient)
}

// ConfirmComposition 相组成已定：候选确认后批次流转到 composition_fixed。
func (s *Service) ConfirmComposition(id int64) error {
	b, err := s.store.Get(id)
	if err != nil {
		return err
	}
	if b.Status == model.BatchSealed {
		return model.ErrBatchSealed
	}
	if b.Status != model.BatchPendingReview {
		return fmt.Errorf("%w: 批次状态 %s 不可确认相组成", model.ErrInvalidState, b.Status)
	}
	return s.store.UpdateStatus(id, model.BatchCompositionFixed)
}

// Seal 封存批次：此后禁止直接修改（观察/候选/报告照常可读）。
func (s *Service) Seal(id int64) error {
	b, err := s.store.Get(id)
	if err != nil {
		return err
	}
	if b.Status == model.BatchSealed {
		return nil // 幂等
	}
	return s.store.Seal(id)
}

// Fingerprint 计算批次指纹：规范化的成分键值对 + 热历史步骤摘要。
func Fingerprint(comp model.Composition, hh model.HeatHistory) string {
	els := make([]string, 0, len(comp))
	for el := range comp {
		els = append(els, el)
	}
	sort.Strings(els)
	var sb strings.Builder
	for _, el := range els {
		fmt.Fprintf(&sb, "%s=%.4f;", el, comp[el])
	}
	for _, st := range hh.Steps {
		fmt.Fprintf(&sb, "|%s@%.1fC/%gh/%s", st.Process, st.TempC, st.DurationH, st.Cooling)
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

// MarshalComposition 供服务层序列化。
func MarshalComposition(c model.Composition) (string, error) {
	b, err := json.Marshal(c)
	return string(b), err
}

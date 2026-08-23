// Package arbitration 实现冲突仲裁模块：处理守恒失败与观察者分歧，
// 作出「确认 / 下调 / 拒绝」决定，并在决定后驱动候选与批次状态流转。
package arbitration

import (
	"fmt"

	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/store"
)

// Kind 常量：仲裁触发类型。
const (
	KindObservationConflict = "observation_conflict" // 观察者分歧
	KindConservationFailure = "conservation_failure" // 成分守恒失败
)

// Service 仲裁模块服务。
type Service struct {
	store *store.ArbitrationStore
	cands *store.CandidateStore
	batch *store.BatchStore
}

// New 构造仲裁服务。
func New(store *store.ArbitrationStore, cands *store.CandidateStore, batch *store.BatchStore) *Service {
	return &Service{store: store, cands: cands, batch: batch}
}

// OpenInput 创建仲裁入参。
type OpenInput struct {
	BatchID     int64  `json:"batch_id"`
	CandidateID int64  `json:"candidate_id"`
	Kind        string `json:"kind"`
	Reason      string `json:"reason"`
}

// Open 打开一次仲裁。候选需处于 arbitration 状态（或可仲裁状态）。
func (s *Service) Open(in OpenInput) (*model.Arbitration, error) {
	c, err := s.cands.Get(in.CandidateID)
	if err != nil {
		return nil, err
	}
	if c.BatchID != in.BatchID {
		return nil, fmt.Errorf("候选 %d 不属于批次 %d", in.CandidateID, in.BatchID)
	}
	if c.Status != model.CandArbitration {
		return nil, fmt.Errorf("%w: 候选状态 %s 无需仲裁", model.ErrInvalidState, c.Status)
	}
	has, err := s.store.HasOpenForCandidate(in.CandidateID)
	if err != nil {
		return nil, err
	}
	if has {
		return nil, fmt.Errorf("候选 %d 已有打开的仲裁", in.CandidateID)
	}
	if in.Kind != KindObservationConflict && in.Kind != KindConservationFailure {
		return nil, fmt.Errorf("未知仲裁类型 %q", in.Kind)
	}
	a := &model.Arbitration{
		BatchID:     in.BatchID,
		CandidateID: in.CandidateID,
		Kind:        in.Kind,
		Reason:      in.Reason,
	}
	return s.store.Create(a)
}

// DecideInput 仲裁决定入参。
type DecideInput struct {
	Decision string `json:"decision"` // confirm / downscale / reject
	Note     string `json:"note"`
	NewFraction *float64 `json:"new_fraction,omitempty"` // downscale 时的新比例
}

// Decide 执行仲裁决定并驱动状态流转：
//   - confirm  → 候选 confirmed，批次 → composition_fixed（若比例和 ≤ 1）；
//   - downscale → 候选按新比例重写并置 acceptable（需守恒重检），批次 → pending_review；
//   - reject   → 候选 rejected。
func (s *Service) Decide(arbID int64, in DecideInput) (*model.Arbitration, error) {
	a, err := s.store.Get(arbID)
	if err != nil {
		return nil, err
	}
	if a.Status != model.ArbOpen {
		return nil, fmt.Errorf("%w: 仲裁已关闭", model.ErrInvalidState)
	}
	c, err := s.cands.Get(a.CandidateID)
	if err != nil {
		return nil, err
	}

	switch in.Decision {
	case model.DecisionConfirm:
		if err := s.cands.Confirm(c.ID); err != nil {
			return nil, err
		}
		// 批次推进由调用方显式流转（MoveToReview → ConfirmComposition），
		// 因为可能还有其它候选需要一并确认。
	case model.DecisionDownscale:
		if in.NewFraction == nil {
			return nil, fmt.Errorf("下调决定必须提供新比例")
		}
		nf := *in.NewFraction
		if nf < 0 || nf > 1 {
			return nil, fmt.Errorf("%w: 新比例 %v", model.ErrNegativeFraction, nf)
		}
		// 重写比例：直接更新 fraction 字段（经 upsert 落盘）。
		if err := s.rewriteFraction(c, nf); err != nil {
			return nil, err
		}
		if err := s.cands.UpdateStatus(c.ID, model.CandAcceptable); err != nil {
			return nil, err
		}
		if err := s.batch.UpdateStatus(a.BatchID, model.BatchPendingReview); err != nil {
			return nil, err
		}
	case model.DecisionReject:
		if err := s.cands.UpdateStatus(c.ID, model.CandArbitration); err != nil {
			return nil, err
		}
		if err := s.batch.UpdateStatus(a.BatchID, model.BatchInsufficient); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("未知仲裁决定 %q", in.Decision)
	}

	if err := s.store.Decide(arbID, in.Decision, in.Note); err != nil {
		return nil, err
	}
	return s.store.Get(arbID)
}

// rewriteFraction 重写候选比例（fraction/区间），供下调使用。
func (s *Service) rewriteFraction(c *model.PhaseCandidate, frac float64) error {
	// 通过 upsert 全量写回（同 batch+diagram+phase 冲突更新）。
	up := *c
	up.Fraction = frac
	up.FractionLow = frac * 0.8
	up.FractionHigh = frac * 1.2
	if up.FractionHigh > 1 {
		up.FractionHigh = 1
	}
	_, err := s.cands.Upsert(&up)
	return err
}

// ListOpen 列出批次打开的仲裁（重启后恢复待仲裁任务）。
func (s *Service) ListOpen(batchID int64) ([]*model.Arbitration, error) {
	return s.store.ListOpen(batchID)
}

// Get 查询仲裁详情。
func (s *Service) Get(id int64) (*model.Arbitration, error) { return s.store.Get(id) }

// OpenCount 统计批次打开仲裁数。
func (s *Service) OpenCount(batchID int64) (int, error) {
	list, err := s.store.ListOpen(batchID)
	return len(list), err
}

package inference

import (
	"fmt"
	"sort"

	"task187-phaseaudit/internal/model"
)

// SumCheckResult 比例和检查结果。
type SumCheckResult struct {
	OK         bool    `json:"ok"`
	Sum        float64 `json:"sum"`
	Limit      float64 `json:"limit"`
	Violations []string `json:"violations"`
}

// CheckFractionSum 检查批次候选比例和不超过 1（全局不变量）。
// 返回违反的候选 ID 列表；空表示通过。
func (s *Service) CheckFractionSum(batchID int64) (*SumCheckResult, error) {
	cands, err := s.candStore.ListByBatch(batchID)
	if err != nil {
		return nil, err
	}
	sum := 0.0
	var viol []string
	for _, c := range cands {
		if c.Status == model.CandRejected {
			continue
		}
		sum += c.Fraction
		if sum > 1+1e-9 {
			viol = append(viol, fmt.Sprintf("候选 %d(%s) 使比例和达 %.4f", c.ID, c.Phase, sum))
		}
	}
	return &SumCheckResult{OK: len(viol) == 0, Sum: sum, Limit: 1.0, Violations: viol}, nil
}

// ConfirmAcceptable 复核者确认一个可接受候选：状态 → confirmed。
func (s *Service) ConfirmAcceptable(candID int64) (*model.PhaseCandidate, error) {
	c, err := s.candStore.Get(candID)
	if err != nil {
		return nil, err
	}
	if c.Status != model.CandAcceptable && c.Status != model.CandArbitration {
		return nil, fmt.Errorf("%w: 候选状态 %s 不可确认", model.ErrInvalidState, c.Status)
	}
	if err := s.candStore.Confirm(candID); err != nil {
		return nil, err
	}
	return s.candStore.Get(candID)
}

// RejectCandidate 复核者拒绝候选：状态 → rejected。
func (s *Service) RejectCandidate(candID int64, reason string) (*model.PhaseCandidate, error) {
	c, err := s.candStore.Get(candID)
	if err != nil {
		return nil, err
	}
	if c.Status == model.CandConfirmed || c.Status == model.CandRejected {
		return nil, fmt.Errorf("%w: 候选已定案", model.ErrInvalidState)
	}
	if err := s.candStore.UpdateStatus(candID, model.CandRejected); err != nil {
		return nil, err
	}
	return s.candStore.Get(candID)
}

// ListCandidates 列出批次候选（按比例降序）。
func (s *Service) ListCandidates(batchID int64) ([]*model.PhaseCandidate, error) {
	cands, err := s.candStore.ListByBatch(batchID)
	if err != nil {
		return nil, err
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].Fraction > cands[j].Fraction })
	return cands, nil
}

// GetCandidate 查询候选详情。
func (s *Service) GetCandidate(id int64) (*model.PhaseCandidate, error) {
	return s.candStore.Get(id)
}

// ConfirmedSummary 汇总已确认候选，供报告冻结使用。
type ConfirmedSummary struct {
	Confirmed []*model.PhaseCandidate `json:"confirmed"`
	Sum       float64                 `json:"sum"`
}

// Confirmed 返回批次已确认候选及比例和。
func (s *Service) Confirmed(batchID int64) (*ConfirmedSummary, error) {
	cands, err := s.candStore.ListByBatch(batchID)
	if err != nil {
		return nil, err
	}
	var conf []*model.PhaseCandidate
	sum := 0.0
	for _, c := range cands {
		if c.Status == model.CandConfirmed {
			conf = append(conf, c)
			sum += c.Fraction
		}
	}
	return &ConfirmedSummary{Confirmed: conf, Sum: sum}, nil
}

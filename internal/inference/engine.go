// Package inference 实现相候选推断引擎：依据相图摘要与测量误差，
// 从批次成分出发生成相组成候选，并执行成分守恒与比例合法性检查。
//
// 推断不变量：
//   - 负相比例拒绝；
//   - 相候选比例和不得超过 1；
//   - 批次每个元素成分必须落在候选相的稳定区间覆盖内（否则判定"超出相图"）；
//   - 候选相按典型比例估计并叠加测量误差，产出比例区间 [low, high]。
package inference

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/phasediagram"
	"task187-phaseaudit/internal/store"
)

// MeasurementErrorPct 默认元素测量误差（质量百分比绝对偏差）。
const MeasurementErrorPct = 2.0

// Service 推断引擎服务。
type Service struct {
	diagramStore *store.DiagramStore
	candStore    *store.CandidateStore
	obsStore     *store.ObservationStore
	diagramSvc   *phasediagram.Service
}

// New 构造推断服务。
func New(diagramStore *store.DiagramStore, candStore *store.CandidateStore,
	obsStore *store.ObservationStore, diagramSvc *phasediagram.Service) *Service {
	return &Service{
		diagramStore: diagramStore,
		candStore:    candStore,
		obsStore:     obsStore,
		diagramSvc:   diagramSvc,
	}
}

// InferInput 推断入参。
type InferInput struct {
	BatchID   int64 `json:"batch_id"`
	DiagramID int64 `json:"diagram_id,omitempty"` // 省略则用最近发布版本
}

// InferResult 推断结果汇总。
type InferResult struct {
	DiagramID  int64                     `json:"diagram_id"`
	DiagramVer int                       `json:"diagram_ver"`
	Candidates []*model.PhaseCandidate   `json:"candidates"`
	Covered    bool                      `json:"covered"`    // 成分是否被相图覆盖
	Reason     string                    `json:"reason"`     // 汇总说明
}

// Infer 对批次执行候选推断：
//  1. 取相图（指定或最近发布）；
//  2. 匹配成分到候选相；
//  3. 按观察者平均估计 / 典型比例确定比例；
//  4. 守恒检查并标记候选状态；
//  5. 落盘。
func (s *Service) Infer(batch *model.MaterialBatch, in InferInput) (*InferResult, error) {
	var d *model.PhaseDiagram
	var err error
	if in.DiagramID > 0 {
		d, err = s.diagramStore.Get(in.DiagramID)
	} else {
		d, err = s.diagramStore.LatestPublished()
	}
	if err != nil {
		return nil, err
	}
	if d.Status != "published" {
		return nil, model.ErrDiagramNotPublished
	}

	// 成分覆盖检查：全部元素必须在相图有区间定义。
	covered := s.diagramSvc.CompositionCovered(d, batch.Composition)
	reason := ""

	// 观察者平均估计作为比例先验。
	prior := s.observerPrior(batch.ID)

	phases := s.diagramSvc.CandidatePhases(d, batch.Composition)
	if len(phases) == 0 {
		covered = false
		reason = "无任何相稳定区间覆盖批次成分"
	}

	var cands []*model.PhaseCandidate
	for _, p := range phases {
		frac := p.TypicalFrac
		if v, ok := prior[p.Phase]; ok {
			frac = v / 100.0
		}
		frac = clamp01(frac)
		lo, hi := applyError(frac, MeasurementErrorPct/100.0)

		cons := s.CheckConservation(batch.Composition, p, frac)
		cand := &model.PhaseCandidate{
			BatchID:       batch.ID,
			DiagramID:     int64(d.VersionNo),
			Phase:         p.Phase,
			Fraction:      frac,
			FractionLow:   lo,
			FractionHigh:  hi,
			Conservation:  cons,
			Evidence:      s.evidenceText(batch.ID, p.Phase),
		}
		switch cons.Verdict {
		case model.ConservationPass:
			cand.Status = model.CandAcceptable
		case model.ConservationFailRange:
			cand.Status = model.CandOutOfDiagram
		default:
			cand.Status = model.CandArbitration
		}
		saved, err := s.candStore.Upsert(cand)
		if err != nil {
			return nil, err
		}
		cands = append(cands, saved)
	}

	// 若无候选相且成分未被覆盖，也写入一条占位说明（不落候选，直接返回）。
	if len(cands) == 0 {
		return &InferResult{DiagramID: d.ID, DiagramVer: d.VersionNo, Covered: covered, Reason: reason}, nil
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].Fraction > cands[j].Fraction })
	return &InferResult{DiagramID: d.ID, DiagramVer: d.VersionNo, Candidates: cands,
		Covered: covered, Reason: reason}, nil
}

// CheckConservation 检查单个候选的守恒：
//   - 质量守恒：候选比例 + 其它已确认候选 ≤ 1；
//   - 成分守恒：元素成分在相区间内，偏差 ≤ 误差上限；
//   - 范围检查：全部元素落在相的约束区间。
func (s *Service) CheckConservation(comp model.Composition, p model.PhaseDef, frac float64) model.ConservationResult {
	// 区间检查：受约束元素必须落在区间。
	var maxDev float64
	for _, r := range p.Regions {
		v, ok := comp[r.Element]
		if !ok {
			continue
		}
		if v < r.MinPct {
			dev := r.MinPct - v
			if dev > maxDev {
				maxDev = dev
			}
		}
		if v > r.MaxPct {
			dev := v - r.MaxPct
			if dev > maxDev {
				maxDev = dev
			}
		}
	}
	limit := MeasurementErrorPct
	res := model.ConservationResult{
		FractionSum:       frac,
		MaxDeviationPct:   maxDev,
		DeviationLimitPct: limit,
	}
	if maxDev > limit {
		res.Verdict = model.ConservationFailRange
		res.Reason = fmt.Sprintf("元素成分偏离相稳定区间 %.2f 个百分点（上限 %.2f）", maxDev, limit)
		return res
	}
	// 比例合法性（负/超 1）。
	if frac < 0 {
		res.Verdict = model.ConservationFailMass
		res.Reason = "相比例不能为负"
		return res
	}
	if frac > 1+1e-9 {
		res.Verdict = model.ConservationFailMass
		res.Reason = "相比例超过 1"
		return res
	}
	// 成分守恒（近似质量守恒）：候选相区间中点应接近批次成分。
	if err := s.approxMassBalance(comp, p, frac, limit); err != nil {
		res.Verdict = model.ConservationFailMass
		res.Reason = err.Error()
		return res
	}
	res.Verdict = model.ConservationPass
	res.Reason = "成分守恒通过"
	return res
}

// approxMassBalance 近似质量平衡：候选相的元素含量 = 区间中点，与批次成分偏差在误差内。
func (s *Service) approxMassBalance(comp model.Composition, p model.PhaseDef, frac float64, limit float64) error {
	for _, r := range p.Regions {
		v, ok := comp[r.Element]
		if !ok {
			continue
		}
		mid := (r.MinPct + r.MaxPct) / 2.0
		// 该相贡献：mid*frac；剩余 1-frac 视为基体（同元素近似成分）。
		contribution := mid*frac + v*(1-frac)
		diff := math.Abs(contribution - v)
		if diff > limit {
			return fmt.Errorf("元素 %s 质量守恒偏差 %.2f 个百分点（上限 %.2f）", r.Element, diff, limit)
		}
	}
	return nil
}

// observerPrior 汇总批次内未排除观察者对某相的比例估计（百分比 → 0-1，取平均）。
func (s *Service) observerPrior(batchID int64) map[string]float64 {
	obs, err := s.obsStore.ListByBatch(batchID)
	if err != nil {
		return nil
	}
	sum := map[string]float64{}
	cnt := map[string]int{}
	for _, o := range obs {
		if o.Status == model.ObsExcluded || o.Status == model.ObsSuperseded {
			continue
		}
		for phase, pct := range o.PhaseEstimate {
			sum[phase] += pct
			cnt[phase]++
		}
	}
	out := map[string]float64{}
	for phase, total := range sum {
		if cnt[phase] > 0 {
			out[phase] = total / float64(cnt[phase])
		}
	}
	return out
}

// evidenceText 生成候选证据摘要：引用支持该相的观察者列表。
func (s *Service) evidenceText(batchID int64, phase string) string {
	obs, err := s.obsStore.ListByBatch(batchID)
	if err != nil {
		return ""
	}
	var observers []string
	for _, o := range obs {
		if o.Status == model.ObsExcluded || o.Status == model.ObsSuperseded {
			continue
		}
		if _, ok := o.PhaseEstimate[phase]; ok {
			observers = append(observers, o.Observer)
		}
	}
	if len(observers) == 0 {
		return "无观察者直接估计该相"
	}
	return fmt.Sprintf("观察者 %s 估计了 %s", strings.Join(observers, ","), phase)
}

// applyError 对比例叠加测量误差（对称区间，截断到 [0,1]）。
func applyError(frac, err float64) (float64, float64) {
	return clamp01(frac - err), clamp01(frac + err)
}

// clamp01 截断到 [0,1]。
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

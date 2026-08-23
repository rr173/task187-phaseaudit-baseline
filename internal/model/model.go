// Package model 定义材料显微组织相鉴定证据复核台的核心实体、状态机常量与领域错误。
//
// 领域概念：
//   - MaterialBatch 材料批次：一份待鉴定合金，携带成分（元素质量百分比）与热历史。
//   - Observation 显微观察：某观察者对批次显微图像的特征记录与相比例估计。
//   - PhaseDiagram 相图版本：一组相及其成分稳定区间的权威摘要，用于推断相候选。
//   - PhaseCandidate 相候选：推断引擎给出的「某一相 + 比例区间」，受相图与守恒约束。
//   - Arbitration 仲裁：当候选推断触发守恒失败或观察者分歧时的裁决记录。
//   - MicroReport 组织报告：冻结后的批次微观组织结论，绑定相图与输入版本。
package model

import (
	"errors"
	"fmt"
)

// 材料批次状态机：待观察 → 待复核 → 相组成已定 / 证据不足 → 已封存。
const (
	BatchPendingObservation = "pending_observation" // 已登记成分与热历史，等待观察
	BatchPendingReview      = "pending_review"      // 观察齐备，候选推断完成，等待复核
	BatchCompositionFixed   = "composition_fixed"   // 相组成已确认，报告可发布
	BatchInsufficient       = "insufficient"        // 证据不足，需要补测
	BatchSealed             = "sealed"              // 已封存，禁止直接修改
)

// 观察状态机：待处理 → 支持 / 冲突 → 已排除。
const (
	ObsPending   = "pending"    // 待处理
	ObsSupport   = "support"    // 支持某候选
	ObsConflict  = "conflict"   // 与其他观察者分歧
	ObsExcluded  = "excluded"   // 已排除（重复/无效证据）
	ObsSuperseded = "superseded" // 已被新观察取代
)

// 相候选状态机：待推断 → 可接受 / 超出相图 / 待仲裁 → 已确认。
const (
	CandPending      = "pending"       // 待推断
	CandAcceptable   = "acceptable"    // 可接受（守恒通过）
	CandOutOfDiagram = "out_of_diagram" // 超出相图
	CandArbitration  = "arbitration"   // 待仲裁
	CandConfirmed    = "confirmed"     // 已确认
	CandRejected     = "rejected"      // 已拒绝
)

// 组织报告状态机：草案 → 待签核 → 已发布 → 已替代。
const (
	ReportDraft        = "draft"         // 草案
	ReportPendingSign  = "pending_sign"  // 待签核
	ReportPublished    = "published"     // 已发布
	ReportSuperseded   = "superseded"    // 已替代
)

// 仲裁状态机：打开 → 已裁决。
const (
	ArbOpen     = "open"     // 待仲裁
	ArbDecided  = "decided"  // 已裁决
	ArbDismissed = "dismissed" // 驳回（无需仲裁）
)

// 仲裁决定：确认候选 / 下调比例 / 拒绝候选。
const (
	DecisionConfirm   = "confirm"   // 确认候选
	DecisionDownscale = "downscale" // 下调候选比例
	DecisionReject    = "reject"    // 拒绝候选
)

// 守恒检查结论。
const (
	ConservationPass       = "pass"      // 成分守恒通过
	ConservationFailMass   = "mass"      // 质量守恒失败（比例和超出 1 或元素偏离）
	ConservationFailPhase  = "phase"     // 相图外成分
	ConservationFailRange  = "range"     // 元素成分落在全部相稳定区间之外
)

// 领域错误：每个错误与状态机约束一一对应，httpapi 层据此映射 HTTP 状态码。
var (
	ErrNegativeFraction      = errors.New("相比例不能为负")
	ErrFractionSumExceedsOne = errors.New("两个相候选比例和不能超过 1")
	ErrCompositionOutOfRange = errors.New("成分超出相图覆盖范围")
	ErrCrossBatchImage       = errors.New("禁止跨批次引用图像证据")
	ErrBatchSealed           = errors.New("已封存批次禁止直接修改")
	ErrReportVersionDrift    = errors.New("报告版本漂移：基线输入已变更，拒绝并发发布")
	ErrObservationConflict   = errors.New("观察者之间存在分歧，需先仲裁")
	ErrNotFound              = errors.New("资源不存在")
	ErrConflict              = errors.New("状态冲突")
	ErrInvalidState          = errors.New("非法状态流转")
	ErrDuplicateFingerprint  = errors.New("指纹重复（幂等返回既有记录）")
	ErrDiagramNotPublished   = errors.New("相图版本尚未发布")
	ErrReportSuperseded      = errors.New("报告已被替代")
)

// 元素成分：元素符号 → 质量百分比（百分比之和应约等于 100）。
type Composition map[string]float64

// HeatHistory 热历史：一次或多次热处理工序。
type HeatHistory struct {
	Steps []HeatStep `json:"steps"`
}

// HeatStep 单次热处理工序。
type HeatStep struct {
	Order      int     `json:"order"`
	Process    string  `json:"process"`    // 如 固溶/时效/淬火/回火
	TempC      float64 `json:"temp_c"`     // 摄氏温度
	DurationH  float64 `json:"duration_h"` // 小时
	Cooling    string  `json:"cooling"`    // 冷却方式
}

// MaterialBatch 材料批次。
type MaterialBatch struct {
	ID           int64       `json:"id"`
	Name         string      `json:"name"`
	Alloy        string      `json:"alloy"`        // 合金牌号/描述
	Composition  Composition `json:"composition"`  // 元素质量百分比
	HeatHistory  HeatHistory `json:"heat_history"` // 热历史
	Status       string      `json:"status"`
	Fingerprint  string      `json:"fingerprint"` // 成分+热历史指纹
	CreatedAt    string      `json:"created_at"`
	UpdatedAt    string      `json:"updated_at"`
	SealedAt     string      `json:"sealed_at,omitempty"`
}

// Observation 显微观察：一个观察者对一张显微图像的证据记录。
type Observation struct {
	ID             int64             `json:"id"`
	BatchID        int64             `json:"batch_id"`
	Observer       string            `json:"observer"`
	ImageRef       string            `json:"image_ref"`        // 图像标识
	FeatureNotes   string            `json:"feature_notes"`    // 特征描述（晶粒/夹杂/偏析）
	GrainSizeUM    float64           `json:"grain_size_um"`    // 平均晶粒尺寸 μm
	InclusionLevel float64           `json:"inclusion_level"`  // 夹杂物等级 0-5
	PhaseEstimate  map[string]float64 `json:"phase_estimate"`  // 观察者估计的各相比例
	Status         string            `json:"status"`
	Fingerprint    string            `json:"fingerprint"` // 图像+观察者+特征指纹
	CreatedAt      string            `json:"created_at"`
}

// PhaseRegion 相在元素空间中的稳定区间（质量百分比范围）。
type PhaseRegion struct {
	Element string  `json:"element"`
	MinPct  float64 `json:"min_pct"`
	MaxPct  float64 `json:"max_pct"`
}

// PhaseDef 相定义：一个相（如 α-Fe、Fe3C、γ）的成分稳定区间与典型比例。
type PhaseDef struct {
	Phase        string        `json:"phase"`
	Regions      []PhaseRegion `json:"regions"`       // 各元素区间
	TypicalFrac  float64       `json:"typical_frac"`  // 典型体积分数（参考）
	Notes        string        `json:"notes"`
}

// PhaseDiagram 相图版本：权威的成分-相区间摘要。
type PhaseDiagram struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	VersionNo   int        `json:"version_no"` // 版本号（同一名称递增）
	Status      string     `json:"status"`     // draft → published
	Phases      []PhaseDef `json:"phases"`
	Summary     string     `json:"summary"`
	CreatedAt   string     `json:"created_at"`
	PublishedAt string     `json:"published_at,omitempty"`
}

// ConservationResult 守恒检查结果。
type ConservationResult struct {
	Verdict        string  `json:"verdict"`         // pass/mass/phase/range
	Reason         string  `json:"reason"`          // 人类可读说明
	FractionSum    float64 `json:"fraction_sum"`    // 候选比例和
	MaxDeviationPct float64 `json:"max_deviation_pct"` // 元素成分最大偏差（百分点）
	DeviationLimitPct float64 `json:"deviation_limit_pct"` // 允许偏差上限
}

// PhaseCandidate 相候选：推断引擎产出的候选。
type PhaseCandidate struct {
	ID                 int64              `json:"id"`
	BatchID            int64              `json:"batch_id"`
	DiagramID          int64              `json:"diagram_id"`
	Phase              string             `json:"phase"`
	Fraction           float64            `json:"fraction"`
	FractionLow        float64            `json:"fraction_low"`  // 含测量误差的下界
	FractionHigh       float64            `json:"fraction_high"` // 含测量误差的上界
	Status             string             `json:"status"`
	Conservation       ConservationResult `json:"conservation"`
	Evidence           string             `json:"evidence"` // 支撑证据（观察 ID 列表摘要）
	InferredAt         string             `json:"inferred_at"`
	ConfirmedAt        string             `json:"confirmed_at,omitempty"`
}

// Arbitration 仲裁：守恒失败或观察分歧的裁决记录。
type Arbitration struct {
	ID          int64  `json:"id"`
	BatchID     int64  `json:"batch_id"`
	CandidateID int64  `json:"candidate_id"`
	Kind        string `json:"kind"` // observation_conflict / conservation_failure
	Reason      string `json:"reason"`
	Status      string `json:"status"`
	Decision    string `json:"decision,omitempty"`
	Note        string `json:"note,omitempty"`
	CreatedAt   string `json:"created_at"`
	DecidedAt   string `json:"decided_at,omitempty"`
}

// ReportInputVersion 报告输入版本：绑定相图版本与输入指纹，防止漂移。
type ReportInputVersion struct {
	DiagramID     int64  `json:"diagram_id"`
	DiagramVer    int    `json:"diagram_ver"`
	BatchFingerprint string `json:"batch_fingerprint"`
	ObsFingerprints []string `json:"obs_fingerprints"`
}

// MicroReport 组织报告：冻结后的批次微观组织结论。
type MicroReport struct {
	ID            int64             `json:"id"`
	BatchID       int64             `json:"batch_id"`
	Title         string            `json:"title"`
	Conclusion    string            `json:"conclusion"`
	Status        string            `json:"status"`
	InputVersion  ReportInputVersion `json:"input_version"`
	Snapshot      string            `json:"snapshot"` // 冻结的 JSON 快照
	CreatedAt     string            `json:"created_at"`
	PublishedAt   string            `json:"published_at,omitempty"`
	SupersededAt  string            `json:"superseded_at,omitempty"`
	SupersededBy  int64             `json:"superseded_by,omitempty"`
}

// VerifyConservation 校验候选比例合法性：负比例与比例和超过上限直接拒绝。
// 观察者估计使用百分比（上限 100），推断候选使用 0-1 比例（上限 1）。
func VerifyConservation(fractions map[string]float64, limit float64) error {
	sum := 0.0
	for phase, frac := range fractions {
		if frac < 0 {
			return fmt.Errorf("%w: %s=%v", ErrNegativeFraction, phase, frac)
		}
		sum += frac
	}
	if sum > limit+1e-9 {
		return fmt.Errorf("%w: sum=%v limit=%v", ErrFractionSumExceedsOne, sum, limit)
	}
	if limit < 0 {
		return fmt.Errorf("比例和上限不能为负: %v", limit)
	}
	return nil
}

// NormalizeComposition 将成分归一化到百分比总和，并返回总和。
func NormalizeComposition(c Composition) (Composition, float64) {
	total := 0.0
	for _, v := range c {
		total += v
	}
	if total <= 0 {
		return c, total
	}
	norm := make(Composition, len(c))
	for el, v := range c {
		norm[el] = v * 100.0 / total
	}
	return norm, total
}

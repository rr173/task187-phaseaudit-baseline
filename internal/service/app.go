// Package service 编排各业务模块，向 HTTP 层提供统一的应用门面。
package service

import (
	"fmt"

	"task187-phaseaudit/internal/arbitration"
	"task187-phaseaudit/internal/batch"
	"task187-phaseaudit/internal/inference"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/observation"
	"task187-phaseaudit/internal/phasediagram"
	"task187-phaseaudit/internal/report"
	"task187-phaseaudit/internal/store"
)

// App 应用门面：聚合全部业务模块服务。
type App struct {
	Batch       *batch.Service
	Observation *observation.Service
	Diagram     *phasediagram.Service
	Inference   *inference.Service
	Arbitration *arbitration.Service
	Report      *report.Service
}

// New 组装全部服务。
func New(db *store.DB) (*App, error) {
	batchStore := store.NewBatchStore(db)
	obsStore := store.NewObservationStore(db)
	diagStore := store.NewDiagramStore(db)
	candStore := store.NewCandidateStore(db)
	arbStore := store.NewArbitrationStore(db)
	repStore := store.NewReportStore(db)

	diagSvc := phasediagram.New(diagStore)
	batchSvc := batch.New(batchStore, obsStore)
	obsSvc := observation.New(obsStore, batchStore)
	infSvc := inference.New(diagStore, candStore, obsStore, diagSvc)
	arbSvc := arbitration.New(db, arbStore, candStore, batchStore)
	repSvc := report.New(repStore, batchStore, candStore, obsStore, diagStore)

	return &App{
		Batch:       batchSvc,
		Observation: obsSvc,
		Diagram:     diagSvc,
		Inference:   infSvc,
		Arbitration: arbSvc,
		Report:      repSvc,
	}, nil
}

// RegisterBatchFlow 一站式创建批次并登记首条观察（供演示与自检）。
func (a *App) RegisterBatchFlow(name, alloy string, comp model.Composition,
	hh model.HeatHistory, observer, imageRef, feature string, est map[string]float64) (*model.MaterialBatch, *model.Observation, error) {
	b, err := a.Batch.Create(batch.CreateInput{Name: name, Alloy: alloy, Composition: comp, HeatHistory: hh})
	if err != nil {
		return nil, nil, fmt.Errorf("create batch: %w", err)
	}
	o, err := a.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: observer, ImageRef: imageRef, FeatureNotes: feature,
		GrainSizeUM: 12.5, InclusionLevel: 1.2, PhaseEstimate: est,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create observation: %w", err)
	}
	return b, o, nil
}

// FullLoop 完整业务闭环（自检/冒烟使用）：
// 批次 → 观察 → 推断 → 复核/仲裁 → 发布报告 → 修订。
func (a *App) FullLoop(comp model.Composition, observer, imageRef string) (summary *LoopSummary, err error) {
	summary = &LoopSummary{}
	// 1. 建相图并发布。
	d, err := a.Diagram.Create(phasediagram.CreateInput{
		Name: "Fe-Cr-Ni 平衡相图",
		Phases: []model.PhaseDef{
			{Phase: "austenite", Regions: []model.PhaseRegion{
				{Element: "Fe", MinPct: 60, MaxPct: 95},
				{Element: "Cr", MinPct: 0, MaxPct: 20},
				{Element: "Ni", MinPct: 5, MaxPct: 30},
			}, TypicalFrac: 0.75, Notes: "γ-Fe 奥氏体"},
			{Phase: "ferrite", Regions: []model.PhaseRegion{
				{Element: "Fe", MinPct: 90, MaxPct: 100},
				{Element: "Cr", MinPct: 0, MaxPct: 10},
				{Element: "Ni", MinPct: 0, MaxPct: 5},
			}, TypicalFrac: 0.15, Notes: "α-Fe 铁素体"},
			{Phase: "carbide", Regions: []model.PhaseRegion{
				{Element: "Fe", MinPct: 60, MaxPct: 85},
				{Element: "Cr", MinPct: 5, MaxPct: 25},
				{Element: "Ni", MinPct: 0, MaxPct: 5},
			}, TypicalFrac: 0.1, Notes: "M23C6 碳化物"},
		},
		Summary: "1100°C 固溶 + 时效析出碳化物",
	})
	if err != nil {
		return nil, fmt.Errorf("create diagram: %w", err)
	}
	d, err = a.Diagram.Publish(d.ID)
	if err != nil {
		return nil, fmt.Errorf("publish diagram: %w", err)
	}
	summary.DiagramID = d.ID

	// 2. 批次。
	b, err := a.Batch.Create(batch.CreateInput{
		Name: "304L-批次A", Alloy: "304L",
		Composition: comp,
		HeatHistory: model.HeatHistory{Steps: []model.HeatStep{
			{Order: 1, Process: "固溶", TempC: 1100, DurationH: 1, Cooling: "水淬"},
			{Order: 2, Process: "时效", TempC: 650, DurationH: 8, Cooling: "空冷"},
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("create batch: %w", err)
	}
	summary.BatchID = b.ID

	// 3. 两条观察（一位观察者支持、另一位分歧 → 模拟冲突仲裁路径由调用方补充）。
	if _, err := a.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: observer, ImageRef: imageRef,
		FeatureNotes: "奥氏体基体 + 晶界碳化物", GrainSizeUM: 18, InclusionLevel: 1.5,
		PhaseEstimate: map[string]float64{"austenite": 72, "ferrite": 12, "carbide": 16},
	}); err != nil {
		return nil, fmt.Errorf("create obs: %w", err)
	}

	// 4. 推断。
	res, err := a.Inference.Infer(b, inference.InferInput{BatchID: b.ID, DiagramID: d.ID})
	if err != nil {
		return nil, fmt.Errorf("infer: %w", err)
	}
	summary.Candidates = len(res.Candidates)

	// 5. 确认所有可接受候选。
	confirmed := 0
	for _, c := range res.Candidates {
		if c.Status == model.CandAcceptable || c.Status == model.CandArbitration {
			if _, err := a.Inference.ConfirmAcceptable(c.ID); err != nil {
				// 仲裁候选暂不自动确认，留待仲裁。
				continue
			}
			confirmed++
		}
	}
	summary.Confirmed = confirmed
	if confirmed == 0 {
		return summary, fmt.Errorf("无候选可确认")
	}

	// 6. 批次 → composition_fixed（已确认候选 → 相组成已定）。
	if err := a.Batch.MoveToReview(b.ID); err != nil {
		return nil, fmt.Errorf("move to review: %w", err)
	}
	if err := a.markFixed(b.ID); err != nil {
		return nil, fmt.Errorf("mark fixed: %w", err)
	}

	// 7. 发布报告。
	rep, err := a.Report.Publish(report.PublishInput{
		BatchID: b.ID, Title: "304L-批次A 微观组织报告",
		Conclusion: "奥氏体为主，少量铁素体与晶界碳化物，符合固溶+时效预期",
	})
	if err != nil {
		return nil, fmt.Errorf("publish report: %w", err)
	}
	rep, err = a.Report.SignOff(rep.ID)
	if err != nil {
		return nil, fmt.Errorf("signoff report: %w", err)
	}
	summary.ReportID = rep.ID
	return summary, nil
}

// markFixed 将批次置为相组成已定（composition_fixed）。
func (a *App) markFixed(id int64) error {
	// 通过统一状态更新入口流转（复用 batch 模块的确认入口）。
	return a.Batch.ConfirmComposition(id)
}

// LoopSummary 闭环结果摘要。
type LoopSummary struct {
	DiagramID  int64 `json:"diagram_id"`
	BatchID    int64 `json:"batch_id"`
	Candidates int   `json:"candidates"`
	Confirmed  int   `json:"confirmed"`
	ReportID   int64 `json:"report_id"`
}

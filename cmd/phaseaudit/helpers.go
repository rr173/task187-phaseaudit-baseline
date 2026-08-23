package main

import (
	"task187-phaseaudit/internal/arbitration"
	"task187-phaseaudit/internal/batch"
	"task187-phaseaudit/internal/inference"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/observation"
	"task187-phaseaudit/internal/phasediagram"
	"task187-phaseaudit/internal/report"
)

// phaseDiagramInput 冒烟使用的 Fe-Cr-Ni 相图定义（三相交替覆盖）。
func phaseDiagramInput() phasediagram.CreateInput {
	return phasediagram.CreateInput{
		Name: "Fe-Cr-Ni 平衡相图",
		Phases: []model.PhaseDef{
			{Phase: "austenite", Regions: []model.PhaseRegion{
				{Element: "Fe", MinPct: 55, MaxPct: 90},
				{Element: "Cr", MinPct: 0, MaxPct: 22},
				{Element: "Ni", MinPct: 5, MaxPct: 30},
			}, TypicalFrac: 0.7, Notes: "γ-Fe 奥氏体"},
			{Phase: "ferrite", Regions: []model.PhaseRegion{
				{Element: "Fe", MinPct: 90, MaxPct: 100},
				{Element: "Cr", MinPct: 0, MaxPct: 10},
				{Element: "Ni", MinPct: 0, MaxPct: 5},
			}, TypicalFrac: 0.15, Notes: "α-Fe 铁素体"},
			{Phase: "carbide", Regions: []model.PhaseRegion{
				{Element: "Fe", MinPct: 55, MaxPct: 80},
				{Element: "Cr", MinPct: 5, MaxPct: 30},
				{Element: "Ni", MinPct: 0, MaxPct: 5},
			}, TypicalFrac: 0.15, Notes: "M23C6 碳化物"},
		},
		Summary: "1100°C 固溶 + 650°C 时效",
	}
}

// serviceBatchInput 冒烟批次入参（含热历史）。
func serviceBatchInput(name string, comp model.Composition) batch.CreateInput {
	return batch.CreateInput{
		Name:        name,
		Alloy:       "316L",
		Composition: comp,
		HeatHistory: model.HeatHistory{Steps: []model.HeatStep{
			{Order: 1, Process: "固溶", TempC: 1100, DurationH: 1, Cooling: "水淬"},
			{Order: 2, Process: "时效", TempC: 650, DurationH: 8, Cooling: "空冷"},
		}},
	}
}

// observationInput 冒烟观察入参。
func observationInput(batchID int64, observer, imageRef string, est map[string]float64) observation.CreateInput {
	return observation.CreateInput{
		BatchID:        batchID,
		Observer:       observer,
		ImageRef:       imageRef,
		FeatureNotes:   "奥氏体基体 + 晶界碳化物",
		GrainSizeUM:    16,
		InclusionLevel: 1.4,
		PhaseEstimate:  est,
	}
}

// inferInput 推断入参。
func inferInput(diagramID, batchID int64) inference.InferInput {
	return inference.InferInput{BatchID: batchID, DiagramID: diagramID}
}

// arbInput 仲裁打开入参。
func arbInput(batchID, candidateID int64) arbitration.OpenInput {
	return arbitration.OpenInput{
		BatchID:     batchID,
		CandidateID: candidateID,
		Kind:        arbitration.KindObservationConflict,
		Reason:      "观察员对奥氏体比例估计分歧超过 5 个百分点",
	}
}

// decideInputConfirm 仲裁确认决定入参。
func decideInputConfirm() arbitration.DecideInput {
	return arbitration.DecideInput{Decision: model.DecisionConfirm, Note: "复核确认，成分与相图一致"}
}

// reportInput 报告发布入参。
func reportInput(batchID int64, title, conclusion string) report.PublishInput {
	return report.PublishInput{BatchID: batchID, Title: title, Conclusion: conclusion}
}

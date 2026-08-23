package report_test

import (
	"errors"
	"testing"

	"task187-phaseaudit/internal/batch"
	"task187-phaseaudit/internal/inference"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/observation"
	"task187-phaseaudit/internal/phasediagram"
	"task187-phaseaudit/internal/report"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func TestTask187Bug09_NewObservationInvalidatesReportRevision(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	app, err := service.New(db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	d, err := app.Diagram.Create(phasediagram.CreateInput{
		Name: "输入版本相图",
		Phases: []model.PhaseDef{{Phase: "alpha", TypicalFrac: 1, Regions: []model.PhaseRegion{{Element: "Fe", MinPct: 90, MaxPct: 100}}}},
	})
	if err != nil {
		t.Fatalf("create diagram: %v", err)
	}
	if _, err := app.Diagram.Publish(d.ID); err != nil {
		t.Fatalf("publish diagram: %v", err)
	}
	b, err := app.Batch.Create(batch.CreateInput{Name: "输入漂移批次", Composition: model.Composition{"Fe": 100}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if _, err := app.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: "观察员甲", ImageRef: "drift-image-1",
		PhaseEstimate: map[string]float64{"alpha": 100},
	}); err != nil {
		t.Fatalf("create first observation: %v", err)
	}
	res, err := app.Inference.Infer(b, inference.InferInput{BatchID: b.ID, DiagramID: d.ID})
	if err != nil || len(res.Candidates) != 1 {
		t.Fatalf("infer candidates=%d err=%v", len(res.Candidates), err)
	}
	if _, err := app.Inference.ConfirmAcceptable(res.Candidates[0].ID); err != nil {
		t.Fatalf("confirm candidate: %v", err)
	}
	if err := app.Batch.MoveToReview(b.ID); err != nil {
		t.Fatalf("move to review: %v", err)
	}
	if err := app.Batch.ConfirmComposition(b.ID); err != nil {
		t.Fatalf("confirm composition: %v", err)
	}
	rep, err := app.Report.Publish(report.PublishInput{BatchID: b.ID, Title: "输入快照报告", Conclusion: "初始结论"})
	if err != nil {
		t.Fatalf("publish report: %v", err)
	}
	if _, err := app.Report.SignOff(rep.ID); err != nil {
		t.Fatalf("sign off report: %v", err)
	}
	if _, err := app.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: "观察员乙", ImageRef: "drift-image-2",
		PhaseEstimate: map[string]float64{"alpha": 100},
	}); err != nil {
		t.Fatalf("create new observation: %v", err)
	}
	_, err = app.Report.Revise(rep.ID, "修订结论", "不应允许直接修订")
	if !errors.Is(err, model.ErrReportVersionDrift) {
		t.Fatalf("revise error=%v, want report version drift", err)
	}
}

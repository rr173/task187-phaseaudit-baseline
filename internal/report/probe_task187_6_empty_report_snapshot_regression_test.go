package report_test

import (
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

func TestTask187Bug06_RejectOnlyBatchReturnsReportError(t *testing.T) {
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
		Name: "空快照相图",
		Phases: []model.PhaseDef{{Phase: "alpha", TypicalFrac: 0.6, Regions: []model.PhaseRegion{{Element: "Fe", MinPct: 90, MaxPct: 100}}}},
	})
	if err != nil {
		t.Fatalf("create diagram: %v", err)
	}
	if _, err := app.Diagram.Publish(d.ID); err != nil {
		t.Fatalf("publish diagram: %v", err)
	}
	b, err := app.Batch.Create(batch.CreateInput{Name: "仅拒绝候选批次", Composition: model.Composition{"Fe": 100}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if _, err := app.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: "复核员", ImageRef: "empty-report-image",
		PhaseEstimate: map[string]float64{"alpha": 100},
	}); err != nil {
		t.Fatalf("create observation: %v", err)
	}
	res, err := app.Inference.Infer(b, inference.InferInput{BatchID: b.ID, DiagramID: d.ID})
	if err != nil || len(res.Candidates) != 1 {
		t.Fatalf("infer candidates=%d err=%v", len(res.Candidates), err)
	}
	if _, err := app.Inference.RejectCandidate(res.Candidates[0].ID, "证据不足"); err != nil {
		t.Fatalf("reject candidate: %v", err)
	}
	if err := app.Batch.MoveToReview(b.ID); err != nil {
		t.Fatalf("move to review: %v", err)
	}
	if err := app.Batch.ConfirmComposition(b.ID); err != nil {
		t.Fatalf("confirm composition: %v", err)
	}
	_, err = app.Report.Publish(report.PublishInput{BatchID: b.ID, Title: "空候选报告", Conclusion: "应拒绝发布"})
	if err == nil {
		t.Fatal("publish report unexpectedly succeeded without confirmed candidates")
	}
}

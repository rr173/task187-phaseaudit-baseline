package arbitration_test

import (
	"testing"

	"task187-phaseaudit/internal/arbitration"
	"task187-phaseaudit/internal/batch"
	"task187-phaseaudit/internal/inference"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/phasediagram"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func newArbitrationApp(t *testing.T) *service.App {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app, err := service.New(db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	return app
}

func TestRejectDecisionClosesArbitrationAndMarksBatchInsufficient(t *testing.T) {
	app := newArbitrationApp(t)
	d, err := app.Diagram.Create(phasediagram.CreateInput{
		Name: "仲裁相图",
		Phases: []model.PhaseDef{{Phase: "phase-a", TypicalFrac: 1, Regions: []model.PhaseRegion{
			{Element: "Fe", MinPct: 60, MaxPct: 70},
			{Element: "Cr", MinPct: 30, MaxPct: 48},
		}}},
	})
	if err != nil {
		t.Fatalf("create diagram: %v", err)
	}
	if _, err := app.Diagram.Publish(d.ID); err != nil {
		t.Fatalf("publish diagram: %v", err)
	}
	b, err := app.Batch.Create(batch.CreateInput{Name: "待仲裁批次", Composition: model.Composition{"Fe": 61, "Cr": 39}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	res, err := app.Inference.Infer(b, inference.InferInput{BatchID: b.ID, DiagramID: d.ID})
	if err != nil || len(res.Candidates) != 1 {
		t.Fatalf("infer candidates=%d err=%v", len(res.Candidates), err)
	}
	c := res.Candidates[0]
	if c.Status != model.CandArbitration {
		t.Fatalf("candidate status=%q, want arbitration", c.Status)
	}
	a, err := app.Arbitration.Open(arbitration.OpenInput{
		BatchID: b.ID, CandidateID: c.ID, Kind: arbitration.KindConservationFailure, Reason: "质量守恒偏差需要复核",
	})
	if err != nil {
		t.Fatalf("open arbitration: %v", err)
	}
	if _, err := app.Arbitration.Decide(a.ID, arbitration.DecideInput{Decision: model.DecisionReject, Note: "证据不足"}); err != nil {
		t.Fatalf("reject arbitration: %v", err)
	}
	gotCandidate, err := app.Inference.GetCandidate(c.ID)
	if err != nil {
		t.Fatalf("get candidate: %v", err)
	}
	if gotCandidate.Status != model.CandRejected {
		t.Fatalf("candidate status=%q, want rejected", gotCandidate.Status)
	}
	gotBatch, err := app.Batch.Get(b.ID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if gotBatch.Status != model.BatchInsufficient {
		t.Fatalf("batch status=%q, want insufficient", gotBatch.Status)
	}
}

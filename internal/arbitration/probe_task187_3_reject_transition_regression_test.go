package arbitration_test

import (
	"testing"

	"task187-phaseaudit/internal/arbitration"
	"task187-phaseaudit/internal/batch"
	"task187-phaseaudit/internal/inference"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/observation"
	"task187-phaseaudit/internal/phasediagram"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func TestTask187Bug03_RejectDecisionPersistsBothStatuses(t *testing.T) {
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
		Name: "仲裁拒绝相图",
		Phases: []model.PhaseDef{{Phase: "alpha", TypicalFrac: 0.8,
			Regions: []model.PhaseRegion{{Element: "Fe", MinPct: 90, MaxPct: 100}}}},
	})
	if err != nil {
		t.Fatalf("create diagram: %v", err)
	}
	if _, err := app.Diagram.Publish(d.ID); err != nil {
		t.Fatalf("publish diagram: %v", err)
	}
	b, err := app.Batch.Create(batch.CreateInput{Name: "仲裁拒绝批次", Composition: model.Composition{"Fe": 100}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if _, err := app.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: "仲裁观察员", ImageRef: "reject-image",
		PhaseEstimate: map[string]float64{"alpha": 100},
	}); err != nil {
		t.Fatalf("create observation: %v", err)
	}
	if _, err := app.Inference.Infer(b, inference.InferInput{BatchID: b.ID, DiagramID: d.ID}); err != nil {
		t.Fatalf("infer: %v", err)
	}
	candidates, err := app.Inference.ListCandidates(b.ID)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("candidates=%v err=%v", candidates, err)
	}
	if candidates[0].Status != model.CandArbitration {
		t.Fatalf("candidate status=%q, want %q", candidates[0].Status, model.CandArbitration)
	}
	a, err := app.Arbitration.Open(arbitration.OpenInput{
		BatchID: b.ID, CandidateID: candidates[0].ID,
		Kind: arbitration.KindConservationFailure, Reason: "质量守恒未通过",
	})
	if err != nil {
		t.Fatalf("open arbitration: %v", err)
	}
	if _, err := app.Arbitration.Decide(a.ID, arbitration.DecideInput{Decision: model.DecisionReject, Note: "拒绝该候选"}); err != nil {
		t.Fatalf("decide rejection: %v", err)
	}
	gotCandidate, err := app.Inference.GetCandidate(candidates[0].ID)
	if err != nil {
		t.Fatalf("get candidate: %v", err)
	}
	gotBatch, err := app.Batch.Get(b.ID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if gotCandidate.Status != model.CandRejected || gotBatch.Status != model.BatchInsufficient {
		t.Fatalf("candidate=%q batch=%q, want candidate=%q batch=%q", gotCandidate.Status, gotBatch.Status, model.CandRejected, model.BatchInsufficient)
	}
}


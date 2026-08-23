package inference_test

import (
	"math"
	"testing"

	"task187-phaseaudit/internal/batch"
	"task187-phaseaudit/internal/inference"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/observation"
	"task187-phaseaudit/internal/phasediagram"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func TestTask187Bug10_ExcludedObservationNotReused(t *testing.T) {
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
		Name: "排除证据相图",
		Phases: []model.PhaseDef{{Phase: "alpha", TypicalFrac: 0.4, Regions: []model.PhaseRegion{{Element: "Fe", MinPct: 90, MaxPct: 100}}}},
	})
	if err != nil {
		t.Fatalf("create diagram: %v", err)
	}
	if _, err := app.Diagram.Publish(d.ID); err != nil {
		t.Fatalf("publish diagram: %v", err)
	}
	b, err := app.Batch.Create(batch.CreateInput{Name: "排除证据批次", Composition: model.Composition{"Fe": 100}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	o, err := app.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: "待排除观察员", ImageRef: "excluded-image",
		PhaseEstimate: map[string]float64{"alpha": 80},
	})
	if err != nil {
		t.Fatalf("create observation: %v", err)
	}
	first, err := app.Inference.Infer(b, inference.InferInput{BatchID: b.ID, DiagramID: d.ID})
	if err != nil || len(first.Candidates) != 1 {
		t.Fatalf("first infer candidates=%d err=%v", len(first.Candidates), err)
	}
	if math.Abs(first.Candidates[0].Fraction-0.8) > 1e-9 {
		t.Fatalf("first fraction=%v, want 0.8", first.Candidates[0].Fraction)
	}
	if err := app.Observation.Exclude(o.ID); err != nil {
		t.Fatalf("exclude observation: %v", err)
	}
	second, err := app.Inference.Infer(b, inference.InferInput{BatchID: b.ID, DiagramID: d.ID})
	if err != nil || len(second.Candidates) != 1 {
		t.Fatalf("second infer candidates=%d err=%v", len(second.Candidates), err)
	}
	if math.Abs(second.Candidates[0].Fraction-0.4) > 1e-9 {
		t.Fatalf("excluded observation still affected fraction=%v, want 0.4", second.Candidates[0].Fraction)
	}
	if second.Candidates[0].Evidence != "无观察者直接估计该相" {
		t.Fatalf("excluded observation still appeared in evidence=%q", second.Candidates[0].Evidence)
	}
}

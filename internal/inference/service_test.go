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

func newInferenceApp(t *testing.T) *service.App {
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

func TestInferUsesObserverPriorAndMeasurementInterval(t *testing.T) {
	app := newInferenceApp(t)
	d, err := app.Diagram.Create(phasediagram.CreateInput{
		Name: "推断相图",
		Phases: []model.PhaseDef{{Phase: "austenite", TypicalFrac: 0.4, Regions: []model.PhaseRegion{
			{Element: "Fe", MinPct: 68, MaxPct: 72},
			{Element: "Cr", MinPct: 28, MaxPct: 32},
		}}},
	})
	if err != nil {
		t.Fatalf("create diagram: %v", err)
	}
	if _, err := app.Diagram.Publish(d.ID); err != nil {
		t.Fatalf("publish diagram: %v", err)
	}
	b, err := app.Batch.Create(batch.CreateInput{Name: "推断批次", Composition: model.Composition{"Fe": 70, "Cr": 30}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if _, err := app.Observation.Create(observationInput(b.ID)); err != nil {
		t.Fatalf("create observation: %v", err)
	}
	res, err := app.Inference.Infer(b, inference.InferInput{BatchID: b.ID, DiagramID: d.ID})
	if err != nil {
		t.Fatalf("infer: %v", err)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("candidates=%d, want 1", len(res.Candidates))
	}
	c := res.Candidates[0]
	if math.Abs(c.Fraction-0.6) > 1e-9 || math.Abs(c.FractionLow-0.58) > 1e-9 || math.Abs(c.FractionHigh-0.62) > 1e-9 {
		t.Fatalf("candidate interval=%+v", c)
	}
	if c.Status != model.CandAcceptable || c.Conservation.Verdict != model.ConservationPass {
		t.Fatalf("candidate status=%q conservation=%q", c.Status, c.Conservation.Verdict)
	}
}

func observationInput(batchID int64) observation.CreateInput {
	return observation.CreateInput{
		BatchID: batchID, Observer: "观察员", ImageRef: "image-infer",
		PhaseEstimate: map[string]float64{"austenite": 60, "ferrite": 40},
	}
}

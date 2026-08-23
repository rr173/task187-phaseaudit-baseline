package observation_test

import (
	"testing"

	"task187-phaseaudit/internal/batch"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/observation"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func newObservationApp(t *testing.T) *service.App {
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

func TestResolveStatusMarksDivergenceAndProtectsConflictEvidence(t *testing.T) {
	app := newObservationApp(t)
	b, err := app.Batch.Create(batch.CreateInput{Name: "观察批次", Composition: model.Composition{"Fe": 100}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	base := observation.CreateInput{BatchID: b.ID, Observer: "甲", ImageRef: "img-a", PhaseEstimate: map[string]float64{"austenite": 70, "ferrite": 30}}
	if _, err := app.Observation.Create(base); err != nil {
		t.Fatalf("create first observation: %v", err)
	}
	second := base
	second.Observer = "乙"
	second.ImageRef = "img-b"
	second.PhaseEstimate = map[string]float64{"austenite": 55, "ferrite": 45}
	created, err := app.Observation.Create(second)
	if err != nil {
		t.Fatalf("create second observation: %v", err)
	}
	if err := app.Observation.ResolveStatus(b.ID); err != nil {
		t.Fatalf("resolve status: %v", err)
	}
	diverged, err := app.Observation.Divergence(b.ID)
	if err != nil || !diverged {
		t.Fatalf("divergence=%v err=%v, want true", diverged, err)
	}
	got, err := app.Observation.Get(created.ID)
	if err != nil {
		t.Fatalf("get observation: %v", err)
	}
	if got.Status != model.ObsConflict {
		t.Fatalf("status=%q, want %q", got.Status, model.ObsConflict)
	}
	if err := app.Observation.Exclude(created.ID); err == nil {
		t.Fatal("exclude conflict unexpectedly succeeded")
	}
}

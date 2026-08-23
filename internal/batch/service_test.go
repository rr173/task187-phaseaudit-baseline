package batch_test

import (
	"math"
	"testing"

	"task187-phaseaudit/internal/batch"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func newBatchApp(t *testing.T) *service.App {
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

func TestCreateNormalizesCompositionAndCompletesLifecycle(t *testing.T) {
	app := newBatchApp(t)
	in := batch.CreateInput{
		Name:        "热处理批次",
		Alloy:       "316L",
		Composition: model.Composition{"Fe": 68, "Cr": 17, "Ni": 12},
		HeatHistory: model.HeatHistory{Steps: []model.HeatStep{
			{Order: 1, Process: "固溶", TempC: 1100, DurationH: 1, Cooling: "水淬"},
		}},
	}
	b, err := app.Batch.Create(in)
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	total := 0.0
	for _, value := range b.Composition {
		total += value
	}
	if math.Abs(total-100) > 1e-9 {
		t.Fatalf("composition was not normalized: %.6f", total)
	}
	if b.Status != model.BatchPendingObservation {
		t.Fatalf("initial status=%q", b.Status)
	}
	if err := app.Batch.MoveToReview(b.ID); err != nil {
		t.Fatalf("move to review: %v", err)
	}
	if err := app.Batch.ConfirmComposition(b.ID); err != nil {
		t.Fatalf("confirm composition: %v", err)
	}
	if err := app.Batch.Seal(b.ID); err != nil {
		t.Fatalf("seal batch: %v", err)
	}
	if err := app.Batch.Seal(b.ID); err != nil {
		t.Fatalf("seal should be idempotent: %v", err)
	}
	got, err := app.Batch.Get(b.ID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if got.Status != model.BatchSealed {
		t.Fatalf("final status=%q, want %q", got.Status, model.BatchSealed)
	}
}

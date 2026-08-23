package inference_test

import (
	"errors"
	"testing"

	"task187-phaseaudit/internal/batch"
	"task187-phaseaudit/internal/inference"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func TestTask187Bug07_MissingDiagramReturnsNotFound(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	app, err := service.New(db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	b, err := app.Batch.Create(batch.CreateInput{Name: "缺失相图批次", Composition: model.Composition{"Fe": 100}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	_, err = app.Inference.Infer(b, inference.InferInput{BatchID: b.ID, DiagramID: 99999})
	if err == nil {
		t.Fatal("infer unexpectedly succeeded with a missing diagram")
	}
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("error=%v, want model.ErrNotFound", err)
	}
}

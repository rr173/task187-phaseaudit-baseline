package inference_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"task187-phaseaudit/internal/batch"
	"task187-phaseaudit/internal/httpapi"
	"task187-phaseaudit/internal/inference"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/observation"
	"task187-phaseaudit/internal/phasediagram"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func TestTask187Bug02_CandidateKeepsPublishedDiagramIdentity(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	app, err := service.New(db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	seed := diagramInput("unrelated-diagram")
	first, err := app.Diagram.Create(seed)
	if err != nil {
		t.Fatalf("create first diagram: %v", err)
	}
	if _, err := app.Diagram.Publish(first.ID); err != nil {
		t.Fatalf("publish first diagram: %v", err)
	}
	target, err := app.Diagram.Create(diagramInput("target-diagram"))
	if err != nil {
		t.Fatalf("create target diagram: %v", err)
	}
	if _, err := app.Diagram.Publish(target.ID); err != nil {
		t.Fatalf("publish target diagram: %v", err)
	}
	b, err := app.Batch.Create(batch.CreateInput{Name: "候选身份批次", Composition: model.Composition{"Fe": 100}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if _, err := app.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: "观察员", ImageRef: "candidate-identity-image",
		PhaseEstimate: map[string]float64{"alpha": 100},
	}); err != nil {
		t.Fatalf("create observation: %v", err)
	}
	if _, err := app.Inference.Infer(b, inference.InferInput{BatchID: b.ID, DiagramID: target.ID}); err != nil {
		t.Fatalf("infer: %v", err)
	}

	ts := httptest.NewServer(httpapi.New(app).Handler())
	defer ts.Close()
	resp, err := http.Get(fmt.Sprintf("%s/api/batches/%d/candidates", ts.URL, b.ID))
	if err != nil {
		t.Fatalf("get candidates: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusOK)
	}
	var candidates []struct {
		DiagramID int64 `json:"diagram_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&candidates); err != nil {
		t.Fatalf("decode candidates: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected at least one candidate")
	}
	for _, candidate := range candidates {
		if candidate.DiagramID != target.ID {
			t.Fatalf("diagram_id=%d, want published diagram %d", candidate.DiagramID, target.ID)
		}
	}
}

func diagramInput(name string) phasediagram.CreateInput {
	return phasediagram.CreateInput{
		Name: name,
		Phases: []model.PhaseDef{{
			Phase: "alpha", TypicalFrac: 0.8,
			Regions: []model.PhaseRegion{{Element: "Fe", MinPct: 90, MaxPct: 100}},
		}},
	}
}


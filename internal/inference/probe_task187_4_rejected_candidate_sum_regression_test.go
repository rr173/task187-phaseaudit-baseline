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

func TestTask187Bug04_RejectedCandidateExcludedFromFractionSum(t *testing.T) {
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
		Name: "比例和相图",
		Phases: []model.PhaseDef{
			{Phase: "alpha", TypicalFrac: 0.6, Regions: []model.PhaseRegion{{Element: "Fe", MinPct: 90, MaxPct: 100}}},
			{Phase: "beta", TypicalFrac: 0.3, Regions: []model.PhaseRegion{{Element: "Fe", MinPct: 90, MaxPct: 100}}},
		},
	})
	if err != nil {
		t.Fatalf("create diagram: %v", err)
	}
	if _, err := app.Diagram.Publish(d.ID); err != nil {
		t.Fatalf("publish diagram: %v", err)
	}
	b, err := app.Batch.Create(batch.CreateInput{Name: "比例和批次", Composition: model.Composition{"Fe": 100}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if _, err := app.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: "比例观察员", ImageRef: "fraction-sum-image",
		PhaseEstimate: map[string]float64{"alpha": 60, "beta": 30},
	}); err != nil {
		t.Fatalf("create observation: %v", err)
	}
	result, err := app.Inference.Infer(b, inference.InferInput{BatchID: b.ID, DiagramID: d.ID})
	if err != nil || len(result.Candidates) != 2 {
		t.Fatalf("infer candidates=%v err=%v", result.Candidates, err)
	}
	if _, err := app.Inference.RejectCandidate(result.Candidates[0].ID, "复核排除"); err != nil {
		t.Fatalf("reject candidate: %v", err)
	}
	ts := httptest.NewServer(httpapi.New(app).Handler())
	defer ts.Close()
	resp, err := http.Get(fmt.Sprintf("%s/api/batches/%d/fractionsum", ts.URL, b.ID))
	if err != nil {
		t.Fatalf("get fraction sum: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusOK)
	}
	var got struct {
		Sum float64 `json:"sum"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode fraction sum: %v", err)
	}
	if got.Sum < 0.29 || got.Sum > 0.31 {
		t.Fatalf("sum=%.4f, want remaining candidate sum 0.30", got.Sum)
	}
}


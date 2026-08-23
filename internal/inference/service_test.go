package inference_test

import (
	"math"
	"strings"
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

// TestExcludedObservationDoesNotInfluenceInfer 被复核者明确排除的观察：
//   - 不再参与相比例先验（不把比例推向被排除观察的估计）；
//   - 不出现在候选证据摘要的观察者列表中。
// 同时保持未排除观察的先验统计语义不变。
func TestExcludedObservationDoesNotInfluenceInfer(t *testing.T) {
	app := newInferenceApp(t)
	d, err := app.Diagram.Create(phasediagram.CreateInput{
		Name: "排除相图",
		Phases: []model.PhaseDef{{Phase: "austenite", TypicalFrac: 0.3, Regions: []model.PhaseRegion{
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
	b, err := app.Batch.Create(batch.CreateInput{Name: "排除批次", Composition: model.Composition{"Fe": 70, "Cr": 30}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	// 观察者A 估计 60%，观察者B（高偏离）估计 80% —— 先验平均应为 70%（0.70）。
	if _, err := app.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: "obsA", ImageRef: "img-a",
		PhaseEstimate: map[string]float64{"austenite": 60},
	}); err != nil {
		t.Fatalf("create obsA: %v", err)
	}
	if _, err := app.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: "obsB", ImageRef: "img-b",
		PhaseEstimate: map[string]float64{"austenite": 80},
	}); err != nil {
		t.Fatalf("create obsB: %v", err)
	}

	// 首次推断：先验 = 平均(60,80) = 70%。
	first, err := app.Inference.Infer(b, inference.InferInput{BatchID: b.ID, DiagramID: d.ID})
	if err != nil {
		t.Fatalf("first infer: %v", err)
	}
	if len(first.Candidates) != 1 {
		t.Fatalf("first candidates=%d, want 1", len(first.Candidates))
	}
	if math.Abs(first.Candidates[0].Fraction-0.70) > 1e-9 {
		t.Fatalf("first fraction=%.4f, want 0.70 (prior avg of 60%% & 80%%)", first.Candidates[0].Fraction)
	}
	if !strings.Contains(first.Candidates[0].Evidence, "obsA") || !strings.Contains(first.Candidates[0].Evidence, "obsB") {
		t.Fatalf("first evidence should list both observers, got %q", first.Candidates[0].Evidence)
	}

	// 排除观察者B（高偏离）后重新推断：先验应回落到 60%（0.60），不再被 B 拉到 0.80。
	excluded, _ := listObsByObserver(app, b.ID, "obsB")
	if excluded == nil {
		t.Fatalf("obsB not found")
	}
	if err := app.Observation.Exclude(excluded.ID); err != nil {
		t.Fatalf("exclude obsB: %v", err)
	}

	second, err := app.Inference.Infer(b, inference.InferInput{BatchID: b.ID, DiagramID: d.ID})
	if err != nil {
		t.Fatalf("second infer: %v", err)
	}
	if len(second.Candidates) != 1 {
		t.Fatalf("second candidates=%d, want 1", len(second.Candidates))
	}
	c := second.Candidates[0]
	// 排除 obsB 后先验只剩 obsA(60%) → 0.60；若仍计入 obsB 则会停在 0.80 或回落到 0.70。
	if math.Abs(c.Fraction-0.60) > 1e-9 {
		t.Fatalf("excluded observation still influences prior: fraction=%.4f, want 0.60", c.Fraction)
	}
	if strings.Contains(c.Evidence, "obsB") {
		t.Fatalf("excluded observer still in evidence: %q", c.Evidence)
	}
	if !strings.Contains(c.Evidence, "obsA") {
		t.Fatalf("active observer missing from evidence: %q", c.Evidence)
	}
}

func listObsByObserver(app *service.App, batchID int64, observer string) (*model.Observation, error) {
	obs, err := app.Observation.ListByBatch(batchID)
	if err != nil {
		return nil, err
	}
	for _, o := range obs {
		if o.Observer == observer {
			return o, nil
		}
	}
	return nil, nil
}

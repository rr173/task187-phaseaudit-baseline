package service

import (
	"testing"

	"task187-phaseaudit/internal/batch"
	"task187-phaseaudit/internal/inference"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/observation"
	"task187-phaseaudit/internal/report"
)

// inferTestInput 推断入参。
func inferTestInput(batchID int64) inference.InferInput {
	return inference.InferInput{BatchID: batchID}
}

// TestBatchFingerprintIdempotent 验证同成分+热历史批次指纹幂等。
func TestBatchFingerprintIdempotent(t *testing.T) {
	app := newTestApp(t)
	comp := model.Composition{"Fe": 68, "Cr": 17, "Ni": 12}
	in := batch.CreateInput{Name: "批次X", Alloy: "316L", Composition: comp,
		HeatHistory: model.HeatHistory{Steps: []model.HeatStep{
			{Order: 1, Process: "固溶", TempC: 1100, DurationH: 1, Cooling: "水淬"},
		}}}
	b1, err := app.Batch.Create(in)
	if err != nil {
		t.Fatalf("create b1: %v", err)
	}
	b2, err := app.Batch.Create(in)
	if err != nil {
		t.Fatalf("create b2: %v", err)
	}
	if b1.ID != b2.ID {
		t.Fatalf("fingerprint dedup failed: %d != %d", b1.ID, b2.ID)
	}
}

// TestNegativeFractionRejected 拒绝负相比例。
func TestNegativeFractionRejected(t *testing.T) {
	app := newTestApp(t)
	b, err := app.Batch.Create(batch.CreateInput{Name: "批次N", Composition: model.Composition{"Fe": 100}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if _, err := app.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: "obs1", ImageRef: "img-n",
		PhaseEstimate: map[string]float64{"austenite": -5},
	}); err == nil {
		t.Fatalf("expected negative fraction rejection")
	}
}

// TestCrossBatchImageRejected 拒绝跨批次图像引用。
func TestCrossBatchImageRejected(t *testing.T) {
	app := newTestApp(t)
	b1, _ := app.Batch.Create(batch.CreateInput{Name: "批次1", Composition: model.Composition{"Fe": 100}})
	b2, _ := app.Batch.Create(batch.CreateInput{Name: "批次2", Composition: model.Composition{"Fe": 98, "Cr": 2}})
	if b1.ID == b2.ID {
		t.Fatalf("batches should differ")
	}
	in := observation.CreateInput{BatchID: b1.ID, Observer: "obs1", ImageRef: "img-shared",
		PhaseEstimate: map[string]float64{"austenite": 100}}
	if _, err := app.Observation.Create(in); err != nil {
		t.Fatalf("create obs on b1: %v", err)
	}
	in.BatchID = b2.ID
	if _, err := app.Observation.Create(in); err == nil {
		t.Fatalf("expected cross-batch image rejection")
	}
}

// TestSealedBatchReadOnly 封存批次禁止直接修改。
func TestSealedBatchReadOnly(t *testing.T) {
	app := newTestApp(t)
	b, _ := app.Batch.Create(batch.CreateInput{Name: "批次S", Composition: model.Composition{"Fe": 100}})
	if err := app.Batch.Seal(b.ID); err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := app.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: "obs1", ImageRef: "img-s",
		PhaseEstimate: map[string]float64{"austenite": 100},
	}); err == nil {
		t.Fatalf("expected sealed batch rejection")
	}
}

// TestFullLoopReportPublished 端到端：推断→确认→发布报告。
func TestFullLoopReportPublished(t *testing.T) {
	app := newTestApp(t)
	seedDiagram(t, app)
	b, err := app.Batch.Create(batch.CreateInput{Name: "闭环批次", Alloy: "316L",
		Composition: model.Composition{"Fe": 68, "Cr": 17, "Ni": 12}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if _, err := app.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: "obsA", ImageRef: "img-a",
		PhaseEstimate: map[string]float64{"austenite": 70, "carbide": 30},
	}); err != nil {
		t.Fatalf("create obs: %v", err)
	}
	res, err := app.Inference.Infer(b, inferTestInput(b.ID))
	if err != nil {
		t.Fatalf("infer: %v", err)
	}
	if len(res.Candidates) == 0 {
		t.Fatalf("no candidates")
	}
	for _, c := range res.Candidates {
		if c.Status == model.CandAcceptable || c.Status == model.CandArbitration {
			if _, err := app.Inference.ConfirmAcceptable(c.ID); err != nil {
				t.Fatalf("confirm %d: %v", c.ID, err)
			}
		}
	}
	if err := app.Batch.MoveToReview(b.ID); err != nil {
		t.Fatalf("move to review: %v", err)
	}
	if err := app.Batch.ConfirmComposition(b.ID); err != nil {
		t.Fatalf("confirm composition: %v", err)
	}
	rep, err := app.Report.Publish(report.PublishInput{
		BatchID: b.ID, Title: "闭环报告", Conclusion: "奥氏体为主",
	})
	if err != nil {
		t.Fatalf("publish report: %v", err)
	}
	if _, err := app.Report.SignOff(rep.ID); err != nil {
		t.Fatalf("signoff: %v", err)
	}
	got, err := app.Report.Get(rep.ID)
	if err != nil || got.Status != model.ReportPublished {
		t.Fatalf("report not published: %v", err)
	}
}

// TestNewDiagramDoesNotRewriteOldReport 新相图版本不改写旧报告。
func TestNewDiagramDoesNotRewriteOldReport(t *testing.T) {
	app := newTestApp(t)
	d1 := seedDiagram(t, app)
	b, _ := app.Batch.Create(batch.CreateInput{Name: "批次R", Composition: model.Composition{"Fe": 68, "Cr": 17, "Ni": 12}})
	if _, err := app.Observation.Create(observation.CreateInput{
		BatchID: b.ID, Observer: "obs1", ImageRef: "img-r",
		PhaseEstimate: map[string]float64{"austenite": 70},
	}); err != nil {
		t.Fatalf("create obs: %v", err)
	}
	if _, err := app.Inference.Infer(b, inferTestInput(b.ID)); err != nil {
		t.Fatalf("infer: %v", err)
	}
	cands, _ := app.Inference.ListCandidates(b.ID)
	for _, c := range cands {
		if c.Status == model.CandAcceptable || c.Status == model.CandArbitration {
			_, _ = app.Inference.ConfirmAcceptable(c.ID)
		}
	}
	_ = app.Batch.MoveToReview(b.ID)
	_ = app.Batch.ConfirmComposition(b.ID)
	rep, err := app.Report.Publish(report.PublishInput{BatchID: b.ID, Title: "旧报告", Conclusion: "结论A"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	_, _ = app.Report.SignOff(rep.ID)

	// 新相图版本发布。
	if _, err := app.Diagram.Publish(d1); err != nil {
		t.Fatalf("republish d1: %v", err)
	}
	d2, err := app.Diagram.Create(createDiagramInput())
	if err != nil {
		t.Fatalf("create d2: %v", err)
	}
	if _, err := app.Diagram.Publish(d2.ID); err != nil {
		t.Fatalf("publish d2: %v", err)
	}
	got, err := app.Report.Get(rep.ID)
	if err != nil {
		t.Fatalf("get report: %v", err)
	}
	if got.Status != model.ReportPublished {
		t.Fatalf("old report rewritten: status=%s", got.Status)
	}
	if got.InputVersion.DiagramVer != 1 {
		t.Fatalf("old report input version drifted: ver=%d", got.InputVersion.DiagramVer)
	}
}

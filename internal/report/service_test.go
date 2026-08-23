package report_test

import (
	"errors"
	"testing"

	"task187-phaseaudit/internal/batch"
	"task187-phaseaudit/internal/inference"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/observation"
	"task187-phaseaudit/internal/phasediagram"
	"task187-phaseaudit/internal/report"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func newReportApp(t *testing.T) *service.App {
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

func TestReviseSupersedesPublishedReportAndKeepsInputVersion(t *testing.T) {
	app := newReportApp(t)
	b, err := app.Batch.Create(batch.CreateInput{Name: "报告批次", Composition: model.Composition{"Fe": 100}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if err := app.Batch.MoveToReview(b.ID); err != nil {
		t.Fatalf("move to review: %v", err)
	}
	// 直接准备一个确认候选，验证报告模块对冻结输入和修订历史的处理。
	if _, err := app.Inference.GetCandidate(999); err == nil {
		t.Fatal("unexpected candidate in empty database")
	}
	// 报告发布前必须有确认候选，使用应用闭环创建真实候选与报告。
	app = newReportAppWithConfirmedCandidate(t)
	batchList, err := app.Batch.List(10, 0)
	if err != nil || len(batchList) != 1 {
		t.Fatalf("list batch: len=%d err=%v", len(batchList), err)
	}
	b = batchList[0]
	rep, err := app.Report.Publish(report.PublishInput{BatchID: b.ID, Title: "初版报告", Conclusion: "结论A"})
	if err != nil {
		t.Fatalf("publish report: %v", err)
	}
	if _, err := app.Report.SignOff(rep.ID); err != nil {
		t.Fatalf("sign off: %v", err)
	}
	revised, err := app.Report.Revise(rep.ID, "修订报告", "结论B")
	if err != nil {
		t.Fatalf("revise report: %v", err)
	}
	old, err := app.Report.Get(rep.ID)
	if err != nil {
		t.Fatalf("get old report: %v", err)
	}
	if old.Status != model.ReportSuperseded || old.SupersededBy != revised.ID {
		t.Fatalf("old report=%+v", old)
	}
	if revised.InputVersion.BatchFingerprint != old.InputVersion.BatchFingerprint {
		t.Fatal("revision changed batch input fingerprint")
	}
	revisions, err := app.Report.Revisions(rep.ID)
	if err != nil || len(revisions) != 1 {
		t.Fatalf("revisions=%d err=%v", len(revisions), err)
	}
}

// TestReviseRejectedWhenObservationsAddedAfterPublish 验证已发布报告绑定了发布时的观察证据版本：
// 发布后新增观察会改变证据指纹集合，旧报告的修订应被拒绝并提示输入版本漂移，
// 而非把新证据直接混入旧报告的修订流程。
func TestReviseRejectedWhenObservationsAddedAfterPublish(t *testing.T) {
	app := newReportAppWithConfirmedCandidate(t)
	batchList, err := app.Batch.List(10, 0)
	if err != nil || len(batchList) != 1 {
		t.Fatalf("list batch: len=%d err=%v", len(batchList), err)
	}
	b := batchList[0]
	rep, err := app.Report.Publish(report.PublishInput{BatchID: b.ID, Title: "初版报告", Conclusion: "结论A"})
	if err != nil {
		t.Fatalf("publish report: %v", err)
	}
	if _, err := app.Report.SignOff(rep.ID); err != nil {
		t.Fatalf("sign off: %v", err)
	}
	// 发布时绑定的观察证据版本必须真实落盘，而非被丢弃为空。
	published, err := app.Report.Get(rep.ID)
	if err != nil {
		t.Fatalf("get published report: %v", err)
	}
	if len(published.InputVersion.ObsFingerprints) == 0 {
		t.Fatalf("published report did not bind observation evidence version")
	}

	// 发布后新增一条观察证据。
	if _, err := app.Observation.Create(observationInputExtra(b.ID)); err != nil {
		t.Fatalf("create extra observation: %v", err)
	}
	if _, err := app.Report.Revise(rep.ID, "修订报告", "结论B"); !errors.Is(err, model.ErrReportVersionDrift) {
		t.Fatalf("expected ErrReportVersionDrift after adding observation, got %v", err)
	}

	// 旧报告仍处于已发布，未被替代（修订流程未把新证据混入）。
	old, err := app.Report.Get(rep.ID)
	if err != nil {
		t.Fatalf("get old report: %v", err)
	}
	if old.Status != model.ReportPublished || old.SupersededBy != 0 {
		t.Fatalf("old report should remain published: %+v", old)
	}
}

// TestReviseStillWorksWhenNoObservationChange 验证发布与修订之间无观察变化时正常修订流程不变。
func TestReviseStillWorksWhenNoObservationChange(t *testing.T) {
	app := newReportAppWithConfirmedCandidate(t)
	batchList, err := app.Batch.List(10, 0)
	if err != nil || len(batchList) != 1 {
		t.Fatalf("list batch: len=%d err=%v", len(batchList), err)
	}
	b := batchList[0]
	rep, err := app.Report.Publish(report.PublishInput{BatchID: b.ID, Title: "初版报告", Conclusion: "结论A"})
	if err != nil {
		t.Fatalf("publish report: %v", err)
	}
	if _, err := app.Report.SignOff(rep.ID); err != nil {
		t.Fatalf("sign off: %v", err)
	}
	revised, err := app.Report.Revise(rep.ID, "修订报告", "结论B")
	if err != nil {
		t.Fatalf("revise should succeed when no observation changed: %v", err)
	}
	if revised.InputVersion.BatchFingerprint != rep.InputVersion.BatchFingerprint {
		t.Fatal("revision changed batch input fingerprint")
	}
	if !sameObsFingerprints(revised.InputVersion.ObsFingerprints, rep.InputVersion.ObsFingerprints) {
		t.Fatal("revision changed observation evidence fingerprint")
	}
}

func sameObsFingerprints(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func newReportAppWithConfirmedCandidate(t *testing.T) *service.App {
	t.Helper()
	app := newReportApp(t)
	if _, err := app.Diagram.Create(phasediagramInput()); err != nil {
		t.Fatalf("create diagram: %v", err)
	}
	diagrams, err := app.Diagram.List()
	if err != nil {
		t.Fatalf("list diagrams: %v", err)
	}
	if _, err := app.Diagram.Publish(diagrams[0].ID); err != nil {
		t.Fatalf("publish diagram: %v", err)
	}
	b, err := app.Batch.Create(batch.CreateInput{Name: "报告闭环", Composition: model.Composition{"Fe": 68, "Cr": 17, "Ni": 12}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if _, err := app.Observation.Create(observationInput(b.ID)); err != nil {
		t.Fatalf("create observation: %v", err)
	}
	res, err := app.Inference.Infer(b, inferenceInput(b.ID, diagrams[0].ID))
	if err != nil {
		t.Fatalf("infer: %v", err)
	}
	for _, c := range res.Candidates {
		if c.Status == model.CandAcceptable {
			if _, err := app.Inference.ConfirmAcceptable(c.ID); err != nil {
				t.Fatalf("confirm candidate: %v", err)
			}
		}
	}
	if err := app.Batch.MoveToReview(b.ID); err != nil {
		t.Fatalf("move review: %v", err)
	}
	if err := app.Batch.ConfirmComposition(b.ID); err != nil {
		t.Fatalf("confirm composition: %v", err)
	}
	return app
}

func phasediagramInput() phasediagram.CreateInput {
	return phasediagram.CreateInput{
		Name: "报告相图",
		Phases: []model.PhaseDef{{Phase: "austenite", TypicalFrac: 0.7, Regions: []model.PhaseRegion{
			{Element: "Fe", MinPct: 60, MaxPct: 80},
			{Element: "Cr", MinPct: 10, MaxPct: 25},
			{Element: "Ni", MinPct: 5, MaxPct: 20},
		}}},
	}
}

func observationInput(batchID int64) observation.CreateInput {
	return observation.CreateInput{
		BatchID: batchID, Observer: "报告观察员", ImageRef: "report-image",
		PhaseEstimate: map[string]float64{"austenite": 100},
	}
}

// observationInputExtra 构造发布后新增的第二条观察证据（不同图像，确保指纹不同）。
func observationInputExtra(batchID int64) observation.CreateInput {
	return observation.CreateInput{
		BatchID: batchID, Observer: "补充观察员", ImageRef: "report-image-extra",
		PhaseEstimate: map[string]float64{"austenite": 100},
	}
}

func inferenceInput(batchID, diagramID int64) inference.InferInput {
	return inference.InferInput{BatchID: batchID, DiagramID: diagramID}
}

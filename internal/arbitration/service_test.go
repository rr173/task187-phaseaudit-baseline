package arbitration_test

import (
	"testing"

	"task187-phaseaudit/internal/arbitration"
	"task187-phaseaudit/internal/batch"
	"task187-phaseaudit/internal/inference"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/phasediagram"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func newArbitrationApp(t *testing.T) *service.App {
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

// seedArbitrationCandidate 构造一个因守恒失败进入 arbitration 的候选及其打开的仲裁。
func seedArbitrationCandidate(t *testing.T, app *service.App) (*model.MaterialBatch, *model.PhaseCandidate, *model.Arbitration) {
	t.Helper()
	d, err := app.Diagram.Create(phasediagram.CreateInput{
		Name: "仲裁相图",
		Phases: []model.PhaseDef{{Phase: "phase-a", TypicalFrac: 1, Regions: []model.PhaseRegion{
			{Element: "Fe", MinPct: 60, MaxPct: 70},
			{Element: "Cr", MinPct: 30, MaxPct: 48},
		}}},
	})
	if err != nil {
		t.Fatalf("create diagram: %v", err)
	}
	if _, err := app.Diagram.Publish(d.ID); err != nil {
		t.Fatalf("publish diagram: %v", err)
	}
	b, err := app.Batch.Create(batch.CreateInput{Name: "待仲裁批次", Composition: model.Composition{"Fe": 61, "Cr": 39}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	res, err := app.Inference.Infer(b, inference.InferInput{BatchID: b.ID, DiagramID: d.ID})
	if err != nil || len(res.Candidates) != 1 {
		t.Fatalf("infer candidates=%d err=%v", len(res.Candidates), err)
	}
	c := res.Candidates[0]
	if c.Status != model.CandArbitration {
		t.Fatalf("candidate status=%q, want arbitration", c.Status)
	}
	a, err := app.Arbitration.Open(arbitration.OpenInput{
		BatchID: b.ID, CandidateID: c.ID, Kind: arbitration.KindConservationFailure, Reason: "质量守恒偏差需要复核",
	})
	if err != nil {
		t.Fatalf("open arbitration: %v", err)
	}
	return b, c, a
}


func TestRejectDecisionClosesArbitrationAndMarksBatchInsufficient(t *testing.T) {
	app := newArbitrationApp(t)
	b, c, a := seedArbitrationCandidate(t, app)
	if _, err := app.Arbitration.Decide(a.ID, arbitration.DecideInput{Decision: model.DecisionReject, Note: "证据不足"}); err != nil {
		t.Fatalf("reject arbitration: %v", err)
	}
	gotCandidate, err := app.Inference.GetCandidate(c.ID)
	if err != nil {
		t.Fatalf("get candidate: %v", err)
	}
	if gotCandidate.Status != model.CandRejected {
		t.Fatalf("candidate status=%q, want rejected", gotCandidate.Status)
	}
	gotBatch, err := app.Batch.Get(b.ID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if gotBatch.Status != model.BatchInsufficient {
		t.Fatalf("batch status=%q, want insufficient", gotBatch.Status)
	}
	// 仲裁应随状态流转一同关闭，不应停留在 open。
	gotArb, err := app.Arbitration.Get(a.ID)
	if err != nil {
		t.Fatalf("get arbitration: %v", err)
	}
	if gotArb.Status != model.ArbDecided {
		t.Fatalf("arbitration status=%q, want decided", gotArb.Status)
	}
}

// TestRejectPersistsBothStatesAcrossReopen 验证拒绝决定后候选「已拒绝」与批次
// 「证据不足」两个状态一起落盘：关闭并重开数据库后二者仍各自保持，
// 不会因非原子写停留在 arbitration / pending_review。
func TestRejectPersistsBothStatesAcrossReopen(t *testing.T) {
	dbPath := t.TempDir() + "/phaseaudit-reject.db"
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	app, err := service.New(db)
	if err != nil {
		db.Close()
		t.Fatalf("new app: %v", err)
	}
	b, c, a := seedArbitrationCandidate(t, app)
	if _, err := app.Arbitration.Decide(a.ID, arbitration.DecideInput{Decision: model.DecisionReject, Note: "证据不足"}); err != nil {
		db.Close()
		t.Fatalf("reject arbitration: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	// 重开同一数据库，验证两个状态仍稳定落盘。
	db2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db2.Close()
	app2, err := service.New(db2)
	if err != nil {
		t.Fatalf("new app2: %v", err)
	}
	gotCandidate, err := app2.Inference.GetCandidate(c.ID)
	if err != nil {
		t.Fatalf("get candidate after reopen: %v", err)
	}
	if gotCandidate.Status != model.CandRejected {
		t.Fatalf("after reopen candidate status=%q, want rejected", gotCandidate.Status)
	}
	gotBatch, err := app2.Batch.Get(b.ID)
	if err != nil {
		t.Fatalf("get batch after reopen: %v", err)
	}
	if gotBatch.Status != model.BatchInsufficient {
		t.Fatalf("after reopen batch status=%q, want insufficient", gotBatch.Status)
	}
}

// TestConfirmDecisionMarksCandidateConfirmed 验证确认决定的状态流转正常：
// 候选 → confirmed，仲裁 → decided。批次推进由调用方显式流转，故这里不强校验批次。
func TestConfirmDecisionMarksCandidateConfirmed(t *testing.T) {
	app := newArbitrationApp(t)
	_, c, a := seedArbitrationCandidate(t, app)
	if _, err := app.Arbitration.Decide(a.ID, arbitration.DecideInput{Decision: model.DecisionConfirm, Note: "复核确认"}); err != nil {
		t.Fatalf("confirm arbitration: %v", err)
	}
	gotCandidate, err := app.Inference.GetCandidate(c.ID)
	if err != nil {
		t.Fatalf("get candidate: %v", err)
	}
	if gotCandidate.Status != model.CandConfirmed {
		t.Fatalf("candidate status=%q, want confirmed", gotCandidate.Status)
	}
	gotArb, err := app.Arbitration.Get(a.ID)
	if err != nil {
		t.Fatalf("get arbitration: %v", err)
	}
	if gotArb.Status != model.ArbDecided || gotArb.Decision != model.DecisionConfirm {
		t.Fatalf("arbitration status=%q decision=%q, want decided/confirm", gotArb.Status, gotArb.Decision)
	}
}

// TestDownscaleDecisionRewritesFractionAndMarksReview 验证下调决定的状态流转正常：
// 候选比例重写、状态 → acceptable，批次 → pending_review，仲裁 → decided。
func TestDownscaleDecisionRewritesFractionAndMarksReview(t *testing.T) {
	app := newArbitrationApp(t)
	b, c, a := seedArbitrationCandidate(t, app)
	newFrac := 0.42
	if _, err := app.Arbitration.Decide(a.ID, arbitration.DecideInput{
		Decision: model.DecisionDownscale, Note: "下调比例以通过守恒", NewFraction: &newFrac,
	}); err != nil {
		t.Fatalf("downscale arbitration: %v", err)
	}
	gotCandidate, err := app.Inference.GetCandidate(c.ID)
	if err != nil {
		t.Fatalf("get candidate: %v", err)
	}
	if gotCandidate.Status != model.CandAcceptable {
		t.Fatalf("candidate status=%q, want acceptable", gotCandidate.Status)
	}
	if gotCandidate.Fraction != newFrac {
		t.Fatalf("candidate fraction=%v, want %v", gotCandidate.Fraction, newFrac)
	}
	gotBatch, err := app.Batch.Get(b.ID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if gotBatch.Status != model.BatchPendingReview {
		t.Fatalf("batch status=%q, want pending_review", gotBatch.Status)
	}
	gotArb, err := app.Arbitration.Get(a.ID)
	if err != nil {
		t.Fatalf("get arbitration: %v", err)
	}
	if gotArb.Status != model.ArbDecided || gotArb.Decision != model.DecisionDownscale {
		t.Fatalf("arbitration status=%q decision=%q, want decided/downscale", gotArb.Status, gotArb.Decision)
	}
}

// TestDownscaleRequiresNewFraction 验证下调决定的入参校验仍在事务前置（不落盘）。
func TestDownscaleRequiresNewFraction(t *testing.T) {
	app := newArbitrationApp(t)
	b, c, a := seedArbitrationCandidate(t, app)
	if _, err := app.Arbitration.Decide(a.ID, arbitration.DecideInput{Decision: model.DecisionDownscale, Note: "缺比例"}); err == nil {
		t.Fatalf("expected error for downscale without new_fraction")
	}
	// 入参非法不应推进任何状态。
	gotCandidate, _ := app.Inference.GetCandidate(c.ID)
	if gotCandidate.Status != model.CandArbitration {
		t.Fatalf("candidate status=%q, want arbitration after failed downscale", gotCandidate.Status)
	}
	gotBatch, _ := app.Batch.Get(b.ID)
	if gotBatch.Status != model.BatchPendingObservation {
		t.Fatalf("batch status=%q, want pending_observation after failed downscale", gotBatch.Status)
	}
	gotArb, _ := app.Arbitration.Get(a.ID)
	if gotArb.Status != model.ArbOpen {
		t.Fatalf("arbitration status=%q, want open after failed downscale", gotArb.Status)
	}
}


package phasediagram_test

import (
	"testing"

	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/phasediagram"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func newDiagramApp(t *testing.T) *service.App {
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

func diagramInput(name string) phasediagram.CreateInput {
	return phasediagram.CreateInput{
		Name: name,
		Phases: []model.PhaseDef{
			{Phase: "austenite", TypicalFrac: 0.7, Regions: []model.PhaseRegion{
				{Element: "Fe", MinPct: 60, MaxPct: 80},
			}},
			{Phase: "ferrite", TypicalFrac: 0.3, Regions: []model.PhaseRegion{
				{Element: "Fe", MinPct: 80, MaxPct: 100},
			}},
		},
	}
}

func TestDiagramVersionsAndCandidateCoverage(t *testing.T) {
	app := newDiagramApp(t)
	d1, err := app.Diagram.Create(diagramInput("Fe 相图"))
	if err != nil {
		t.Fatalf("create v1: %v", err)
	}
	if d1.VersionNo != 1 || d1.Status != "draft" {
		t.Fatalf("v1=%+v", d1)
	}
	if _, err := app.Diagram.Publish(d1.ID); err != nil {
		t.Fatalf("publish v1: %v", err)
	}
	d2, err := app.Diagram.Create(diagramInput("Fe 相图"))
	if err != nil {
		t.Fatalf("create v2: %v", err)
	}
	if d2.VersionNo != 2 {
		t.Fatalf("v2 version=%d, want 2", d2.VersionNo)
	}
	phases := app.Diagram.CandidatePhases(d1, model.Composition{"Fe": 70})
	if len(phases) != 1 || phases[0].Phase != "austenite" {
		t.Fatalf("candidate phases=%v", phases)
	}
	if !app.Diagram.CompositionCovered(d1, model.Composition{"Fe": 70}) {
		t.Fatal("expected Fe composition to be covered")
	}
	if len(app.Diagram.CandidatePhases(d1, model.Composition{"Fe": 95})) != 1 {
		t.Fatal("expected ferrite candidate for Fe=95")
	}
}

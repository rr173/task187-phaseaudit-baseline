package service

import (
	"testing"

	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/phasediagram"
	"task187-phaseaudit/internal/store"
)

// newTestApp 创建内存库应用。
func newTestApp(t *testing.T) *App {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	app, err := New(db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	return app
}

// seedDiagram 建相图并发布。
func seedDiagram(t *testing.T, app *App) int64 {
	t.Helper()
	d, err := app.Diagram.Create(createDiagramInput())
	if err != nil {
		t.Fatalf("create diagram: %v", err)
	}
	if _, err := app.Diagram.Publish(d.ID); err != nil {
		t.Fatalf("publish diagram: %v", err)
	}
	return d.ID
}

// createDiagramInput 测试用 Fe-Cr-Ni 相图。
func createDiagramInput() phasediagram.CreateInput {
	return phasediagram.CreateInput{
		Name: "测试相图",
		Phases: []model.PhaseDef{
			{Phase: "austenite", Regions: []model.PhaseRegion{
				{Element: "Fe", MinPct: 55, MaxPct: 90},
				{Element: "Cr", MinPct: 0, MaxPct: 22},
				{Element: "Ni", MinPct: 5, MaxPct: 30},
			}, TypicalFrac: 0.7, Notes: "γ-Fe"},
			{Phase: "carbide", Regions: []model.PhaseRegion{
				{Element: "Fe", MinPct: 55, MaxPct: 80},
				{Element: "Cr", MinPct: 5, MaxPct: 30},
				{Element: "Ni", MinPct: 0, MaxPct: 5},
			}, TypicalFrac: 0.2, Notes: "M23C6"},
		},
		Summary: "测试相图",
	}
}

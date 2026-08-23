package phasediagram_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"task187-phaseaudit/internal/httpapi"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/phasediagram"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func TestTask187Bug05_DiagramVersionsAdvanceByOne(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	app, err := service.New(db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	first, err := app.Diagram.Create(versionDiagramInput())
	if err != nil {
		t.Fatalf("create first diagram: %v", err)
	}
	second, err := app.Diagram.Create(versionDiagramInput())
	if err != nil {
		t.Fatalf("create second diagram: %v", err)
	}
	if first.VersionNo != 1 || second.VersionNo != 2 {
		t.Fatalf("versions=(%d,%d), want (1,2)", first.VersionNo, second.VersionNo)
	}
	ts := httptest.NewServer(httpapi.New(app).Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/diagrams")
	if err != nil {
		t.Fatalf("list diagrams: %v", err)
	}
	defer resp.Body.Close()
	var diagrams []struct {
		VersionNo int `json:"version_no"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&diagrams); err != nil {
		t.Fatalf("decode diagrams: %v", err)
	}
	if len(diagrams) != 2 || diagrams[0].VersionNo != 2 || diagrams[1].VersionNo != 1 {
		t.Fatalf("listed versions=%v, want [2 1]", diagrams)
	}
}

func versionDiagramInput() phasediagram.CreateInput {
	return phasediagram.CreateInput{
		Name: "版本递进相图",
		Phases: []model.PhaseDef{{Phase: "alpha", TypicalFrac: 0.5,
			Regions: []model.PhaseRegion{{Element: "Fe", MinPct: 90, MaxPct: 100}}}},
	}
}


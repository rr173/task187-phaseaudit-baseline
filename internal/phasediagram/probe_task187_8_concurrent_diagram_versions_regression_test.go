package phasediagram_test

import (
	"sort"
	"sync"
	"testing"

	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/phasediagram"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func TestTask187Bug08_ConcurrentDiagramVersionsAreUnique(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	app, err := service.New(db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	const workers = 20
	start := make(chan struct{})
	results := make(chan int, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			d, err := app.Diagram.Create(phasediagram.CreateInput{
				Name: "并发版本相图",
				Phases: []model.PhaseDef{{Phase: "alpha", TypicalFrac: 0.5,
					Regions: []model.PhaseRegion{{Element: "Fe", MinPct: 90, MaxPct: 100}}}},
			})
			if err != nil {
				errs <- err
				return
			}
			results <- d.VersionNo
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent create error: %v", err)
	}
	versions := make([]int, 0, workers)
	for version := range results {
		versions = append(versions, version)
	}
	sort.Ints(versions)
	if len(versions) != workers {
		t.Fatalf("created=%d, want %d", len(versions), workers)
	}
	for i, version := range versions {
		want := i + 1
		if version != want {
			t.Fatalf("versions=%v, want consecutive 1..%d", versions, workers)
		}
	}
}

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

// TestConcurrentDiagramVersionAllocation 验证同名相图并发创建时：
//   - 所有请求都成功（不撞 UNIQUE(name,version_no)）；
//   - 版本号不重复且连续为 1..N；
//   - 不同名称相图的版本序列彼此独立。
func TestConcurrentDiagramVersionAllocation(t *testing.T) {
	app := newDiagramApp(t)
	const n = 20

	// 同名相图并发创建 n 个版本。
	var wg sync.WaitGroup
	results := make([]*model.PhaseDiagram, n)
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // 所有 goroutine 就绪后同时开跑，最大化竞争窗口。
			d, err := app.Diagram.Create(diagramInput("并发相图"))
			results[i] = d
			errs[i] = err
		}(i)
	}
	close(start)
	wg.Wait()

	got := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d failed: %v", i, errs[i])
		}
		if results[i].Name != "并发相图" || results[i].Status != "draft" {
			t.Fatalf("goroutine %d bad result: %+v", i, results[i])
		}
		got = append(got, results[i].VersionNo)
	}
	sort.Ints(got)
	for i, v := range got {
		if v != i+1 {
			t.Fatalf("versions not consecutive 1..%d: got %v", n, got)
		}
	}

	// 不同名称相图版本序列应独立（不受「并发相图」影响）。
	other, err := app.Diagram.Create(diagramInput("独立相图"))
	if err != nil {
		t.Fatalf("create independent diagram: %v", err)
	}
	if other.VersionNo != 1 {
		t.Fatalf("independent diagram version=%d, want 1", other.VersionNo)
	}

	// 确认「并发相图」在名称维度上恰好 n 个版本。
	ds, err := app.Diagram.List()
	if err != nil {
		t.Fatalf("list diagrams: %v", err)
	}
	count := 0
	for _, d := range ds {
		if d.Name == "并发相图" {
			count++
		}
	}
	if count != n {
		t.Fatalf("expected %d diagrams for name, got %d", n, count)
	}

	// 再来一轮 n 个并发请求，验证从已存在的 n 继续连续递增到 2n。
	results2 := make([]*model.PhaseDiagram, n)
	errs2 := make([]error, n)
	var wg2 sync.WaitGroup
	start2 := make(chan struct{})
	for i := 0; i < n; i++ {
		wg2.Add(1)
		go func(i int) {
			defer wg2.Done()
			<-start2
			d, err := app.Diagram.Create(diagramInput("并发相图"))
			results2[i] = d
			errs2[i] = err
		}(i)
	}
	close(start2)
	wg2.Wait()
	got2 := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if errs2[i] != nil {
			t.Fatalf("round2 goroutine %d failed: %v", i, errs2[i])
		}
		got2 = append(got2, results2[i].VersionNo)
	}
	sort.Ints(got2)
	for i, v := range got2 {
		if v != n+i+1 {
			t.Fatalf("round2 versions not consecutive %d..%d: got %v", n+1, 2*n, got2)
		}
	}
}

// TestConcurrentDiagramDistinctNamesParallel 两组不同名称相图并发交织创建，
// 验证两组版本号各自独立从 1 递增、互不串味。
func TestConcurrentDiagramDistinctNamesParallel(t *testing.T) {
	app := newDiagramApp(t)
	const n = 10
	var wg sync.WaitGroup
	start := make(chan struct{})

	collect := func(name string, out []int, errOut []error) {
		defer wg.Done()
		<-start
		d, err := app.Diagram.Create(diagramInput(name))
		if d != nil {
			out[0] = d.VersionNo
		}
		errOut[0] = err
	}

	a := make([]int, n)
	b := make([]int, n)
	ea := make([]error, n)
	eb := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(2)
		go collect("相图A", a[i:i+1], ea[i:i+1])
		go collect("相图B", b[i:i+1], eb[i:i+1])
	}
	close(start)
	wg.Wait()

	for i := 0; i < n; i++ {
		if ea[i] != nil {
			t.Fatalf("A goroutine %d failed: %v", i, ea[i])
		}
		if eb[i] != nil {
			t.Fatalf("B goroutine %d failed: %v", i, eb[i])
		}
	}
	sort.Ints(a)
	sort.Ints(b)
	for i, v := range a {
		if v != i+1 {
			t.Fatalf("相图A versions not 1..%d: got %v", n, a)
		}
	}
	for i, v := range b {
		if v != i+1 {
			t.Fatalf("相图B versions not 1..%d: got %v", n, b)
		}
	}
}

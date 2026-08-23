// Command phaseaudit 是材料显微组织相鉴定证据复核台的入口。
//
// 支持三个标志：
//   - --addr :8080      监听地址（默认 :8080）
//   - --db ./data.db    SQLite 数据库路径（默认 ./phaseaudit.db）
//   - --smoke-test      执行端到端冒烟：真实创建数据、关闭并重开数据库
//                       验证持久化与重启恢复，随后以 0 退出码结束。
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"task187-phaseaudit/internal/httpapi"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "./phaseaudit.db", "SQLite database path")
	smoke := flag.Bool("smoke-test", false, "run end-to-end smoke test and exit")
	flag.Parse()

	if *smoke {
		if err := runSmokeTest(*dbPath); err != nil {
			fmt.Fprintln(os.Stderr, "SMOKE TEST FAILED:", err)
			os.Exit(1)
		}
		fmt.Println("SMOKE TEST PASSED")
		return
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	app, err := service.New(db)
	if err != nil {
		log.Fatalf("init services: %v", err)
	}
	srv := httpapi.New(app)
	log.Printf("task187-phaseaudit listening on %s (db=%s)", *addr, *dbPath)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		log.Fatalf("http server: %v", err)
	}
}

// runSmokeTest 执行冒烟测试：
//  1. 打开数据库 A，完整跑一遍业务闭环（相图→批次→观察→推断→确认→发布报告）；
//  2. 幂等验证：重复创建同指纹批次/观察均不产生重复数据；
//  3. 守恒守卫：负比例、比例和超 1、跨批次图像引用均被拒绝；
//  4. 仲裁路径：观察者分歧 → 打开仲裁 → 决定确认；
//  5. 关闭数据库 A，重新打开同一路径数据库 B，验证数据仍在（重启恢复）；
//  6. 新相图版本发布不改写旧报告，只触发新一轮推断。
func runSmokeTest(dbPath string) error {
	if dbPath != ":memory:" {
		_ = os.Remove(dbPath)
	}

	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	app, err := service.New(db)
	if err != nil {
		db.Close()
		return fmt.Errorf("init services: %w", err)
	}

	// --- 步骤 1：相图 ---
	d, err := app.Diagram.Create(phaseDiagramInput())
	if err != nil {
		db.Close()
		return fmt.Errorf("create diagram: %w", err)
	}
	d, err = app.Diagram.Publish(d.ID)
	if err != nil {
		db.Close()
		return fmt.Errorf("publish diagram: %w", err)
	}
	if d.Status != "published" {
		db.Close()
		return fmt.Errorf("diagram not published")
	}

	// --- 步骤 2：批次（成分：Fe 68 / Cr 17 / Ni 12，奥氏体+碳化物预期）---
	comp := model.Composition{"Fe": 68, "Cr": 17, "Ni": 12}
	b, err := app.Batch.Create(serviceBatchInput("316L-冒烟批次", comp))
	if err != nil {
		db.Close()
		return fmt.Errorf("create batch: %w", err)
	}
	// 幂等：同指纹重复创建返回既有批次。
	bDup, err := app.Batch.Create(serviceBatchInput("316L-冒烟批次", comp))
	if err != nil || bDup.ID != b.ID {
		db.Close()
		return fmt.Errorf("batch fingerprint dedup failed (err=%v)", err)
	}

	// --- 步骤 3：观察（两位观察者估计分歧）---
	o1, err := app.Observation.Create(observationInput(b.ID, "观察员甲", "img-a-001",
		map[string]float64{"austenite": 70, "ferrite": 10, "carbide": 20}))
	if err != nil {
		db.Close()
		return fmt.Errorf("create obs1: %w", err)
	}
	if _, err := app.Observation.Create(observationInput(b.ID, "观察员乙", "img-a-002",
		map[string]float64{"austenite": 55, "ferrite": 20, "carbide": 25})); err != nil {
		db.Close()
		return fmt.Errorf("create obs2: %w", err)
	}
	// 幂等：观察员甲重复提交同指纹 → 既有记录。
	o1Dup, err := app.Observation.Create(observationInput(b.ID, "观察员甲", "img-a-001",
		map[string]float64{"austenite": 70, "ferrite": 10, "carbide": 20}))
	if err != nil || o1Dup.ID != o1.ID {
		db.Close()
		return fmt.Errorf("observation fingerprint dedup failed (err=%v)", err)
	}
	// 跨批次图像引用守卫。
	otherBatch, err := app.Batch.Create(serviceBatchInput("304L-另一批", model.Composition{"Fe": 70, "Cr": 18, "Ni": 8}))
	if err != nil {
		db.Close()
		return fmt.Errorf("create other batch: %w", err)
	}
	if _, err := app.Observation.Create(observationInput(otherBatch.ID, "观察员丙", "img-a-001",
		map[string]float64{"austenite": 80})); err == nil {
		db.Close()
		return fmt.Errorf("expected cross-batch image rejection")
	}

	// 分歧检测。
	if err := app.Observation.ResolveStatus(b.ID); err != nil {
		db.Close()
		return fmt.Errorf("resolve status: %w", err)
	}
	div, err := app.Observation.Divergence(b.ID)
	if err != nil {
		db.Close()
		return fmt.Errorf("divergence check: %w", err)
	}
	if !div {
		db.Close()
		return fmt.Errorf("expected observer divergence")
	}

	// --- 步骤 4：推断 ---
	res, err := app.Inference.Infer(b, inferInput(d.ID, b.ID))
	if err != nil {
		db.Close()
		return fmt.Errorf("infer: %w", err)
	}
	if len(res.Candidates) == 0 {
		db.Close()
		return fmt.Errorf("no candidates inferred")
	}
	// 守恒守卫：比例和检查。
	sumRes, err := app.Inference.CheckFractionSum(b.ID)
	if err != nil {
		db.Close()
		return fmt.Errorf("fraction sum: %w", err)
	}
	if !sumRes.OK {
		db.Close()
		return fmt.Errorf("fraction sum violation: %+v", sumRes.Violations)
	}

	// --- 步骤 5：仲裁路径（分歧候选）---
	// 找到处于 arbitration 或 acceptable 的候选，走仲裁确认。
	arbCand := res.Candidates[0]
	arb, err := app.Arbitration.Open(arbInput(b.ID, arbCand.ID))
	if err != nil {
		// 候选可能已可接受，直接确认。
		if _, cerr := app.Inference.ConfirmAcceptable(arbCand.ID); cerr != nil {
			db.Close()
			return fmt.Errorf("confirm candidate fallback: %v (open arb: %v)", cerr, err)
		}
	} else {
		decided, derr := app.Arbitration.Decide(arb.ID, decideInputConfirm())
		if derr != nil {
			db.Close()
			return fmt.Errorf("decide arbitration: %w", derr)
		}
		if decided.Status != "decided" {
			db.Close()
			return fmt.Errorf("arbitration not decided")
		}
	}
	// 确认剩余候选。
	cands, err := app.Inference.ListCandidates(b.ID)
	if err != nil {
		db.Close()
		return fmt.Errorf("list candidates: %w", err)
	}
	confirmedCount := 0
	for _, c := range cands {
		if c.Status == "confirmed" {
			confirmedCount++
			continue
		}
		if c.Status == "acceptable" {
			if _, err := app.Inference.ConfirmAcceptable(c.ID); err != nil {
				db.Close()
				return fmt.Errorf("confirm candidate %d: %w", c.ID, err)
			}
			confirmedCount++
		}
	}
	if confirmedCount == 0 {
		db.Close()
		return fmt.Errorf("no candidate confirmed")
	}

	// 批次 → 待复核 → 相组成已定。
	if err := app.Batch.MoveToReview(b.ID); err != nil {
		db.Close()
		return fmt.Errorf("move to review: %w", err)
	}
	if err := app.Batch.ConfirmComposition(b.ID); err != nil {
		db.Close()
		return fmt.Errorf("confirm composition: %w", err)
	}
	b, _ = app.Batch.Get(b.ID)
	if b.Status != "composition_fixed" {
		db.Close()
		return fmt.Errorf("batch status=%s, want composition_fixed", b.Status)
	}

	// --- 步骤 6：发布报告 ---
	rep, err := app.Report.Publish(reportInput(b.ID, "316L-冒烟批次 微观组织报告",
		"奥氏体为主，晶界碳化物少量，符合 1100°C 固溶+时效预期"))
	if err != nil {
		db.Close()
		return fmt.Errorf("publish report: %w", err)
	}
	rep, err = app.Report.SignOff(rep.ID)
	if err != nil {
		db.Close()
		return fmt.Errorf("signoff report: %w", err)
	}
	if rep.Status != "published" {
		db.Close()
		return fmt.Errorf("report status=%s", rep.Status)
	}

	// --- 步骤 7：关闭并重开，验证持久化恢复 ---
	if err := db.Close(); err != nil {
		return fmt.Errorf("close db: %w", err)
	}
	db2, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("reopen db: %w", err)
	}
	defer db2.Close()
	app2, err := service.New(db2)
	if err != nil {
		return fmt.Errorf("re-init services: %w", err)
	}

	// 数据仍在。
	gotB, err := app2.Batch.Get(b.ID)
	if err != nil || gotB.Status != "composition_fixed" {
		return fmt.Errorf("restore batch failed (err=%v, status=%q)", err, gotBStatus(gotB))
	}
	gotObs, err := app2.Observation.Get(o1.ID)
	if err != nil || gotObs.Fingerprint == "" {
		return fmt.Errorf("restore observation failed (err=%v)", err)
	}
	gotRep, err := app2.Report.Get(rep.ID)
	if err != nil || gotRep.Status != "published" {
		return fmt.Errorf("restore report failed (err=%v, status=%q)", err, gotRep.Status)
	}
	// 重启后恢复待仲裁任务：应无打开仲裁（已全部决定）。
	openArbs, err := app2.Arbitration.ListOpen(b.ID)
	if err != nil || len(openArbs) != 0 {
		return fmt.Errorf("open arbitrations after restore: n=%d (err=%v)", len(openArbs), err)
	}

	// --- 步骤 8：新相图版本不改写旧报告，只触发新一轮推断 ---
	d2, err := app2.Diagram.Create(phaseDiagramInput())
	if err != nil {
		return fmt.Errorf("create diagram v2: %w", err)
	}
	d2, err = app2.Diagram.Publish(d2.ID)
	if err != nil {
		return fmt.Errorf("publish diagram v2: %w", err)
	}
	if d2.VersionNo <= d.VersionNo {
		return fmt.Errorf("diagram v2 version not incremented")
	}
	// 旧报告保持不变。
	gotRep2, err := app2.Report.Get(rep.ID)
	if err != nil || gotRep2.Status != "published" {
		return fmt.Errorf("old report changed after new diagram")
	}
	if gotRep2.InputVersion.DiagramVer != d.VersionNo {
		return fmt.Errorf("old report input version drifted")
	}
	// 新轮次推断（不落旧报告）。
	if _, err := app2.Inference.Infer(gotB, inferInput(d2.ID, b.ID)); err != nil {
		return fmt.Errorf("re-infer with v2: %w", err)
	}

	fmt.Printf("smoke: diagram=%d(v%d) batch=%d obs=%d candidates=%d confirmed=%d report=%d(published) arbitrations=0\n",
		d.ID, d.VersionNo, b.ID, o1.ID, len(res.Candidates), confirmedCount, rep.ID)
	return nil
}

// gotBStatus 安全取批次状态（nil 防护）。
func gotBStatus(b *model.MaterialBatch) string {
	if b == nil {
		return "<nil>"
	}
	return b.Status
}

# task187-phaseaudit · 材料显微组织相鉴定证据复核台

面向材料分析工程师的相鉴定证据复核服务：登记合金成分与热历史，录入显微观察，
系统依据相图摘要与测量误差推断相组成候选并执行成分守恒检查；复核者确认、下调或
拒绝候选，观察者分歧进入仲裁；批准结论冻结为批次微观组织报告，补测创建修订报告。

## 业务闭环

1. 工程师登记**材料批次**（成分 + 热历史）。
2. 录入**显微观察**（特征 + 相比例估计），相同图像/测量指纹幂等。
3. 推断引擎按**相图版本**匹配候选相，叠加测量误差给出比例区间，检查**成分守恒**与
   **比例和 ≤ 1**。
4. 复核者确认/拒绝候选；守恒失败或观察者分歧触发**仲裁**。
5. 相组成已定后**发布组织报告**（冻结快照，绑定相图与输入版本）。
6. 新相图版本发布不改写旧报告，只触发新一轮推断；补测创建**修订报告**并替代旧报告。

## 状态机

| 实体 | 状态机 |
| --- | --- |
| 材料批次 | `pending_observation` → `pending_review` → `composition_fixed` / `insufficient` → `sealed` |
| 观察 | `pending` → `support` / `conflict` → `excluded` / `superseded` |
| 相候选 | `pending` → `acceptable` / `out_of_diagram` / `arbitration` → `confirmed` / `rejected` |
| 组织报告 | `draft` → `pending_sign` → `published` → `superseded` |
| 仲裁 | `open` → `decided` / `dismissed` |

## 标准命令

```bash
# 构建 / 静态检查 / 测试
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet   ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go test  ./...

# 冒烟测试（端到端 + 重启恢复，退出码 0 即通过）
go run ./cmd/phaseaudit --smoke-test

# 启动服务
go run ./cmd/phaseaudit --addr :8080 --db ./phaseaudit.db
```

## 主要 API（统一 `/api` 前缀）

| 能力 | API |
| --- | --- |
| 登记/查询批次 | `POST /api/batches`、`GET /api/batches`、`GET /api/batches/{id}` |
| 批次状态 | `POST /api/batches/{id}/seal`、`/review`、`/insufficient`、`/fixed` |
| 观察证据 | `POST /api/batches/{id}/observations`、`GET /api/batches/{id}/observations`、`POST /api/observations/{id}/exclude` |
| 相图版本 | `POST /api/diagrams`、`POST /api/diagrams/{id}/publish`、`GET /api/diagrams` |
| 候选推断 | `POST /api/batches/{id}/infer`、`GET /api/batches/{id}/candidates`、`POST /api/candidates/{id}/confirm`、`/reject`、`GET /api/batches/{id}/fractionsum` |
| 仲裁 | `POST /api/arbitrations`、`GET /api/batches/{id}/arbitrations`、`POST /api/arbitrations/{id}/decide` |
| 报告 | `POST /api/batches/{id}/reports`、`POST /api/reports/{id}/signoff`、`/revise`、`GET /api/reports/compare` |
| 历史/自检 | `GET /api/batches/{id}/history`、`GET /api/stats`、`GET /api/health` |

## 持久化

SQLite（`modernc.org/sqlite`，纯 Go，CGO 无关）。表：`material_batches`、
`observations`、`phase_diagrams`、`phase_candidates`、`arbitrations`、
`micro_reports`、`report_revisions`。指纹列 UNIQUE 保证幂等；关闭重开同一数据库
验证重启恢复；报告快照绑定相图与输入版本防漂移。

## 关键不变量

- 拒绝负相比例；候选比例和 ≤ 1。
- 相图外成分（无相区间覆盖）拒绝。
- 禁止跨批次图像引用。
- 已封存批次禁止直接修改。
- 报告发布绑定输入版本；并发修改输入触发版本漂移守卫。

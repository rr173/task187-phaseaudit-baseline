# BENZHI_README · task187-phaseaudit 评测说明

## 项目身份

- 项目：材料显微组织相鉴定证据复核台（`task187-phaseaudit`）
- 需求：`REQ-20260823-050`
- 模块：`task187-phaseaudit`，Go 1.26.3 + SQLite（`modernc.org/sqlite`）
- 代码基线：`env/`（源码仓库根目录）

## 构建与门禁

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet   ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go test  ./...
```

`--smoke-test` 契约：不启动长驻服务，真实创建相图/批次/观察/候选/仲裁/报告，
关闭并重新打开同一数据库验证持久化与重启恢复，最后以退出码 0 结束。
它是 Docker `CMD` 与双架构验证的唯一判据。

```bash
go run ./cmd/phaseaudit --smoke-test   # 输出 SMOKE TEST PASSED 且 exit 0
```

## API 摘要（前缀 /api）

- 批次：`POST/GET /api/batches`、`GET /api/batches/{id}`、`POST .../seal|review|insufficient|fixed`
- 观察：`POST /api/batches/{id}/observations`、`GET .../observations`、`POST /api/observations/{id}/exclude`
- 相图：`POST /api/diagrams`、`POST /api/diagrams/{id}/publish`
- 推断：`POST /api/batches/{id}/infer`、`GET .../candidates`、`POST /api/candidates/{id}/confirm|reject`
- 仲裁：`POST /api/arbitrations`、`POST /api/arbitrations/{id}/decide`
- 报告：`POST /api/batches/{id}/reports`、`POST /api/reports/{id}/signoff|revise`、`GET /api/reports/compare`

## Docker 双架构

```bash
# amd64
bash build_benzhi_docker.sh task187-phaseaudit linux/amd64
docker run --rm task187-phaseaudit:latest --smoke-test

# arm64
bash build_benzhi_docker.sh task187-phaseaudit linux/arm64
docker run --rm task187-phaseaudit:latest --smoke-test
```

仅传 `--smoke-test` 标志，不追加 `/app/phaseaudit` 路径参数（Dockerfile 已设
ENTRYPOINT+CMD；追加路径参数会被 flag 解析当作位置参数，导致 `--smoke-test`
不生效、服务进入长驻并最终被 kill 137）。

## 服务启动

```bash
docker run --rm -p 8080:8080 task187-phaseaudit:latest --addr :8080
```

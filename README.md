# 高纬度天文台低温仪器观测窗口与校准归档服务

单进程 Go HTTP JSON 服务，基于 `database/sql` 与真实嵌入式 SQLite（`modernc.org/sqlite`）
持久化到 `DB_PATH` 文件，管理天文台站点、观测仪器、探测器通道、低温系统、校准方案版本、
标准源、观测窗口、目标计划、原始观测批次、质量指标、异常处置、数据归档与发布许可。

## 快速开始

```bash
# 环境要求：Go toolchain go1.26.5，GOTOOLCHAIN=local，go.mod 语言版本 go 1.21
export GOTOOLCHAIN=local

# 构建与测试（使用真实临时 SQLite 文件）
go build ./...
go vet ./...
go test ./...

# 运行（PORT 默认 8080，DB_PATH 必填且禁止 :memory:）
PORT=8080 DB_PATH=./observatory.db ./server  # go build -o server ./cmd/server
curl http://localhost:8080/healthz
```

## 文档

- [docs/领域说明.md](docs/领域说明.md)：领域背景、子域职责、业务主链与关键不变量
- [docs/状态转换表.md](docs/状态转换表.md)：全部实体状态机
- [docs/数据模型.md](docs/数据模型.md)：20 张表的列级定义与索引
- [docs/接口契约.md](docs/接口契约.md)：HTTP JSON 接口、统一错误、分页与幂等约定

## 架构分层

```
cmd/server            入口：配置加载、迁移、作业恢复、优雅关闭（SIGINT/SIGTERM）
internal/config       环境变量配置（PORT/DB_PATH/作业参数）
internal/clock        可注入时钟（Real / Fake）
internal/logging      slog JSON 结构化日志
internal/apperr       统一错误码与 HTTP 映射
internal/domain       状态机与领域规则（温度/校准覆盖/曝光序列/质量/双人复核/窗口冻结）
internal/model        15 类实体模型
internal/store/sqlite 连接（WAL、外键、busy_timeout，禁止 :memory:）、迁移、事务
internal/repo         17 个仓储（乐观锁、幂等键、键集分页、分析查询）
internal/service      12 个领域服务（三个真实事务：校准封存+观测启用、指标封存+异常复测、归档校验+成果发布）
internal/jobs         持久化作业运行器：预冷超时、校准到期、窗口结束、归档校验、失败重试、重启恢复
internal/httpx        轻量路由（go1.21 兼容）、中间件、统一响应、稳定分页、13 组处理器
```

## 关键业务规则

1. 窗口批准即冻结仪器配置、探测器通道、校准方案与目标优先级快照；冻结期拒绝配置变更。
2. 预冷与观测全程温度必须处于 `[temp_min_mK, temp_max_mK]`；观测期越界即隔离批次并登记异常。
3. 校准有效期必须完整覆盖观测批次 `[started_at, finished_at]`，且需存在合格校准记录。
4. 目标曝光序号严格连续（`seq = max+1`），跳号/重复返回 412。
5. 封存指标不达标：批次保持隔离，仅能创建关联复测批次；复测达标后原异常自动闭环。
6. 迟到指标不得覆盖已归档批次（412）。
7. 成果发布实行双人复核（复核人 ≠ 提交人），过期许可禁止发布。
8. 低温读数、目标排程、归档请求、批次创建均使用幂等键；重复请求返回首次结果（`replay=true`）。
9. 全部可变实体乐观锁（`version`），失配返回 409 `version_conflict`。
10. 归档对象仅软删除（`deleted_at`），无物理删除入口；仪器状态历史、校准记录、封存指标不可变。

## 分析查询（稳定分页）

| 接口 | 说明 |
| --- | --- |
| `GET /api/v1/queries/instruments-pending-calibration?within_hours=72` | 临近窗口仍未完成校准的仪器 |
| `GET /api/v1/queries/cryo-anomaly-trend?days=7` | 低温异常趋势（按系统按日聚合） |
| `GET /api/v1/queries/target-conflicts` | 目标排程冲突（窗口时间重叠） |
| `GET /api/v1/queries/quality-decline?min_consecutive=3` | 质量指标连续下降批次链 |
| `GET /api/v1/queries/pending-retests` | 待复测隔离批次 |
| `GET /api/v1/queries/expired-releases` | 已过期发布许可 |

## Docker 构建（linux/amd64、linux/arm64）

`Dockerfile` 与 `benzhi.Dockerfile` 逐字一致，基于
`golang@sha256:53eeac89074db483fdf0ab3be1df32bf6e47562263d2d0d6baa7f26acb4957dd`：

```bash
./build_docker.sh        hwj-gowork-110:amd64 linux/amd64
./build_docker.sh        hwj-gowork-110:arm64 linux/arm64
./build_benzhi_docker.sh hwj-gowork-110:benzhi-amd64 linux/amd64
./build_benzhi_docker.sh hwj-gowork-110:benzhi-arm64 linux/arm64

# 无网络运行验收
docker run --rm -it --network none hwj-gowork-110:benzhi-amd64 bash
# 容器内：go test ./... && go vet ./... && go build ./...
```

## 示例流程（curl）

```bash
H='Content-Type: application/json'
B=http://localhost:8080/api/v1
curl -XPOST $B/sites -H "$H" -d '{"code":"DOME-A","name":"昆仑站","latitude":-80.25,"longitude":77.06,"altitude_m":4093}'
curl -XPOST $B/instruments -H "$H" -d '{"site_id":1,"code":"CryoCam-1","name":"低温相机","kind":"imager","temp_min_mK":250,"temp_max_mK":350}'
curl -XPOST $B/instruments/1/cryo -H "$H" -d '{"name":"稀释制冷机","target_temp_mK":300}'
curl -XPOST $B/cryo/1/precool -H "$H" -H 'X-Actor: op' -d '{"target_temp_mK":300,"deadline_at":"2026-09-01T00:00:00Z"}'
curl -XPOST $B/cryo/1/readings -H "$H" -H 'X-Actor: op' -d '{"temp_mK":300,"idempotency_key":"rd-1"}'
```

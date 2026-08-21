# ADR-009: Agent Runtime 存储迁移 PostgreSQL

> 状态：Accepted
> 日期：2026-08-20
> 里程碑：M8 前置架构优化（P0-1）
> 相关：[ADR-003 Durable Agent Run](ADR-003_DURABLE_AGENT_RUN.md)、[ADR-008 pgvector 迁移](ADR-008_PGVECTOR_MIGRATION.md)、[GRAPH_RUNTIME_SPEC.md](GRAPH_RUNTIME_SPEC.md)

## 背景

Runtime V2 的两份持久化状态目前都落在单实例本地 SQLite：

1. LangGraph Checkpoint：`AsyncSqliteSaver`，默认 `/tmp/enterprise-agent-platform/agent-checkpoints.sqlite3`；
2. RuntimeStore（Run/Interrupt/Command/Event Outbox 元数据）：同一 SQLite 文件内的 `runtime_v2_*` 四张表。

由此产生三个结构性约束：

- **单实例锁定**：Run 执行任务保存在进程内存（`RuntimeV2Service._tasks`），checkpoint 是本地文件，agent-service 无法水平扩容；M9 实验（Shadow/Canary）与多租户调度均以多实例为前提。
- **容器不可靠**：checkpoint 位于 `/tmp`（docker-compose 挂 volume 缓解，但本地裸跑即丢失），Durable Run 的"持久"承诺依赖部署纪律。
- **双库漂移**：业务状态在 PostgreSQL，Agent Runtime 状态在 SQLite，备份/恢复、租户隔离、审计各需两套口径。

## 决策

Agent Runtime 存储整体迁移至平台既有 PostgreSQL 16（复用 ADR-008 的组件收敛思路）：

- LangGraph Checkpoint 切换为 `AsyncPostgresSaver`（`langgraph-checkpoint-postgres`）；
- RuntimeStore 新增 PostgreSQL 实现 `PostgresRuntimeStore`（schema `agent_runtime`），与 SQLite 实现保持**接口 100% 同构**；
- 运行时后端由环境变量选择：

| 优先级 | 变量 | 行为 |
|---|---|---|
| 1 | `RUNTIME_DATABASE_URL` | 设置即启用 PG 后端（显式 DSN，含 docker-compose） |
| 2 | `RUNTIME_STORE_BACKEND=postgres` | 回退使用 `DATABASE_URL` 作为 DSN |
| 3 | 均未设置 | SQLite 兼容路径（本地开发/单元测试不变） |

### 理由

1. **水平扩展前提**：checkpoint 落 PG 后，任意实例可恢复任意 Run；配合后续 lease/抢占语义即可多实例部署。
2. **运维面收敛**：与 Go 业务库同实例、独立 schema；备份/恢复/监控单口径。
3. **锁语义升级**：SQLite `BEGIN IMMEDIATE` 是库级写锁；PG 用 `SELECT ... FOR UPDATE` 行级锁（`_locked_run`），进程内仍保留 asyncio 写锁防惊群，跨进程正确性由行锁保证。
4. **不新增组件**：PostgreSQL 已在 docker-compose 中，只新增两个 Python 依赖（`psycopg`、`langgraph-checkpoint-postgres`）。

### 代价与接受

- 每次写操作建立短连接，吞吐低于长连接池：当前 Run 并发为个位数，M9 前引入 `psycopg_pool` 即可，无需提前优化。
- SQLite 实现保留为测试/本地路径：两份实现存在漂移风险，通过同构接口 + PG 集成测试对冲（同 Go 侧 `*_integration_test.go` 惯例）。
- 历史本地 checkpoint 数据不迁移：单机原型阶段数据无保留价值，按 fixture 重放验证。

## 表设计

Schema `agent_runtime`，与 SQLite 版字段一一对应（时间戳保持 ISO 文本，序列列升为 BIGINT）：

- `runtime_v2_runs`：Run 状态机（`attempt`/`checkpoint_version`/`next_sequence`/`active_step_*`）。
- `runtime_v2_interrupts`：人工/外部中断与 resume_schema。
- `runtime_v2_commands`：Start/Resume/Cancel 幂等命令表（保留 partial unique index 保证单活跃 Resume）。
- `runtime_v2_event_outbox`：at-least-once 事件出站箱；`delivery_order BIGINT GENERATED ALWAYS AS IDENTITY`，head-of-line 投递语义与 SQLite 版一致。

新增运维索引：`runs(status)`（恢复扫描）、`outbox(run_id, status)`（投递扫描）。

事件契约不变：`checkpoint.saved` 的 `payload.backend` 由 `"sqlite"` 改为 `"postgres"`；Go 侧仅作展示用途，无解析依赖。

## 迁移步骤

1. `requirements.txt` 新增 `psycopg[binary]`、`langgraph-checkpoint-postgres`。
2. 新增 `app/runtime/store_pg.py`：同构接口 + DDL + FOR UPDATE 行锁。
3. `app/main.py` lifespan 按环境变量选择后端；checkpointer 引用挂到 `app.state`（供 ADR-010 reload 复用）。
4. docker-compose：agent-service 传入 `RUNTIME_DATABASE_URL`，移除 `CHECKPOINT_DB_PATH` 与 `agent_runtime_data` volume。
5. 回归验收：SQLite 路径全部单测通过；PG 路径集成测试（设置 `RUNTIME_DATABASE_URL` 时运行）覆盖 Start→Interrupt→Resume→终态 + outbox 投递。

## 验证

- `python -m unittest discover` 在无 PG 环境全绿（SQLite 兼容路径未回归）。
- 设置 `RUNTIME_DATABASE_URL` 后：Durable Run 全生命周期（含进程重启恢复、幂等重放、事件 outbox 顺序投递）在 PG 后端复现 SQLite 测试语义。

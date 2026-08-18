# Trace & Eval Architecture (M5)

> 文档状态：Active Specification
> 更新日期：2026-08-18
> 目标：为 Agent 构建全链路追踪系统和自动化评估体系，保障可观测性和质量。

## 1. 核心概念

*   **Trace (追踪)**：记录 Agent 系统中每一层级（Workflow, Run, Tool Call 等）的行为事件，形成完整的因果链，是系统的“黑匣子”。
*   **Eval (评估)**：基于 Trace 数据，自动计算 Agent 运行的各项指标，包括成本、效果、效率等，用于质量监控和优化。
*   **Replay (回放)**：使用历史 Trace 数据重新运行 Agent，用于调试、问题复现和算法对比。
*   **Shadow (影子流量)**：将线上生产流量复制一份给新版本 Agent，对比新旧版本的输出差异，不影响生产结果。
*   **Canary (金丝雀发布)**：逐步将生产流量切换到新版本，通过指标监控实现平滑发布和快速回滚。

## 2. 六层 Trace 模型

为了实现精细化的可观测性，Trace 划分为六个层级，覆盖了从业务流程到系统中断的全过程。

| 层级 (Layer) | 名称 | 描述 | 核心 Trace ID |
|---|---|---|---|
| **L1** | **Workflow** | 顶层业务流程 (如“费用报销审批流程”) | `workflow_instance_id` |
| **L2** | **Agent Run** | 单个 Agent 的执行过程 (如“处理张三的报销单”) | `agent_run_id` |
| **L3** | **Model Turn** | Agent 与 LLM 的每一次交互 (如“第一次调用生成 JSON”) | `turn_id` (由 `agent_run_id` + step number 组成) |
| **L4** | **Tool Call** | Agent 对外部工具/系统的调用 (如“查数据库”、“调 ERP”) | `tool_call_id` |
| **L5** | **Checkpoint** | Agent 执行过程中的关键状态保存点，用于断点恢复 | `checkpoint_id` |
| **L6** | **Interrupt** | 打断 Agent 运行的事件 (如“等待审批”、“用户输入”) | `interrupt_id` |

### 2.1. Trace 关联关系

所有层级的 Trace 通过 ID 形成父子关系：

```
Workflow (L1)
 └── Agent Run (L2)
      ├── Model Turn 1 (L3)
      │    └── Tool Call 1 (L4)
      ├── Checkpoint 1 (L5)
      ├── Interrupt 1 (L6) ---> 等待人工审批
      ├── Model Turn 2 (L3) ---> 审批通过后恢复
      │    └── Tool Call 2 (L4)
      └── ...
```

### 2.2. Trace 数据模型

为了存储和查询 Trace 事件，引入 `trace_events` 表。

```sql
CREATE TABLE trace_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id VARCHAR(128) NOT NULL,     -- 唯一 Trace ID (如 agent_run_id)
    layer VARCHAR(16) NOT NULL,          -- 'L1', 'L2', 'L3', 'L4', 'L5', 'L6'
    parent_id UUID,                      -- 父事件 ID (建立层级关系)
    event_type VARCHAR(64) NOT NULL,     -- 'start', 'end', 'error', 'pause', 'resume' 等
    payload_json JSONB,                  -- 事件详情 (如 LLM 输入/输出、工具参数)
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_ms INT,                     -- 事件耗时 (用于计算延迟)
    tenant_id UUID NOT NULL,
    metadata_json JSONB DEFAULT '{}'     -- 扩展元数据
);

-- 核心查询索引
CREATE INDEX idx_trace_events_trace_id ON trace_events(trace_id);
CREATE INDEX idx_trace_events_layer_timestamp ON trace_events(layer, timestamp);
CREATE INDEX idx_trace_events_tenant ON trace_events(tenant_id);
```

> 字段命名以 `DATABASE_SCHEMA.md` 为准（`payload_json` / `metadata_json`）。

### 2.3. 生产者与写入路径

Trace 事件通过 `internal/trace` 包的异步 Recorder 写入（channel 缓冲 + 后台批量 insert，best-effort，不阻塞主链路）：

| 层级 | 生产者 | 触发点 |
|---|---|---|
| L1 Workflow | Go workflow engine/service | 实例 created/completed/failed/cancelled |
| L2 Run | Go runtime handler | run started/succeeded/failed/cancelled |
| L3 Model Turn | Python `app/core/llm.py`（全图唯一 LLM 调用点） | model_turn start/end（fire-and-forget POST internal API） |
| L4 Tool Call | Go tool lifecycle | tool_call created/succeeded/failed/timed_out/dead_letter |
| L5 Checkpoint | Go runtime handler（接收 Python `checkpoint.saved` 事件） | checkpoint saved |
| L6 Interrupt | Go runtime handler | run interrupted/resumed |

### 2.4. API 契约

*   `POST /internal/v1/trace/events` — 批量追加（InternalServiceToken + X-Tenant-ID），供 Go 模块与 Python Agent Service 使用。
*   `GET /api/v1/traces/:trace_id` — 查询某 Trace 的全部事件（按 timestamp 升序），权限 `trace:read`。

## 3. Eval 评估体系

Eval 模块基于 Trace 数据，提供对 Agent 质量的量化评估。

### 3.1. 评估维度

| 维度 | 指标 | 计算方法 | 数据来源 (Trace Layer) |
|---|---|---|---|
| **成本 (Cost)** | `total_tokens` | 汇总所有 `Model Turn` 的 Token 使用量 | L3 |
| | `total_cost` | Token 数 * 单价 | L3 |
| **效率 (Efficiency)** | `total_duration` | 从 Run start 到 end 的总耗时 | L2 |
| | `llm_latency` | 单次 LLM 调用平均耗时 | L3 |
| **质量 (Quality)** | `tool_success_rate` | 成功的 `Tool Call` 比例 | L4 |
| | `approval_pass_rate` | 审批通过率 (业务指标) | L6 (结合业务数据) |
| **稳定性 (Stability)** | `error_rate` | 错误事件比例 | L1-L6 |
| | `interrupt_count` | 发生打断的次数 | L6 |

### 3.2. API 契约

`EvalService` 提供 API 用于生成评估报告。

```go
// 请求
type EvalReportRequest struct {
    TenantID    string
    StartTime   time.Time
    EndTime     time.Time
    Filters     map[string]string // 如 {"agent_id": "xxx", "workflow_id": "yyy"}
    Metrics     []string          // 要计算的指标列表
}

// 响应
type EvalReportResponse struct {
    Summary map[string]float64    // 汇总指标，如 {"total_cost": 12.34, "avg_duration": 5000}
    Details []MetricDetail        // 详细的分维度数据
}

type MetricDetail struct {
    MetricName  string
    Value       float64
    Trend       string // 'up', 'down', 'stable'
    Comparison  float64 // 与上一周期的对比
}
```

*   `POST /api/v1/eval/reports` — 生成评估报告，权限 `eval:read`。
*   请求体为 `EvalReportRequest` 的 JSON 形式（`tenant_id` 由服务端从认证上下文注入）。

### 3.3. 指标数据源与计算窗口

| 指标 | 数据源 (表) | 计算方式 |
|---|---|---|
| `total_tokens` | `agent_run_logs.usage_json` | `SUM(usage_json->>'total_tokens')`，排除 shadow/replay Run |
| `total_cost` | `agent_run_logs.usage_json` | `SUM(usage_json->>'cost')`，排除 shadow/replay Run |
| `total_duration` / `avg_duration` | `agent_run_logs.duration_ms` | 终态 Run 的总/平均耗时 |
| `llm_latency` | `trace_events` (L3, `event_type='end'`) | `AVG(duration_ms)` |
| `tool_success_rate` | `tool_calls` | `succeeded / (succeeded + failed + indeterminate)`，窗口内终态调用（`timed_out` 落库为 `indeterminate`；`dead_letter` 为 `is_dead_letter` 标志位，已含于 failed/indeterminate，不重复计数） |
| `approval_pass_rate` | `approvals` | `approved / (approved + rejected)` |
| `error_rate` | `agent_run_logs` | `failed / total 终态 Run` |
| `interrupt_count` | `trace_events` (L6, `event_type='interrupted'`) | `COUNT(*)` |

*   **对比周期**: `Comparison` 与 `Trend` 通过比较当前窗口 `[start, end)` 与等长上一窗口 `[start-(end-start), start)` 计算。`Comparison` 为相对变化百分比（`(cur-prev)/prev*100`，prev=0 时为 0）；`Trend` 在相对变化 <1% 时视为 `stable`。
*   **排除规则**: `agent_run_logs LEFT JOIN agent_runs ar ON arl.durable_run_id = ar.id`，`ar.metadata_json` 标记 `shadow=true` 或 `replay=true` 的 Run 不计入生产指标（`agent_runs.metadata_json` 由 migration 027 添加，M5-C 的 Shadow/Replay Run 写入该标记）。
*   **窗口字段**: Run 类指标按 `finished_at` 落窗；Tool Call 按 `created_at`；Approval 按 `decided_at`；Trace 类（llm_latency / interrupt_count）按 `timestamp`。

## 4. Replay / Shadow / Canary

这三个机制用于保障 Agent 的迭代质量和线上稳定性。

### 4.1. Replay (回放)

*   **场景**: 开发人员需要复现某个线上 Bug 或验证新算法。
*   **实现**:
    1.  根据 `trace_id` 提取该 Run 的所有 Trace 事件。
    2.  获取历史的 `Context` 快照 (M4)。
    3.  使用新的 Agent 配置重新执行 `Model Turn` 步骤。
    4.  对比新旧输出，生成 Diff 报告。

> **M5-C MVP 范围**：V1 实现为 *输入级回放* —— 以源 Run 的首个 model Step 输入（`agent_run_steps.input_summary_json`，缺失时回退 `agent_run_logs.input_summary_json`）重新发起一次独立 Run（`metadata_json.replay=true`，可指定不同 `graph_key`），对 `output_summary_json` 做字段级 Diff。Checkpoint 级逐步回放（从断点恢复执行）推迟至后续里程碑。

### 4.2. Shadow (影子流量)

*   **场景**: 验证新版本 Agent 的性能，但不影响线上决策。
*   **实现**:
    1.  **流量复制**: 在 `Agent Gateway` 层，将生产流量异步复制一份。
    2.  **影子执行**: 复制的流量转发给新版本 Agent。
    3.  **结果对比**: 对比新旧版本的输出结果、耗时、Token 消耗等指标。
    4.  **报告生成**: 生成 Shadow 测试报告，供发布决策参考。

### 4.3. Canary (金丝雀发布)

*   **场景**: 控制新版本 Agent 的上线节奏，降低风险。
*   **实现**:
    1.  **流量切分**: 配置流量切分规则 (如 1% -> 5% -> 20% -> 100%)。
    2.  **指标监控**: 实时监控新版本的核心指标 (错误率、耗时、成本)。
    3.  **自动回滚**: 若指标超过预设阈值 (如错误率 > 1%)，自动将流量切回旧版本。
    4.  **人工确认**: 每一步切分都需要人工审批确认。

### 4.4. 数据模型

统一存储于 experiment 相关表（business-domain neutral），均含 `tenant_id` 边界。
（migration 027 属 M5-B：`agent_runs.metadata_json` + `eval:read` 权限；experiment 表为 migration 028。）

**replay_sessions** (migration 028)

| Column | Type | Notes |
|---|---|---|
| id | uuid PK | |
| tenant_id | uuid | Tenant boundary |
| source_run_id | varchar(128) | 被回放的原始 Run |
| replay_run_id | varchar(128) | 回放产生的新 Run |
| graph_key | varchar(128) | 回放使用的 graph（可与原始不同，用于算法对比） |
| status | varchar(32) | `pending` → `running` → `completed` / `failed` |
| diff_json | jsonb | 输出 Diff 报告（字段级对比） |
| created_by | varchar(128) | |
| created_at / updated_at | timestamptz | |

**shadow_rules** (migration 028)

| Column | Type | Notes |
|---|---|---|
| id | uuid PK | |
| tenant_id | uuid | |
| business_app_code | varchar(64) | 匹配条件 |
| graph_key | varchar(128) | 匹配条件（基线版本） |
| shadow_graph_key | varchar(128) | 影子执行的目标版本 |
| traffic_percent | int | 0-100，按 run_id 哈希取样 |
| status | varchar(16) | `active` / `stopped` |
| created_by / created_at / updated_at | | |

**shadow_executions** (migration 028)

| Column | Type | Notes |
|---|---|---|
| id | uuid PK | |
| tenant_id | uuid | |
| rule_id | uuid FK → shadow_rules | |
| primary_run_id | varchar(128) | 生产 Run |
| shadow_run_id | varchar(128) | 影子 Run（`metadata_json.shadow=true`，不挂 workflow） |
| comparison_json | jsonb | 输出/耗时/成本对比 |
| created_at | timestamptz | |

**canary_releases** (migration 028)

| Column | Type | Notes |
|---|---|---|
| id | uuid PK | |
| tenant_id | uuid | |
| business_app_code | varchar(64) | 匹配条件 |
| graph_key | varchar(128) | 基线版本 |
| candidate_graph_key | varchar(128) | 候选版本 |
| stages | jsonb | 流量阶梯，如 `[1,5,20,100]` |
| current_stage_index | int | 当前阶梯索引 |
| max_error_rate | float8 | 自动回滚阈值 |
| min_sample_size | int | 触发回滚所需最小样本数 |
| status | varchar(32) | `active` → `promoted` / `rolled_back` |
| created_by / created_at / updated_at | | |

### 4.5. API 契约（权限 `experiment:manage`）

*   `POST /api/v1/replays` — 发起回放（body: `source_run_id`, 可选 `graph_key`）；异步执行，返回 replay session。
*   `GET /api/v1/replays/:id` — 查询回放状态与 Diff 报告。
*   `POST /api/v1/shadow-rules` / `GET /api/v1/shadow-rules` / `POST /api/v1/shadow-rules/:id/stop` — Shadow 规则管理。
*   `GET /api/v1/shadow-executions` — 影子执行列表（含对比结果）。
*   `POST /api/v1/canary-releases` / `GET /api/v1/canary-releases` / `GET /api/v1/canary-releases/:id` — Canary 发布管理。
*   `POST /api/v1/canary-releases/:id/advance` — 人工确认进入下一阶梯（不可跳级）。
*   `POST /api/v1/canary-releases/:id/promote` / `POST /api/v1/canary-releases/:id/rollback` — 全量发布 / 回滚。
*   `POST /api/v1/canary-releases/:id/check` — 触发指标检查（样本数足够且超阈值时自动回滚）。

### 4.6. 安全规则

*   Shadow Run 与 Replay Run 均为**独立 Run**：不设置 `workflow_instance_id`/`node_instance_id`（不触发工作流节点推进），`metadata_json` 标记 `shadow=true` / `replay=true`；此类 Run **不写 `agent_run_logs` 兼容行**（天然不计入 Eval 生产指标）。
*   流量切分决策点收敛在 Agent Gateway 调度处（单一收口，V1 Execute 与 V2 StartV2 双路径生效）：Canary 按 `hash(run_id) % 100 < 当前阶梯百分比` 命中候选版本；Shadow 同理按规则取样。影子 Run 自身不再二次分叉（实验 Run 直投，不经过 Router）。
*   V1 同步路径的 Shadow 复制为 best-effort：经 Runtime V2 发起影子 Run，V2 未配置时跳过并记录日志；Canary 路由替换在两条路径均生效。
*   Canary 自动回滚必须同时满足：窗口内候选版本样本数 ≥ `min_sample_size` 且错误率 > `max_error_rate`。
*   Canary 阶梯只能逐级递增（advance 一次一级），不允许跳级；`promote` 等价于直接进入 100%。
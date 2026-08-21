# Tech Design 模板 (Tech Design Template)

> **说明**：本模板用于规划和记录新功能的技术实现细节（个人版）。在开始编码前，请先按此模板完成设计，确保思路清晰、影响可控。
> **存放位置**：`docs/00_TEMPLATES/` 目录下，作为独立文件，不与正式架构文档 (`docs/02_ARCHITECTURE/` 等) 混淆。
> **使用方式**：复制本文件，重命名为 `TECH_DESIGN_[FeatureName].md`，然后填充内容。

---

## 1. 要解决的问题 (Problem Statement)

**简述这个技术设计要解决的核心问题。**
- **业务背景**：是什么触发了这个需求？（例如：用户在审批时找不到对应的按钮）
- **问题描述**：当前痛点是什么？（例如：Workflow 状态卡死在 pending，无法推进）
- **预期收益**：解决后带来的直接好处？（例如：审批流成功率从 80% 提升到 99.9%）

---

## 2. 技术方案 (Technical Solution)

### 2.1. 核心思路

**一句话概括技术实现路径。**
> 例如：在 Go 后端的 Workflow 模块增加 `RetryStuckNode` 接口，通过修复状态机逻辑，重新触发异步任务队列。

### 2.2. 文件改动清单 (Files to Change)

列出所有需要修改的文件，按影响范围分组。

| 文件路径 | 改动类型 | 改动说明 |
| :--- | :--- | :--- |
| `go-platform/internal/workflow/engine.go` | 修改 | 增加 `RetryStuckNode` 方法的实现 |
| `go-platform/internal/workflow/handler.go` | 修改 | 增加 `POST /api/v1/workflow-instances/:id/retry` 路由的处理逻辑 |
| `go-platform/internal/workflow/service.go` | 修改 | 在 `Service` 接口增加 `RetryStuckNode` 方法定义 |
| `go-platform/internal/workflow/engine_test.go` | 修改 | 增加对 `RetryStuckNode` 的单元测试 |

### 2.3. API 设计 (API Design)

**详细描述需要变更的 API 契约。**

#### 请求 (Request)

`POST /api/v1/workflow-instances/:id/retry`

- **参数**：
  - `id` (string, 必填): Workflow 实例 ID。
  - `node_id` (string, 可选): 指定要重试的节点 ID。如果不传，自动寻找第一个卡在 `pending` 的节点。

#### 响应 (Response)

```json
{
  "status": "success",
  "data": {
    "workflow_instance_id": "wf-12345",
    "retried_node_id": "node-67890",
    "new_status": "running"
  }
}
```

### 2.4. 数据库变更 (Database Schema Changes)

**如果涉及数据库结构变更，详细描述。**
> 注：如果不需要变更，请明确写“无”。

- **新增表**：无
- **修改表**：在 `workflow_instances` 表增加 `last_retry_time` 字段 (TIMESTAMP, nullable)。
- **迁移脚本**：需创建 `go-platform/migrations/040_add_retry_time.up.sql` 和 `go-platform/migrations/040_add_retry_time.down.sql`。

---

## 3. 非功能性考虑 (Non-functional Considerations)

### 3.1. 性能 (Performance)

- **预期影响**：重试接口会触发一次数据库写操作和一次队列消息投递，预计耗时 < 50ms。
- **性能测试**：（可选）已用 `k6` 脚本模拟 100 QPS，CPU 占用 < 5%。

### 3.2. 安全 (Security)

- **权限控制**：必须使用 `workflow:retry` 权限保护该接口，防止未授权重试。
- **数据校验**：校验 `workflow_instance_id` 对应的流程确实属于当前租户。
- **防重放**：复用已有的幂等性机制，防止重复点击重试按钮导致多次触发。

### 3.3. 可靠性 (Reliability)

- **幂等性**：重试成功后再次调用接口应返回成功，不会产生额外副作用。
- **超时与重试**：如果底层队列投递失败，Go 服务应在 10 秒内返回错误，并记录详细日志。
- **回滚方案**：如果上线后发现问题，可通过撤销 API 路由或回滚代码版本解决（数据库字段保留，不回滚）。

---

## 4. 测试计划 (Test Plan)

- **单元测试 (Unit Test)**：
  - 在 `engine_test.go` 中测试 `RetryStuckNode` 的逻辑，覆盖成功、失败（节点不存在）、非法状态等场景。
- **集成测试 (Integration Test)**：
  - 新增 `*_integration_test.go` 测试，模拟完整的 API 请求，验证数据库状态变更和异步任务队列消息。
- **端到端测试 (E2E Test)**：
  - 在前端测试中，验证用户点击“重试”按钮后，界面状态正确刷新。

---

## 5. 备选方案 (Alternatives Considered)

**列出你考虑过但最终放弃的方案及其原因。**

- **方案 A：前端轮询刷新**
  - 描述：不修改后端，让前端在卡住时自动轮询刷新状态。
  - 放弃原因：治标不治本，无法真正恢复执行，增加了数据库查询压力。

- **方案 B：利用 Asynq 重试机制**
  - 描述：依赖 Asynq 自带的重试逻辑，而不是手动触发。
  - 放弃原因：Asynq 重试是针对任务失败的，而当前场景是任务“未被调度”（pending），不适合。

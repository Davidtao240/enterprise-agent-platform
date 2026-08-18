# 8 月 1 日：`/start` 与 Asynq 异步链

> 启动工作流的完整异步链路：从 HTTP 请求 → Service 入队 → Worker 接力推进

---

## 一、核心思维链

```
为什么要异步？ → 异步组件职责 → 链路怎么走？ → 谁派任务？ → 接力推进？
  (超时/解耦)   (Redis+Asynq    (Handler→入队     (Service的       (OnNodeCompleted
                 5个角色)        →分界→Worker)     3个入队场景)    →GetNextNodes)
```

---

## 二、为什么要异步？（1 分钟回答）

| 原因 | 具体说明 |
|---|---|
| **超时边界** | `human_review` 等审批几小时到几天，HTTP 早断；`agent_graph` 调 LLM 几分钟，浏览器超时 |
| **资源与扩展** | HTTP 长连接占 goroutine+连接+DB 连接；Worker 可独立水平扩展（10×N 并发） |
| **职责分离** | HTTP = 命令通道（点火+确认），Worker = 执行通道（接力推进），前端 GET 轮询进度。解耦后各自容错 |

一句话：**HTTP 是启动按钮，按下即返回；Worker 是幕后工人慢慢干活；前端轮询看进度。**

---

## 三、异步组件 5 角色速记

| 组件 | 类比 | 职责 |
|---|---|---|
| **Redis** | 排队夹子 | 内存存储待办任务单（zset/list 结构） |
| **Asynq Client** | 店员 | 序列化任务单 → 塞进 Redis 队列 |
| **Queue** | 队伍 | 排队通道，本项目 `workflow:10, default:1`（workflow 优先） |
| **Asynq Server** | 厨师长 | 从 Redis 拉任务，调度 goroutine 并发处理（Concurrency=10） |
| **Asynq Handler** | 厨师 | 处理任务的函数，switch node_type 分发 |

⚠️ **注意两个 Handler 的区别**：Gin Handler 处理 HTTP 请求，Asynq Handler 处理队列任务。

---

## 四、Node 和 Edge 基础

### 4.1 模板存储位置

Node 和 Edge **不单独建表**，打包存在 `workflow_templates.definition_json`（JSONB 字段）。

### 4.2 财务模板示例（简化版）

```
Nodes:  upload(file_upload) → agent_graph(agent_graph) → human_review(human_review) → archive(system)
Edges:  upload→agent_graph(无条件)  agent_graph→human_review(succeeded)  human_review→archive(approved)
```

**id vs type**：`upload` 是节点 id（名字），`file_upload` 是节点 type（工种）。4 种 type 应对所有节点。

### 4.3 入口节点 / 下一节点

| 找什么 | 规则 | 代码位置 |
|---|---|---|
| **入口节点** | edges 里没有入边的节点 | [engine.go#L122-L135](go-platform/internal/workflow/engine.go#L122-L135) |
| **下一节点** | 遍历 edges，找 `from == 当前节点 id` 且 `when` 匹配的边 | [engine.go#L145-L164](go-platform/internal/workflow/engine.go#L145-L164) |

### 4.4 模板 vs 节点实例

| | 模板（Template） | 节点实例（NodeInstance） |
|---|---|---|
| 谁创建 | 管理员/种子数据 | POST 创建工作流时"复印" |
| 数量 | 每种流程 1 个 | 每个工作流实例 N 个 |
| 存哪 | `definition_json` | `workflow_node_instances` 表 |
| 变不变 | **固定** | **动态**（pending→running→succeeded） |
| Worker 改吗 | **只读**（查流转） | **改**（更新状态） |

---

## 五、`/start` 完整链路

### 5.1 同步部分（HTTP goroutine）

```
POST /workflow-instances/{id}/start
  → Handler.StartInstance
  → Service.StartWorkflow
      ① 查实例，校验 status == draft
      ② UPDATE instance: draft → running
      ③ 查模板 → 解析 definition_json
      ④ GetEntryNodes() 找入口节点
      ⑤ EnqueueExecuteNode(入口节点)  ←★写Redis
  → HTTP 200 {id, status: "running"}  ←★立刻返回
```

### 5.2 异步部分（Worker goroutine）

```
Asynq Server 拉任务 → handleExecuteNode
    ① 解开任务单
    ② UPDATE node: pending → running
    ③ switch node_type 分发
    ④ 节点完成 → OnNodeCompleted
         查模板 GetNextNodes → 有下一节点 → 再入队（接力）
                         → 无下一节点 → 全完成 → UPDATE instance approved
```

### 5.3 同步/异步分界图

```
  同步（HTTP goroutine）                    异步（Worker goroutine）
  ──────────────────────                    ────────────────────────
  Service.StartWorkflow()
    ① 校验 + UPDATE draft→running
    ② 找入口节点
    ③ client.Enqueue(任务单)                Asynq Server 拉到任务
         │                                        │
         ▼                                        ▼
  HTTP 200 返回  ←────────★分界线★────────  handleExecuteNode()
  用户已拿到响应                              节点执行 → 接力入队
```

**分界线 = `client.Enqueue()` 返回的瞬间。** HTTP 返回时 Worker 可能还没开始消费第一个节点。

---

## 六、任务单是谁下发的？（Service 3 个入队点）

| 场景 | 调用方 | 时机 | 代码位置 |
|---|---|---|---|
| **点火** | StartWorkflow | 启动工作流时，给入口节点派活 | [service.go#L247](go-platform/internal/workflow/service.go#L247) |
| **接力** | OnNodeCompleted | 节点完成后，给下一节点派活 | [service.go#L436](go-platform/internal/workflow/service.go#L436) |
| **复活** | RetryNode | 用户手动重试失败节点 | [service.go#L346](go-platform/internal/workflow/service.go#L346) |

**Service = 大脑（决策），Worker = 手脚（执行）。**

---

## 七、Worker node_type 分发

[worker.go#L127-L142](go-platform/internal/workflow/worker.go#L127-L142) — 普通 switch 语句：

| node_type | 处理函数 | 做什么 |
|---|---|---|
| `file_upload` | handleFileUpload | 直接标记 succeeded → 推进 |
| `agent_graph` | handleAgentGraph | 组装 payload → Gateway.Execute → 调 Python LLM |
| `human_review` | handleHumanReview | 创建 approval_task → 节点 waiting_review |
| `system` | handleSystem | archive_result → 归档 output → 实例 archived |

---

## 八、`ExecuteNodePayload` 字段说明

[model.go#L158-L165](go-platform/internal/workflow/model.go#L158-L165)

| 字段 | 必填 | 来源 | 用途 |
|---|---|---|---|
| **WorkflowInstanceID** | ✅ | `inst.ID` | 定位工作流实例 |
| **NodeInstanceID** | ✅ | `node_instance.ID` | 定位节点实例 |
| **NodeType** | ✅ | `node_instance.node_type` | switch 分发 |
| NodeKey | ❌ | `node_instance.node_key` | OnNodeCompleted 匹配 GetNextNodes |
| GraphKey | ❌ | `inst.graph_key` | agent_graph 专用，查 graph_registry |
| **TraceID** | ✅ | `inst.trace_id` | 全链路追踪 |

---

## 九、Trace 跨队列传播

TraceID 在 CreateInstance 时生成（`uuid.New()` → `inst.trace_id`），之后全程携带：

```
HTTP X-Trace-Id → DB workflow_instances.trace_id
    → ExecuteNodePayload.TraceID（Redis 任务单）
    → Worker 取出 → Gateway X-Trace-Id 请求头（传 Python）
    → agent_run_logs.trace_id + audit_logs.trace_id
```

---

## 十、一次 `/start` 请求日志对照

| # | 来源 | 日志 | 状态变化 |
|---|---|---|---|
| 1 | HTTP | `POST /workflow-instances/{id}/start 200` | 请求到达 |
| 2 | Service | (DB 写入) | instance draft → running |
| 3 | Worker Client | `[worker] enqueued node xxx (type=file_upload)` | 入队 |
| 4 | Worker Server | `[worker] processing node xxx` | node pending → running |
| 5 | Worker | (file_upload succeeded) | node succeeded |
| 6 | Worker | `[worker] enqueued node xxx (type=agent_graph)` | 接力入队 |
| 7 | Gateway | `[gateway] calling Python for graph=...` | 调 Python |
| 8 | Gateway | `[gateway] agent run xxx completed` | agent 完成 |
| 9 | Worker | `[worker] enqueued node xxx (type=human_review)` | 接力入队 |
| ... | ... | (human_review 等待人工) | node waiting_review |

---

## 十一、Go 语法：`func (w *Worker) EnqueueExecuteNode(payload *Xxx) error`

| 部分 | 含义 |
|---|---|
| `func` | 关键字，定义方法 |
| `(w *Worker)` | 接收者：此方法属于 Worker 类型，`*` 是指针（操作原件） |
| `EnqueueExecuteNode` | 方法名，大写=公开 |
| `(payload *Xxx)` | 参数：接收一个任务单指针 |
| `error` | 返回值：nil=成功，非nil=失败 |

---

## 十二、任务入队成功 ≠ 业务执行成功

| 阶段 | 可能失败 |
|---|---|
| 入队后 Worker 拉前 | Redis 宕机未持久化 → 任务丢失 |
| Worker 拉取后处理中 | Worker 崩溃 → 任务滞留 active，等 lease 超时回收 |
| handleAgentGraph 中 | Python 超时/5xx → 节点 failed |
| 节点成功后推进 | OnNodeCompleted 入队下一批失败 → 流程卡住 |

✅ **业务成功标志**：`node_instance.status = succeeded` 且 `output_json` 已写入 DB。

---

## 十三、状态修改 + 重复执行副作用

**状态只有 Service 层通过 Repository 修改，Worker 不直接 SQL。**

| 重复执行场景 | 副作用 |
|---|---|
| agent_graph 重复执行 | **重复消耗 LLM token（花钱）** |
| human_review 重复执行 | **重复创建审批任务（多条记录）** |
| OnNodeCompleted 重复调用 | **重复入队下一批 → 雪崩重复** |
| 审计日志重复 | 审计失真 |

防护：`asynq.MaxRetry(0)`（Asynq 不自动重试）；但**缺少幂等检查是已知风险**。

---

## 十四、工程验收指引

| # | 条目 | 怎么做 |
|---|---|---|
| 1 | HTTP 及时返回 + 后台执行 | POST /start → 200 几十毫秒 → 等几秒查 node 状态变了 |
| 2 | 入队前后状态对照 | 前：instance draft + nodes pending；后：instance running + 入口节点推进；保存 Worker log |
| 3 | ExecuteNodePayload ID 对应 DB | 取 log 中 NodeInstanceID → `SELECT * FROM workflow_node_instances WHERE id=xxx` |
| 4 | Worker 未运行时表现 | 停 Worker → 发 /start → HTTP 200 正常 → nodes 全停 pending → 启 Worker → 开始推进 |
| 5 | Trace 关联 | HTTP trace_id → 查 workflow_instances → agent_run_logs → audit_logs，四处能串 |

---

## 十五、个人能力验收 5 条

1. ✅ 分界线 = `client.Enqueue()` 返回瞬间
2. ✅ Redis/Client/Queue/Server/Handler 职责（第三节）
3. ✅ 入队成功 ≠ 业务成功（第十二节）
4. ✅ 状态由 Service 修改 + 重复副作用（第十三节）
5. ✅ 长 Workflow 不占 HTTP 的三条理由（第二节）

---

## 十六、10 个学习问题速查

| # | 问题 | 答案 | 代码 |
|---|---|---|---|
| 1 | Handler 调哪个 Service？ | `h.svc.StartWorkflow()` | [handler.go#L114-L124](go-platform/internal/workflow/handler.go#L114-L124) |
| 2 | draft→running 在哪持久化？ | `repo.UpdateInstanceStatus(running)` 写 PG | [service.go#L211-L214](go-platform/internal/workflow/service.go#L211-L214) |
| 3 | 入口 Node 如何确定？ | edges 无入边的节点 | [engine.go#L122-L135](go-platform/internal/workflow/engine.go#L122-L135) |
| 4 | ExecuteNodePayload 字段？ | 6 个（第八节） | [model.go#L158-L165](go-platform/internal/workflow/model.go#L158-L165) |
| 5 | 任务在哪放 Redis？ | Service→Worker.EnqueueExecuteNode→client.Enqueue | [service.go#L246-L261](go-platform/internal/workflow/service.go#L246-L261) |
| 6 | HTTP 哪步返回？ | 入口节点入队成功后立刻返回 200 | [service.go#L264](go-platform/internal/workflow/service.go#L264) |
| 7 | 返回后哪个 Worker 函数执行？ | `handleExecuteNode`，Asynq Server 回调 | [worker.go#L111](go-platform/internal/workflow/worker.go#L111) |
| 8 | Worker 如何分发？ | switch node_type → 4 个 case | [worker.go#L127-L142](go-platform/internal/workflow/worker.go#L127-L142) |
| 9 | agent_graph 如何进 Gateway？ | handleAgentGraph→组装 payload→Gateway.Execute | [worker.go#L164-L235](go-platform/internal/workflow/worker.go#L164-L235) |
| 10 | Trace 如何跨队列？ | inst.trace_id → payload.TraceID → JSON → Worker → Gateway 头 → 日志 | [service.go#L138](go-platform/internal/workflow/service.go#L138) |

---

## 十七、3 分钟脱稿口述

> `POST /workflow-instances/{id}/start` 启动工作流实例。
>
> 请求经中间件到 Handler，调用 **Service.StartWorkflow**：校验 draft→UPDATE running→查模板→找入口节点→**Worker.EnqueueExecuteNode** 把任务单塞 Redis "workflow" 队列。**此时 HTTP 立刻返回 200**。
>
> 之后独立 goroutine 的 Asynq Server 从 Redis 拉任务，调用 **handleExecuteNode**：更新节点 running→switch node_type 分发（file_upload 直接成功，agent_graph 调 Python，human_review 建审批任务等待，system 归档）。
>
> 节点完成后调 **OnNodeCompleted**：查 edges 找下一节点→再入队接力→全完成则实例 approved。TraceID 从 DB 取出，全程写入任务单、Gateway 头和各类日志。
>
> **总结**：HTTP 点火即返回，Worker 接力干活，前端轮询进度。

---

## 十八、自测清单

### 概念层（不讲代码）
- [ ] 异步三条理由
- [ ] 5 个组件职责
- [ ] 同步/异步分界线

### 数据层（对照模板）
- [ ] Node/Edge 存哪
- [ ] 入口/下一节点规则
- [ ] 模板 vs 节点实例区别

### 代码层（看文件）
- [ ] Service 3 个入队点
- [ ] Worker 4 个分发 case
- [ ] Go 方法签名各部分含义

### 验收层
- [ ] 10 个学习问题逐条答
- [ ] 4 个交付物能画/能写
- [ ] 5 个能力验收能口述
- [ ] 5 个工程验收知道怎么做

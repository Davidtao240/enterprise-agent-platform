# 2026-07-31—2026-08-09 后端到 Agent Runtime 过渡执行卡

上位计划：[[03_九月前执行计划与证据台账]]

项目方向：[[Enterprise_Agentic_System_项目目标与演进规划]]

前序周卡：[[2026-07-14_本周后端学习与功能卡]]

统一验收标准：[[工程交付与个人能力闭环标准]]

执行周期：2026-07-31 至 2026-08-09

项目：`enterprise-agent-platform`

投入：每天 3–5 小时，任何时候只保留一个主任务。

---

## 一、当前起点

### 已验证

- [x] W1-D1 财务正常主链路运行到 `archived`。
- [x] W1-D2 保存 Workflow、Nodes、Agent Run、Approval、Audit 证据。
- [x] 汇总断言输出 `true`。
- [x] 初步理解同步 GET 的 Route → Middleware → Handler → Service → Repository → PostgreSQL。
- [x] 能区分用户、租户、Request Trace 和 Workflow Trace。

### 当前缺口

- [ ] `POST /workflow-instances` 创建链尚不能脱稿讲解。
- [ ] `/start`、Asynq、Worker 与 HTTP 返回边界尚未形成完整心智模型。
- [ ] Go Gateway → Python Agent Service → LangGraph 尚未完整追踪。
- [ ] Agent Service 故障、重试与恢复尚未验证。
- [ ] 幂等、重复审批和事务边界尚未通过本人设计的测试收口。
- [ ] Graph State、Checkpoint、HITL、Structured Output 尚未形成个人实验。

### 范围审核结论（2026-07-31）

结论：任务范围适合当前“先建立后端主链必要工程认知，再进入 Agent Runtime”的阶段，不需要增加新的完整业务场景。

本次审核只做两项校准：

1. 先学习正常的 Go Gateway → Python LangGraph 调用链，再做 Agent Service 故障实验。不了解正常链路就直接制造故障，容易只记录现象，无法准确定位失败边界。
2. 8 月 7—9 日的 Runtime 任务限定为最小 Fixture 和边界实验，不在本卡内建设完整 Memory 服务，也不重写 Finance 主链。

当前能力边界：

```text
需要达到：
同步写请求、事务、异步队列、跨服务契约、失败恢复、幂等、审批一致性能够解释并验证。

暂不要求：
掌握 Gin/pgx/Redis 内部源码、设计完整通用 Runtime、生产级长期 Memory、多 Agent 调度平台。
```

---

## 二、本卡主能力闭环

```text
同步 POST 创建
→ 异步启动与节点执行
→ Go 调用 Python Agent Runtime
→ 故障、重试与恢复
→ 人工审批与归档
→ 幂等和事务收口
→ Graph State / Checkpoint / HITL
```

本卡不是要求读懂全部代码，而是要求每条链都能：

1. 找到入口。
2. 说出每层输入输出。
3. 标记状态和 ID 变化。
4. 运行一个正常或失败案例。
5. 保存证据。
6. 脱稿讲解。

每个任务必须同时通过“工程验收”和“个人能力验收”。代码或实验完成但本人讲不清时，只能标记为“已实现待验证”，不能标记为“已验证”。详细规则见 [[工程交付与个人能力闭环标准]]。

Agent Harness / Runtime 的完整能力树见 [[03_九月前执行计划与证据台账#Agent Harness / Runtime 能力模型]]。

---

## 三、7 月 31 日：同步 POST 创建链

主请求：

```text
POST /api/v1/workflow-instances
```

学习问题：

- [ ] 路由注册在哪里，需要什么权限？
- [ ] JWT 提供哪些 `user_id`、`tenant_id`？
- [ ] Handler 读取哪些 Header 和 Request Body？
- [ ] Service 如何根据 `business_app_code + workflow_template_key` 查询 Template？
- [ ] `definition_json` 如何解析为 Nodes 和 Edges？
- [ ] 如何创建一条 `workflow_instances`？
- [ ] 如何创建多条 `workflow_node_instances`？
- [ ] Workflow Trace 和各类数据库 ID 在哪里生成？
- [ ] Idempotency-Key 当前怎样参与创建？
- [ ] HTTP Response 返回哪些字段？

必须交付：

- [ ] 一张手画调用链。
- [ ] 一张“请求字段 → Go 结构体 → 数据库字段”映射表。
- [ ] 创建前后数据库记录或 API 结果对照。
- [ ] 3 分钟脱稿口述。

完成标准：

```text
能解释 Template 如何被实例化为 Instance 和 Node Instances，
但不要求理解 pgx、Gin 或 PostgreSQL 的全部内部实现。
```

### 工程验收

- [ ] 使用真实请求成功创建一个新的 Workflow Instance。
- [ ] Instance 和 Node Instances 的数据库/API 状态与 Template 定义一致。
- [ ] 至少验证一个错误 Template、非法请求或重复请求案例。
- [ ] Idempotency-Key 和事务边界有实际证据，不只停留在代码阅读。
- [ ] 请求、响应、状态前后对照和 Trace 已保存。

### 个人能力验收

- [ ] 能脱稿画出 Route → Middleware → Handler → Service → Repository → PostgreSQL。
- [ ] 能说明 Template、Instance、Node Instance 的关系与 ID 生命周期。
- [ ] 能指出创建链主要文件和核心函数。
- [ ] 能解释为什么创建 Instance 和 Nodes 需要事务保护。
- [ ] 能根据一次失败响应判断问题更可能位于参数、业务规则还是持久化层。

---

## 四、8 月 1 日：`/start` 与 Asynq 异步链

主请求：

```text
POST /api/v1/workflow-instances/{id}/start
```

学习问题：

- [ ] Handler 调用哪个 Service？
- [ ] `draft → running` 在哪里持久化？
- [ ] 入口 Node 如何确定？
- [ ] `ExecuteNodePayload` 包含哪些 ID 和字段？
- [ ] 在哪里把任务放进 Redis/Asynq？
- [ ] HTTP 请求在哪一步返回？
- [ ] HTTP 返回后哪个 Worker 函数继续执行？
- [ ] Worker 如何根据 `node_type` 分发？
- [ ] `agent_graph` 节点如何进入 Go Agent Gateway？
- [ ] Trace 如何跨队列继续传播？

必须交付：

- [ ] 同步部分与异步部分分界图。
- [ ] `ExecuteNodePayload` 字段说明。
- [ ] 一次 `/start` 请求、队列和 Worker 日志对照。
- [ ] 一分钟回答：“为什么 HTTP 不等待 Workflow 全部完成？”

### 工程验收

- [ ] `/start` 后 HTTP 能及时返回，后台节点继续执行。
- [ ] 保存入队前后的 Workflow、Node 状态和 Worker 日志。
- [ ] `ExecuteNodePayload` 中关键 ID 能与数据库记录对应。
- [ ] 至少验证一次 Worker 未运行或 Redis/队列异常时的可观察表现。
- [ ] Trace 能从 HTTP 请求关联到后台任务。

### 个人能力验收

- [ ] 能画出同步 HTTP 与异步 Worker 的明确分界。
- [ ] 能解释 Redis、Asynq Client、Queue、Server、Handler 各自职责。
- [ ] 能说明任务入队成功不等于业务执行成功。
- [ ] 能指出状态由谁修改，以及 Worker 重复执行可能产生什么副作用。
- [ ] 能用一分钟说明为什么长 Workflow 不应占用原 HTTP 请求。

---

## 五、8 月 2 日：Go Gateway → Python LangGraph

学习链路：

```text
Worker.handleAgentGraph
→ Gateway.Execute
→ AgentRunRequest
→ POST /internal/v1/agent-runs
→ Python graph registry
→ graph_key
→ initial_state
→ LangGraph invoke
→ AgentRunResponse
→ Go 持久化 Run Log 和 Node Output
```

学习问题：

- [ ] Go 在调用 Python 前校验什么？
- [ ] Agent Run ID、Workflow ID、Node ID 和 Trace ID 如何关联？
- [ ] Python 为什么只需要 `graph_key`，不需要整个 Workflow Template？
- [ ] `initial_state` 的字段来自哪里？
- [ ] Python 成功或失败的返回契约是什么？
- [ ] Go 如何更新 `agent_run_logs` 和 Node 状态？

必须交付：

- [ ] 一张 Go/Python 跨服务调用图。
- [ ] `AgentRunRequest` 与 Python `initial_state` 映射表。
- [ ] 一次真实或 Fixture 请求/响应示例。

### 工程验收

- [ ] 完成一次真实或隔离 Fixture 的 Go → Python 正常调用。
- [ ] 请求、响应与 `agent_run_logs`、Node Output 能相互对应。
- [ ] 至少验证一个非法 `graph_key`、非法响应或超时边界。
- [ ] Go/Python 两侧使用同一组 Run、Workflow、Node 和 Trace 标识。
- [ ] 跨服务输入输出契约有可复现证据。

### 个人能力验收

- [ ] 能脱稿解释 Worker → Gateway → HTTP → Graph Registry → LangGraph → 持久化。
- [ ] 能说明 Workflow Template 和 Python Graph 的职责为什么分离。
- [ ] 能解释 `AgentRunRequest` 如何转换为 `initial_state`。
- [ ] 能根据错误现象判断失败更可能位于 Gateway、网络、Registry、Graph 还是响应校验。
- [ ] 能说明跨服务契约为什么必须版本化并进行回归测试。

---

## 六、8 月 3 日：W1-D3 Agent Service 故障与恢复

### 故障假设

实验前先填写：

```text
预期 Workflow 状态：
预期 agent_graph 节点状态：
预期 retry_count：
预期 Agent Run Log：
预期 Audit：
预期恢复方式：
```

### 实验步骤

- [ ] 准备一个新的 Workflow，不复用正常演示实例。
- [ ] 在 Agent 节点执行前停止 `agent-service`，或使用受控错误地址。
- [ ] 启动 Workflow。
- [ ] 保存容器状态、Go 日志、Worker 日志和 Request/Workflow Trace。
- [ ] 查询 Workflow、Nodes、Agent Run Logs 和 Audit Logs。
- [ ] 恢复 `agent-service`。
- [ ] 判断 Asynq 是否自动重试。
- [ ] 若进入人工可重试状态，调用 `/workflow-instances/{id}/retry`。
- [ ] 验证不产生重复审批任务。
- [ ] 验证不重复归档。

必须观察：

```text
workflow.status
agent_graph.status
node.error_json
node.retry_count
node.max_retries
agent_run.status
audit.action
request_trace_id
workflow_trace_id
```

必须交付：

- [ ] 故障前后 JSON。
- [ ] 正常 Trace 与失败 Trace。
- [ ] 实际结果与实验假设差异。
- [ ] 不超过 300 字的故障分析。

### 工程验收

- [ ] 故障实验使用新实例，输入和故障注入方式可复现。
- [ ] Agent Service 不可用时，Workflow、Node 和 Agent Run 最终状态明确。
- [ ] 自动重试与人工 retry 的职责已经通过实验区分。
- [ ] 恢复后不产生重复审批、重复归档或重复副作用。
- [ ] 正常 Trace、失败 Trace、恢复证据和实际差异全部保存。

### 个人能力验收

- [ ] 实验前能写出状态预测，实验后能解释预测为何正确或错误。
- [ ] 能区分 HTTP Gateway 重试、Asynq 重试和业务节点 retry。
- [ ] 能从日志和状态判断失败发生在入队前、Worker、Gateway 还是 Python。
- [ ] 能说明为什么“服务恢复”不必然意味着失败 Workflow 自动恢复。
- [ ] 能提出当前恢复机制的一个风险和一个可行改进。

---

## 七、8 月 4 日：审批通过与归档推进

学习链路：

```text
POST /approval-tasks/{id}/approve
→ 审批权限
→ 审批事务
→ human_review succeeded
→ Workflow 继续推进
→ archive Node 入队
→ archived
```

学习问题：

- [ ] Approval ID 如何关联 Workflow 和 Node？
- [ ] 哪些更新必须位于同一事务？
- [ ] 为什么事务外状态检查防不住并发审批？
- [ ] 审批成功但下一节点投递失败会怎样？
- [ ] 如何防止重复审批和重复归档？

必须交付：

- [ ] 审批事务边界图。
- [ ] 状态变化与 Audit Action 对照。
- [ ] 一分钟回答：“审批成功但 Redis 投递失败怎么办？”

### 工程验收

- [ ] 一次审批可以从 `waiting_review` 正常推进到归档。
- [ ] Approval、Human Review Node、Workflow 和 Audit 状态一致。
- [ ] 至少验证审批后下一节点投递失败的预期或现有处理方式。
- [ ] 审批和 Workflow 推进的事务边界有代码或实验依据。
- [ ] 没有因重复处理产生重复归档。

### 个人能力验收

- [ ] 能画出 Approval ID、Node ID、Workflow ID 和 Workflow Trace 的关联。
- [ ] 能说明哪些状态必须原子更新，哪些跨越 Redis 无法放入同一数据库事务。
- [ ] 能解释事务外“先检查再更新”的并发风险。
- [ ] 能比较直接投递、Outbox、补偿任务等解决方案。
- [ ] 能用一分钟回答审批成功但投递失败如何收敛。

---

## 八、8 月 5 日：W1-D4 幂等测试

目标：

```text
相同 tenant + user + Idempotency-Key
→ 返回同一个 Workflow Instance
→ 不重复创建 Node Instances
```

至少验证：

- [ ] 相同 key 重复创建返回相同 ID。
- [ ] 第二次不重复创建 Nodes。
- [ ] 不提供 key 时可以创建新实例。
- [ ] 唯一约束竞争后能重新查询已有实例。
- [ ] 超长 key 返回明确校验错误。

本人责任：

- [ ] 先写测试场景和断言。
- [ ] 解释“幂等不等于禁止重复请求”。
- [ ] 解释为什么“先查询后插入”仍有并发竞态。

### 工程验收

- [ ] 五个幂等场景均有请求、断言和结果。
- [ ] 相同 key 不重复创建 Instance 和 Nodes。
- [ ] 数据库唯一约束参与解决并发竞态。
- [ ] 超长或非法 key 返回稳定、明确的错误。
- [ ] 测试可重复运行且不依赖手工清理才能判断结果。

### 个人能力验收

- [ ] 能解释幂等、去重、唯一约束和事务的区别。
- [ ] 能画出两个并发请求同时“先查后插入”的竞态。
- [ ] 能说明幂等键作用域为什么包含租户和用户。
- [ ] 能独立写出测试场景与断言，再让 Coding Agent 协助实现。
- [ ] 能把相同原则迁移到 Tool Call 或异步任务副作用。

---

## 九、8 月 6 日：W1-D5 重复审批与事务测试

目标：

```text
pending → approved 只发生一次
```

至少验证：

- [ ] 第一次审批成功。
- [ ] 第二次审批返回 `APPROVAL_NOT_PENDING`。
- [ ] 第二次不推进 Workflow。
- [ ] 第二次不新增成功审计。
- [ ] 事务失败时不继续执行归档。

本人责任：

- [ ] 画 `BEGIN → SELECT FOR UPDATE → UPDATE → COMMIT/ROLLBACK`。
- [ ] 解释为什么业务冲突不应返回笼统的 HTTP 500。

### 工程验收

- [ ] 第一次和第二次审批结果符合目标，状态只推进一次。
- [ ] 并发或重复审批不会重复推进 Workflow。
- [ ] 事务失败时归档任务不会继续执行。
- [ ] 失败不会留下“审批已成功但节点未更新”的部分状态。
- [ ] API 返回可识别的业务冲突错误。

### 个人能力验收

- [ ] 能解释 `SELECT FOR UPDATE` 锁住什么、持续到什么时候。
- [ ] 能说明重复审批为什么是业务冲突而不是系统内部错误。
- [ ] 能画出事务成功和回滚两条状态路径。
- [ ] 能指出数据库事务不能覆盖 Redis 投递的原因。
- [ ] 能说明这套一致性原则如何迁移到 HITL Resume。

---

## 十、8 月 7—9 日：Agent Runtime 第一轮

### Graph State 与 Structured Output

- [ ] 画 Finance Graph State 字段生产者和消费者。
- [ ] 区分 Workflow State 与 Graph State。
- [ ] 检查 Structured Output、Schema Validation、非法 JSON 和 fallback。
- [ ] 为至少一个失败路径运行或补充测试。

### Checkpoint 与 HITL

- [ ] 区分 State、Checkpoint、长期 Memory 和 Audit。
- [ ] 在最小实验中验证 `thread_id`、checkpoint、interrupt 和 resume。
- [ ] 验证恢复前副作用必须幂等。
- [ ] 对比 Go 审批推进与 LangGraph HITL 的职责边界。

### Harness 认知

- [ ] 能根据总台账 Harness 能力树指出本项目已有能力。
- [ ] 标记缺失的 Memory、Skill、Hook、Budget 和 Eval 证据。
- [ ] 确定 8 月 10 日后 RAG/Eval 与 Harness 实验的最小范围。

必须交付：

- [ ] Graph State 读写图。
- [ ] Workflow/Graph/Checkpoint/Memory 边界表。
- [ ] 一个 checkpoint + interrupt/resume 实验。
- [ ] 一组 Structured Output 或错误路径测试结果。

### 工程验收

- [ ] Runtime 实验与 Finance 主链隔离，使用最小可复现 Fixture。
- [ ] Graph State 字段生产者、消费者和最终输出有明确图示。
- [ ] Checkpointer 能保存状态，`interrupt` 后可通过同一 `thread_id` 恢复。
- [ ] 至少验证一次进程中断、重复 resume 或恢复前副作用边界。
- [ ] Structured Output 非法结果能够被校验、拒绝或进入明确 fallback。

### 个人能力验收

- [ ] 能区分 Workflow State、Graph State、Checkpoint、Memory 和 Audit。
- [ ] 能解释 Checkpoint 为什么是 Durable Agent 和 HITL 的基础。
- [ ] 能说明 Go 外层审批与 LangGraph 内部 interrupt 的职责差异。
- [ ] 能指出当前项目在 Loop、Memory、Skill、Hook、Budget、Trace、Eval 上的真实成熟度。
- [ ] 能根据实验提出下一阶段 Runtime 最小迭代，不直接设计大而全框架。

---

## 十一、本卡完成定义

只有同时满足以下条件才算完成：

- [ ] 所有日任务均按 [[工程交付与个人能力闭环标准]] 完成双重验收。
- [ ] 正常 Workflow 证据仍可复现。
- [ ] Agent Service 故障和恢复证据完整。
- [ ] 能脱稿解释同步 POST、异步 `/start` 和 Go/Python 边界。
- [ ] 至少一组本人设计的幂等测试通过。
- [ ] 至少一组重复审批/事务测试通过。
- [ ] Graph State 与持久化边界有图和文字说明。
- [ ] 至少一个 Runtime 最小实验可复现。
- [ ] 所有结果已回填 [[03_九月前执行计划与证据台账#9. 功能与学习证据台账]]。

---

## 十二、每日执行与证据

| 日期 | 当日主任务 | 实际完成 | 证据位置 | 仍讲不清的问题 | 下一步 | 状态 |
|---|---|---|---|---|---|---|
| 2026-07-31 | 同步 POST 创建链 |  |  |  |  | 未开始 |
| 2026-08-01 | `/start` 与 Asynq |  |  |  |  | 未开始 |
| 2026-08-02 | Go Gateway → Python |  |  |  |  | 未开始 |
| 2026-08-03 | W1-D3 故障恢复 |  |  |  |  | 未开始 |
| 2026-08-04 | 审批与归档 |  |  |  |  | 未开始 |
| 2026-08-05 | W1-D4 幂等 |  |  |  |  | 未开始 |
| 2026-08-06 | W1-D5 重复审批 |  |  |  |  | 未开始 |
| 2026-08-07—09 | Runtime 第一轮 |  |  |  |  | 未开始 |

状态只使用：

```text
未开始 / 进行中 / 已实现待验证 / 已验证 / 阻塞 / 暂缓 / 删除
```

---

## 十三、范围约束

本卡期间不做：

- 新采购、HR、法律等完整业务场景。
- 前端视觉重构。
- 多框架迁移。
- 完整长期 Memory 服务。
- 无 Eval 目标的多 Agent 扩张。
- 与当前主链无关的治理页面。

允许做：

- 阅读和手画当前调用链。
- 最小故障实验。
- 幂等和事务测试。
- Runtime Fixture、Checkpoint/HITL 小实验。
- 直接服务于 Trace、Eval 和面试表达的文档。

任何任务工程验收完成但个人能力验收未完成时，不新增无关项目功能；优先补画图、故障实验、复述和设计取舍。个人能力达到“看懂”但工程尚未验证时，也不能只继续阅读，必须进入最小实验。

完成本卡后，从本文件提取结果回填总台账，本文件继续保留为实际执行历史。

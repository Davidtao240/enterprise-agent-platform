# Release Checklist（上线前检查清单）

> 文档状态：Active
> 更新日期：2026-08-21
> 适用范围：企业智能体平台（Go backend + Python Agent Service + Frontend）每次发布上线

## 使用方式

每次发版前，按本清单逐项勾选。**全部通过**才能合并到 `main` 并打 tag、部署。
任一项未通过：必须修复或回滚，禁止"带病上线"。
清单执行人 = 当前 Sprint 的责任人；复核人 = 至少一名其他开发者。

---

## 1. 代码与质量检查

### 1.1 必须通过的自动化门禁
- [ ] CI 全绿（`.github/workflows/ci.yml` 的 `quality` job + `docker-build` job）
- [ ] Go 测试：`go test -race -timeout 10m ./...` 在本地或 CI 通过
- [ ] Go 集成测试：`*_integration_test.go` 在 `TEST_DATABASE_URL` 已设时自动 unskip 真跑
- [ ] Python 测试：`python -m unittest discover -s tests` 全部通过（PG 集成测试在 DSN 设定时跑）
- [ ] 前端测试：`npm run test:coverage` 通过，核心页面（Login / Agent Gallery / Conversation / Run Detail / Registry / Approval / Audit / Experiments）覆盖率 ≥ 50%
- [ ] 前端构建：`npm run build` 通过（`tsc && vite build` 无 TS 类型错误）
- [ ] 安全检查：`bash scripts/security-check.sh` 通过
- [ ] 环境变量检查：`bash scripts/check-env.sh production` 通过（拒绝弱默认值）

### 1.2 代码规范与架构约束
- [ ] 新增 Go `internal/` 代码未引入业务域 if/else（平台业务中立原则）
- [ ] Workflow Engine 未硬编码 finance / procurement / HR 等业务逻辑
- [ ] Go 数据库查询全部使用参数化查询（`$1, $2...`），无 `fmt.Sprintf` 字符串拼接
- [ ] LLM Gateway 要么连接真实 LLM API，要么明确标记 mock-only，无空 stub
- [ ] 用户输入在 LLM prompt 中已用 `<user_input>` 分隔，并使用 structured output 防注入
- [ ] 所有 middleware 使用 async/await，无 callback 风格
- [ ] 长时 goroutine 使用 `context.Background()`，未绑定 HTTP request context
- [ ] 跨栈契约：Go API 响应 shape 与前端 `api.ts` 期望一致；Python Agent 输出与 `agent_run_logs` schema 一致
- [ ] Knowledge Search 响应使用 `collection` 字段名（非 `collection_name`）

### 1.3 文档对齐
- [ ] 本次变更涉及的 API / 数据模型 / 状态机已更新到 `docs/02_ARCHITECTURE/` 或 `docs/03_PLATFORM_SPEC/`
- [ ] 新增业务域有对应的 `04_V1_FINANCE/` 风格领域文档（流程图、契约 Fixture、独立预期结果）
- [ ] ROADMAP.md 中相关里程碑的 Gate 已勾选或新增条目已记录
- [ ] 破坏性变更已写明并通知前端 / Agent Service 同步改

---

## 2. 数据库变更检查

### 2.1 Migration 安全性
- [ ] 新增 `migrations/NNN_*.up.sql` 文件名编号连续，无重复编号
- [ ] 配套 `NNN_*.down.sql` 已编写且能在干净环境回滚成功
- [ ] Migration 是幂等的（`CREATE TABLE IF NOT EXISTS` / `ALTER TABLE ... IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS`）
- [ ] Migration 不包含业务数据硬编码（种子数据走 `004_seed_*.sql` 或独立 fixture）
- [ ] 大表变更（`ALTER TABLE` 加列、加索引）已评估锁表风险，必要时使用 `CREATE INDEX CONCURRENTLY`
- [ ] pgvector 相关 migration 在 `pgvector/pgvector:pg16` 镜像下验证通过

### 2.2 数据兼容性
- [ ] 新增字段有默认值或 nullable，不会让旧版本服务读取时崩溃
- [ ] 删除字段已确认无引用（grep 全仓 + 跨栈检查）
- [ ] Schema 变更不破坏 Finance V1 回归 Fixture（`docs/04_V1_FINANCE/`）

---

## 3. 配置与密钥检查

### 3.1 必填环境变量（缺一不可，否则 panic）
- [ ] `JWT_SECRET` 非空且长度 ≥ 32 字符
- [ ] `INTERNAL_SERVICE_TOKEN` 非空，且非 `replace-with-a-random-service-token` 占位值
- [ ] `TOOL_SECRET_ENCRYPTION_KEY` 非空且为合法的 32 字节加密密钥
- [ ] `LLM_API_KEY` 非空（指向真实 LLM 服务）
- [ ] `DB_PASSWORD` / `MINIO_SECRET_KEY` / `REDIS_*` 已从开发默认值改为生产强密码

### 3.2 配置文件
- [ ] `.env` 已通过 `scripts/check-env.sh production` 校验
- [ ] `.env` 文件未被 git 跟踪（`.gitignore` 已包含）
- [ ] `docker-compose.yml` 中无硬编码 secret，全部通过 `${VAR}` 注入
- [ ] `GO_SERVER_MODE=release`（生产禁止 debug）
- [ ] `STRICT_DOMAIN_POLICY=true`（生产必须开启严格模式）
- [ ] `WORKER_RUNTIME_V2=true`（M1+ Durable Run 已启用）

### 3.3 密钥轮换
- [ ] `JWT_SECRET` 在最近 90 天内轮换过（或本次发版已轮换）
- [ ] `INTERNAL_SERVICE_TOKEN` 在最近 90 天内轮换过
- [ ] 数据库 / MinIO / Redis 密码符合公司密钥管理策略
- [ ] 离职 / 转岗员工的服务 token 已失效

---

## 4. Agent Runtime 与 LLM 检查

### 4.1 Agent 执行边界
- [ ] LLM Gateway 端点、V1 Agent 执行端点、公共文件端点已加 `InternalServiceToken` 鉴权
- [ ] 所有 Tool Call 经 Tool Execution Gateway，无绕过
- [ ] 6 个 finance agent（采集 → 提取 → 分析 → 合规 → 复核 → 报告）端到端 smoke 通过
- [ ] `finance_operating_report` / `document_summary` / `finance_chat` / `meeting_minutes` 4 个 graph 在 LangGraph 层面 smoke 通过
- [ ] Durable Run 恢复测试：执行中杀掉 Python 进程，能从正确 Checkpoint 恢复
- [ ] Durable Run 接管测试：执行中杀掉 Go Worker，不会重复完成

### 4.2 LLM 与成本
- [ ] `LLM_PROVIDER` / `LLM_MODEL` 已配置为生产可用模型（非 dev 默认值）
- [ ] Token / Cost 计量链路（agent_run_logs → audit）已验证
- [ ] 预算超限熔断策略已配置（如启用 M9-B LLM Gateway）
- [ ] Prompt injection 防护测试通过（`<user_input>` 分隔 + structured output）

### 4.3 Domain Policy
- [ ] 跨 Tenant Tool Call 在测试中确定性拒绝
- [ ] 重复调用不产生重复外部副作用（幂等性测试通过）
- [ ] 审批 Payload 与实际执行内容一致

---

## 5. 部署前检查

### 5.1 镜像与编排
- [ ] `docker compose build go-backend agent-service frontend` 在 CI 通过
- [ ] `docker compose config --quiet` 无报错
- [ ] 镜像版本已打 tag（`<service>:<semver>` 或 `<service>:<git-sha>`），禁止 `:latest` 上生产
- [ ] `docker-compose.yml` 中 image tag 已固定为本次发布版本
- [ ] 所有 Dockerfile 中无 `latest` 基础镜像 tag

### 5.2 依赖与服务
- [ ] PostgreSQL 16 实例可达，pgvector extension 已安装
- [ ] Redis 7 实例可达，无残留长连接
- [ ] MinIO bucket 已创建，权限正确（非公开）
- [ ] LLM API 端点可达，API Key 有效性已验证
- [ ] Asynq 队列无积压未完成任务（`asynq inspect`）

### 5.3 数据迁移演练
- [ ] 在 staging 环境完整跑过一次 `migrations/*.up.sql`
- [ ] 在 staging 环境完整跑过一次 `migrations/*.down.sql`（验证回滚链可用）
- [ ] staging 环境 Finance V1 回归 Fixture 全通过

---

## 6. 部署后验证

### 6.1 健康检查
- [ ] `GET http://<go-backend>/health` 返回 200
- [ ] `GET http://<agent-service>:8000/health` 返回 200
- [ ] `GET http://<frontend>/` 返回 200
- [ ] Docker Compose 所有 service healthcheck 状态为 healthy

### 6.2 端到端业务验证
- [ ] 用 `admin / finance_user / finance_manager / ops_viewer` 4 个账号分别登录成功
- [ ] `finance_user` 能发起一个财务协作报告 Workflow 并跑完 6 个 Agent
- [ ] `finance_manager` 能在审批节点通过 / 驳回
- [ ] 审批通过后结果写入 DB（`workflow_instances.status = completed`）
- [ ] 审计日志可查（`audit_logs` 表有本次操作记录）
- [ ] 文件上传 / 下载链路正常（MinIO）

### 6.3 可观测性
- [ ] Trace 链路打通：Workflow → Run → Model Turn → Tool Call → Checkpoint → Interrupt
- [ ] Token / Cost 在 `platform_observability_summary` 接口可见
- [ ] 日志聚合可查（如有 ELK / Loki）
- [ ] 告警通道已配置（如有 Prometheus + Alertmanager）

---

## 7. 回滚预案

### 7.1 回滚决策依据
- [ ] 已定义"什么情况下回滚"（如：核心 Workflow 跑不通 / 审批失效 / 数据写入错误）
- [ ] 已定义"谁有权决定回滚"（on-call 责任人 + 通知机制）
- [ ] 已定义"回滚时限"（如部署后 30 分钟内发现 P0 问题立即回滚）

### 7.2 回滚操作步骤
- [ ] 切回上一版本镜像 tag（`docker-compose.yml` 改 tag → `docker compose up -d`）
- [ ] 执行 `migrations/NNN_*.down.sql`（按倒序）
- [ ] 验证 `down.sql` 在 staging 已演练过（第 5.3 节）
- [ ] 回滚后跑一次 Finance V1 回归 Fixture 验证可用性
- [ ] 回滚事件写入审计日志并通知团队

### 7.3 数据兼容性回滚
- [ ] 如果本次发版有不可逆数据变更（如删字段），已准备好数据迁移脚本
- [ ] 如果本次发版有 schema 变更且 down.sql 不可靠，已准备 forward-fix 方案（不回滚 schema，只回滚代码）

---

## 8. 通讯与流程

- [ ] PR 已获至少 1 名 reviewer approve
- [ ] PR 描述包含：变更范围、影响域、回滚方法、验证结果
- [ ] 已更新 `docs/01_PROJECT/ROADMAP.md` 中相关里程碑状态
- [ ] 已在团队群通知"本次发版范围 + on-call 责任人"
- [ ] 发版后 24 小时内有"值守观察窗口"，责任人可响应异常

---

## 9. 发布记录模板

每次发版完成，在 `docs/01_PROJECT/RELEASE_LOG.md`（如不存在则创建）追加一条记录：

```markdown
## vX.Y.Z — YYYY-MM-DD

- 变更范围：<本次发版的模块与功能>
- 影响域：<Go backend / Agent Service / Frontend / DB schema / 配置>
- 验证结果：<CI 编号 / staging 回归结果 / 端到端验证截图链接>
- 回滚方法：<镜像 tag + down.sql 编号>
- on-call：<责任人>
- 备注：<已知问题 / 后续 follow-up>
```

---

## 引用

- [ROADMAP.md](./ROADMAP.md) — 里程碑与 Gate
- [MVP_SCOPE.md](./MVP_SCOPE.md) — V1.0 核心功能范围
- [../02_ARCHITECTURE/](../02_ARCHITECTURE/) — 28 份架构设计文档
- [../../AGENTS.md](../../AGENTS.md) — 工程约束与代码审查协议
- [../../scripts/check-env.sh](../../scripts/check-env.sh) — 环境变量校验
- [../../scripts/security-check.sh](../../scripts/security-check.sh) — 安全扫描
- [../../.github/workflows/ci.yml](../../.github/workflows/ci.yml) — CI 流水线

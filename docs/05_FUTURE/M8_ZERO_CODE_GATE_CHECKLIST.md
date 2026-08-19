# M8 零代码接入 · Gate 检查清单（可打印 · 可勾选）

> **使用说明**：按步骤从前往后勾选，每 Step 内所有 PASS 都打勾后，方可进入下一步。
> 最终 Gate 判定位于文末；**任一项 FAIL 触发立即终止**，回滚修复后重验。
>
> 主文档：[`M8_ZERO_CODE_ONBOARDING_VERIFICATION.md`](./M8_ZERO_CODE_ONBOARDING_VERIFICATION.md)

---

```
╔══════════════════════════════════════════════════════════════════════════╗
║   M8 零代码接入验证 · Gate Checklist   v1.0  ·  2026-08-19               ║
╠══════════════════════════════════════════════════════════════════════════╣
║                                                                          ║
║  验证分支：agentic-system/m8-zero-code-verification                      ║
║  Commit SHA：____________________   执行日期：____-__-__                 ║
║  执行人：__________   总控人：__________                                 ║
║                                                                          ║
╚══════════════════════════════════════════════════════════════════════════╝
```

## 0. 前置条件（必须全部满足才能开始）

```
╔══ PRE ════════════════════════════════════════════════════════════════════╗
║                                                                            ║
║   □ 0-1  数据库迁移 ≥ 036：                                                ║
║          agent_package_versions 表存在：  □ YES □ NO                      ║
║          agent_package_installations 表存在：□ YES □ NO                   ║
║          connector_sidecars 表存在：□ YES □ NO                            ║
║          psql 查询输出粘贴：                                                ║
║          _____________________________________________________             ║
║                                                                            ║
║   □ 0-2  服务健康检查全部 200：                                            ║
║          Go backend    □ PASS  □ FAIL  ·  Python agent  □ PASS  □ FAIL   ║
║          Frontend Vite □ PASS  □ FAIL  ·  Redis        □ PASS  □ FAIL   ║
║          PostgreSQL    □ PASS  □ FAIL  ·  MinIO        □ PASS  □ FAIL   ║
║                                                                            ║
║   □ 0-3  基线回归全通过：                                                  ║
║          go test ./...  □ PASS (FAIL count = ___)                         ║
║          python -m unittest discover tests/  □ PASS  □ FAIL              ║
║          Finance Contract 回归：  □ PASS (N = ___)  □ FAIL               ║
║          npm run build： □ PASS  □ FAIL                                  ║
║                                                                            ║
║   □ 0-4  仓库状态 + 测试角色：                                             ║
║          git status → clean： □ YES □ NO                                 ║
║          角色 availability：                                              ║
║             admin              □ YES  □ NO                               ║
║             procurement_manager □ YES  □ NO （若需创建：RBAC API/种子，   ║
║                                               不改 Go 代码）              ║
║             procurement_user   □ YES  □ NO                               ║
║             finance_user       □ YES  □ NO                               ║
║                                                                            ║
║   □ 0-5  JWT Token 采集：                                                 ║
║          ADMIN_JWT=           □已得    □未得                              ║
║          PROCUREMENT_MGR_JWT= □已得    □未得                              ║
║          FINANCE_USER_JWT=    □已得    □未得                              ║
║          INTERNAL_SERVICE_TOKEN= □已得 □未得                              ║
║                                                                            ║
║   □ 0-6  前置结论：  □ 全部就绪 → 开始 Step 1                             ║
║                     □ 存在阻塞：_________________________________         ║
║                                                                            ║
╚════════════════════════════════════════════════════════════════════════════╝
```

---

## Step 1：Package Manifest 注册

```
╔══ STEP 1 ══════════════════════════════════════════════════════════════════╗
║                                                                            ║
║  1-1  创建 Manifest 文件（纯新增，不改现有）                                ║
║       路径：agent-service/app/manifests/                                   ║
║              procurement_quote_review_v0.1.0.json                          ║
║       □ 已创建  □ 未创建                                                   ║
║       package_code=procurement_quote_review  □正确 □错误                  ║
║       version=0.1.0                         □正确 □错误                  ║
║       graph_key=procurement_quote_review_graph □正确 □错误               ║
║                                                                            ║
║  1-2  注册 Package 基础记录                                                ║
║       POST /api/v1/gallery/packages                                        ║
║       HTTP 响应：200 OK  □ YES  □ NO (code=____, err=________________)    ║
║       返回 package_code：___________________（非空）                       ║
║                                                                            ║
║  1-3  创建 Version 0.1.0                                                   ║
║       POST /.../packages/:code/versions                                    ║
║       HTTP 响应：200 OK  □ YES  □ NO (code=____)                          ║
║       manifest 字段是否正确保存： □ YES □ NO                               ║
║                                                                            ║
║  1-4  Publish Version                                                      ║
║       POST /.../versions/0.1.0/publish                                     ║
║       HTTP 200  □YES □NO；status=published □YES □NO；is_current=true □Y □N║
║                                                                            ║
║  1-5  DB 验证（agent_package_versions）：                                  ║
║       count(*) ≥ 1  □YES □NO；version=0.1.0 存在： □YES □NO               ║
║                                                                            ║
║  1-6  零 Go 代码验证：                                                     ║
║       cd go-platform && git diff --name-only internal/                    ║
║       输出：___________________________（期望空）                          ║
║       □ 零改动（PASS）  □ 有改动（FAIL：文件列表_________________）       ║
║                                                                            ║
║  Step 1 Gate：  □ PASS    □ FAIL（原因：_______________________）        ║
║                                                                            ║
╚════════════════════════════════════════════════════════════════════════════╝
```

---

## Step 2：Business App + Domain Policy 注册

```
╔══ STEP 2 ══════════════════════════════════════════════════════════════════╗
║                                                                            ║
║  2-1  036_procurement_seed.up.sql 创建（纯新增，允许）                     ║
║       路径：go-platform/migrations/036_procurement_seed.up.sql             ║
║       □ 已创建  □ 未创建                                                   ║
║       INSERT business_apps(code=procurement)： □正确 □错误                ║
║       INSERT domain_policies(tool_denylist=finance.* + hr.* + ...)         ║
║         □正确 □错误                                                       ║
║                                                                            ║
║  2-2  migrate-only 执行：                                                  ║
║       go run cmd/server/main.go --migrate-only                             ║
║       日志包含 036 applied： □ YES  □ NO                                   ║
║                                                                            ║
║  2-3  DB 验证：                                                            ║
║       SELECT * FROM business_apps WHERE code='procurement';                ║
║       记录存在 + status=active： □ YES  □ NO                               ║
║       domain_policies.policy_json 中包含 finance.* 在 denylist：           ║
║         □ YES  □ NO                                                        ║
║                                                                            ║
║  2-4  Policy 生效渗透冒烟：                                                ║
║       用 ADMIN_JWT 伪装 procurement 调 finance.report_generate：           ║
║       HTTP=403 或 含 DOMAIN_POLICY_VIOLATION：                             ║
║         □ PASS（隔离生效）   □ FAIL（____）                                ║
║       实际响应粘贴：                                                        ║
║       _____________________________________________________                ║
║                                                                            ║
║  2-5  零 Go 代码验证：                                                     ║
║       git diff --name-only go-platform/internal/ → 空：                    ║
║         □ PASS  □ FAIL（文件：______________________________）           ║
║                                                                            ║
║  Step 2 Gate：  □ PASS    □ FAIL（原因：_______________________）        ║
║                                                                            ║
╚════════════════════════════════════════════════════════════════════════════╝
```

---

## Step 3：Connector Sidecar 注册

```
╔══ STEP 3 ══════════════════════════════════════════════════════════════════╗
║                                                                            ║
║  3-1  Sidecar 独立目录创建：                                                ║
║       路径：procurement-sidecar/server.py（不放入 go-platform 或          ║
║             agent-service 中；属于独立外部进程）                           ║
║       □ 已创建  □ 未创建                                                   ║
║       端口：______（默认 8700，可改）                                      ║
║                                                                            ║
║  3-2  Sidecar 端点完备性：                                                 ║
║       GET  /health           □实现 □未实现（返回 healthy）                ║
║       POST /execute          □实现 □未实现（至少 supplier_query            ║
║                                       + budget_check）                    ║
║       POST /verify           □实现 □未实现                                ║
║       POST /compensate       □实现 □未实现（可选）                        ║
║                                                                            ║
║  3-3  Sidecar 本地健康：                                                   ║
║       curl -f localhost:<port>/health → 200： □ PASS  □ FAIL             ║
║       execute supplier_query → status=succeeded： □ PASS  □ FAIL          ║
║                                                                            ║
║  3-4  平台注册（零代码 API）：                                              ║
║       POST /api/v1/tools/sidecars/register  → 200 OK：                     ║
║         □ PASS  □ FAIL（code=____；err=________________________）        ║
║       返回 connector_code=procurement_erp： □正确 □错误                   ║
║                                                                            ║
║  3-5  平台代查健康：                                                       ║
║       GET /.../sidecars/procurement_erp/health → healthy：                 ║
║         □ PASS  □ FAIL                                                    ║
║                                                                            ║
║  3-6  真实 Tool Gateway → Sidecar 调用：                                   ║
║       用 procurement_erp 的 supplier_query 调平台                          ║
║         /api/v1/tools/calls → 200 + 返回供应商数据：                       ║
║         □ PASS  □ FAIL（err=____________________________）               ║
║                                                                            ║
║  3-7  零 Go 代码验证：                                                     ║
║       git diff --name-only go-platform/internal/ → 空：                    ║
║         □ PASS  □ FAIL（文件：______________________________）           ║
║                                                                            ║
║  Step 3 Gate：  □ PASS    □ FAIL（原因：_______________________）        ║
║                                                                            ║
╚════════════════════════════════════════════════════════════════════════════╝
```

---

## Step 4：采购 Python Agent Graph 实现

```
╔══ STEP 4 ══════════════════════════════════════════════════════════════════╗
║                                                                            ║
║  4-1  Graph 文件新增：                                                     ║
║       路径：agent-service/app/graphs/                                      ║
║              procurement_quote_review.py                                   ║
║       □ 已创建  □ 未创建                                                   ║
║       命名：build_graph()      □ 对齐 □ 未对齐                             ║
║       契约：contract_version=1.0.0 + request_id 必填校验：                  ║
║         □ 对齐 [06_智能体输入输出契约.md] □ 未对齐                          ║
║                                                                            ║
║  4-2  Graph Registry 注册（仅允许 1 行追加映射）                           ║
║       改文件：agent-service/app/registry/graph_registry.py                 ║
║       仅追加键："procurement_quote_review_graph" → builder_module          ║
║         □ 1 行完成   □ 改了其它内容（Fail）                                ║
║       改了 app/runtime/* 核心文件？  □ NO □ YES（Fail-2）                  ║
║                                                                            ║
║  4-3  Agent 冒烟（curl start + query）：                                   ║
║       start → HTTP 202 Accepted： □ PASS □ FAIL                           ║
║       5~10s 后 query status=succeeded：  □ PASS  □ FAIL                   ║
║       output 字段检查（对齐 [06_智能体输入输出契约.md] §响应）：            ║
║          policy_checks   : □有 □无    （至少 check_min_quotes 出现）      ║
║          scorecards[*].total_score : □有 □无 （数量 = 3）                 ║
║          scorecards[*].rank 1~3 无重复： □ YES □ NO                       ║
║          recommendation.leading_supplier_id： □有 □无                     ║
║                                                                            ║
║  4-4 零代码验证：                                                          ║
║      go-platform/internal/ git diff → 空：                                 ║
║         □ PASS  □ FAIL                                                    ║
║      agent-service/app/runtime/ git diff → 空：                            ║
║         □ PASS  □ FAIL（Fail-2：改了 Runtime 核心）                        ║
║                                                                            ║
║  Step 4 Gate：  □ PASS    □ FAIL（原因：_______________________）        ║
║                                                                            ║
╚════════════════════════════════════════════════════════════════════════════╝
```

---

## Step 5：安装 + UI 可见 + 对话端到端验证

```
╔══ STEP 5 ══════════════════════════════════════════════════════════════════╗
║                                                                            ║
║  5-1  安装（procurement_manager 身份）                                     ║
║       POST /gallery/packages/procurement_quote_review/install → 200       ║
║         □ PASS  □ FAIL                                                     ║
║       agent_package_installations.status = active： □YES □NO              ║
║                                                                            ║
║  5-2  UI：Gallery 可见                                                     ║
║       操作：登录 → Gallery → 筛选 category=采购                            ║
║       "采购询价分析 Agent" 卡片出现：                                      ║
║         □ 自动出现（无需改前端）  □ 需刷新 □ 不出现                       ║
║       卡片字段完整：图标 □、版本 0.1.0 □、样本 Prompt □                   ║
║                                                                            ║
║  5-3  对话端到端（人工操作）                                               ║
║       输入 prompt（≥ 100 tokens 含自然语言 + 3 份报价文件 ID）              ║
║         □ 已执行  □ 未执行                                                 ║
║       SSE 流式打字效果：   □ 有  □ 无                                     ║
║       澄清追问轮数：≤ 2 轮： □ YES（___轮） □ NO（>2 轮）                  ║
║       Tool 调用 procurement.supplier_query：                               ║
║         □ 成功（Sidecar data 返回） □ 失败（原因____________）            ║
║       Tool 调用 procurement.budget_check：                                 ║
║         □ 成功  □ 跳过  □ 失败（原因____________）                        ║
║                                                                            ║
║  5-4  最终输出契约对齐（前端渲染或 API 直查）                               ║
║       contract_version = 1.0.0：     □ Y □ N                              ║
║       policy_checks 长度 ≥ 4：       □ Y □ N （____ 个）                  ║
║       scorecards 长度 = 3 且 rank 1/2/3：  □ Y □ N                        ║
║       recommendation.status ∈ recommended / recommended_with_conditions   ║
║         □ Y □ N                                                            ║
║       review_packet.questions 长度 ≥ 2：  □ Y □ N                         ║
║                                                                            ║
║  5-5  六层 Trace 完整：                                                    ║
║       Run Detail → Trace：                                                 ║
║         Workflow层 □有  Run层 □有  Turn层 □有                              ║
║         Tool Call层 □有  Checkpoint层 □有  Interrupt层 □有                ║
║       □ 6/6 完整    □ 缺失层：_________________________                   ║
║                                                                            ║
║  5-6  审计 Log：                                                           ║
║       /api/v1/audit-logs?filter=agent_procurement_quote_review_graph      ║
║         install 事件  □有   conversation_run 事件  □有                    ║
║         tool_call 事件  □有   uninstall 事件  （5-7 后补）                ║
║       每条都含 user_id、tenant_id、timestamp：                             ║
║         □ YES  □ NO（丢失字段：________________________________）        ║
║                                                                            ║
║  5-7  卸载 & 消失验证：                                                    ║
║       POST /.../packages/:code/uninstall → 200 OK： □ PASS  □ FAIL        ║
║       Gallery 刷新后卡片消失： □ YES（立即）  □ NO（需重启/等待）          ║
║       历史 Run 仍可查看： □ YES  □ NO（卸载把历史删了也 Fail）             ║
║                                                                            ║
║  5-8  端到端计时：                                                         ║
║       从 Step 5-1 安装 到 5-4 输出结果：________ 分钟                      ║
║       ≤ 10 分钟 □ PASS   > 10 分钟 □ FAIL（原因：______）                 ║
║                                                                            ║
║  5-9  零 Go 代码验证：                                                     ║
║       git diff --name-only go-platform/internal/ → 空：                    ║
║         □ PASS  □ FAIL（文件：______________________________）           ║
║                                                                            ║
║  Step 5 Gate：  □ PASS    □ FAIL（原因：_______________________）        ║
║                                                                            ║
╚════════════════════════════════════════════════════════════════════════════╝
```

---

## 最终 Gate（M8 总判定）

```
╔════════════════════════════════════════════════════════════════════════════╗
║                      FINAL GATE · M8 零代码接入验收                         ║
╠════════════════════════════════════════════════════════════════════════════╣
║                                                                            ║
║  ━━━━━━ PASS 判定（必须全部满足）━━━━━━                                   ║
║                                                                            ║
║   □ Gate-1  零 Go 代码改动                                                ║
║            git diff go-platform/internal/ → 输出：                         ║
║            ________________________________________________________        ║
║            （期望空）   □ 0 文件改动（PASS） □ N 文件改动（FAIL）         ║
║                                                                            ║
║   □ Gate-2  端到端可用 ≤ 10 分钟                                          ║
║            Step 5-8 记录：____ 分钟   □ ≤10  □ >10                        ║
║                                                                            ║
║   □ Gate-3  跨域隔离有效                                                  ║
║            运行 scripts/m8_cross_domain_isolation_test.sh：                ║
║            PASS=___  FAIL=___                                             ║
║              □ FAIL=0（PASS）  □ FAIL>0（列：______________________）    ║
║            审计告警存在（渗透 attempt 被记录）：                            ║
║              □ YES □ NO                                                    ║
║                                                                            ║
║   □ Gate-4  卸载即消失                                                    ║
║            Step 5-7 验证结果：                                             ║
║              Gallery 无卡片：□ YES □ NO                                   ║
║              历史 Run 仍可查：□ YES □ NO                                  ║
║              无需重启服务：  □ YES □ NO                                   ║
║                                                                            ║
║   □ Gate-5  基线回归无损                                                  ║
║            go test ./... → FAIL count：___  □ 0 □ >0                     ║
║            python test_finance_contract_regression.py → FAIL：___         ║
║              □ 0 □ >0                                                     ║
║            npm run build → 0 error： □ YES □ NO                           ║
║                                                                            ║
║   □ Gate-6  双域连续通过（建议：采购 + HR/法务任一）                       ║
║            第二业务域：____（例 HR 员工入职、法务合同审查、IT 开户…）       ║
║            Step 1-5 复用，同 PASS：  □ YES  □ NO /  □ 本阶段未执行       ║
║                                                                            ║
║  ━━━━━━ FAIL 判定（任一即整体 FAIL）━━━━━━                               ║
║                                                                            ║
║   □ Fail-1  被迫改了 Go 平台代码： internal/ 下现有 .go 文件被修改        ║
║               □ 触发  文件列表：________________________________________  ║
║               □ 未触发                                                     ║
║   □ Fail-2  被迫改了 Python Runtime 核心（app/runtime/*.py）              ║
║               □ 触发  文件列表：________________________________________  ║
║               □ 未触发                                                     ║
║   □ Fail-3  被迫改前端 App.tsx/Gallery 组件/路由                           ║
║               □ 触发  文件列表：________________________________________  ║
║               □ 未触发                                                     ║
║   □ Fail-4  跨域泄漏：采购 Agent 能读 Finance 真实数据（非 baseline）      ║
║               □ 触发  响应：____________________________________________  ║
║               □ 未触发                                                     ║
║   □ Fail-5  Finance 回归被破坏                                             ║
║               □ 触发  失败用例列表：____________________________________  ║
║               □ 未触发                                                     ║
║   □ Fail-6  安装/卸载需重启 Go 或 Python 服务                              ║
║               □ 触发  需重启的：□ Go □ Python □ Both                     ║
║               □ 未触发                                                     ║
║                                                                            ║
║  ━━━━━━ 结论 ━━━━━━                                                        ║
║                                                                            ║
║     □ M8 Gate 整体通过（PASS 6/6 + FAIL 0/6）                              ║
║       → 下一步：固化 Playbook + HR/法务二选一演练 + 启动 M9 规划          ║
║                                                                            ║
║     □ M8 Gate 未通过（FAIL 计数 = ____ / 6）                               ║
║       → 阻塞原因清单（Fail-N）：                                           ║
║         1) _______________________________________________________________║
║         2) _______________________________________________________________║
║         3) _______________________________________________________________║
║       → 修复负责人：___________  预计修复日期：____________               ║
║       → 重验建议时间：____________                                          ║
║                                                                            ║
╚════════════════════════════════════════════════════════════════════════════╝
```

---

## 附录 A · 关键 git diff 命令速查

在每一步的"零 Go 代码验证"中，执行以下命令并粘贴输出：

```bash
# Step 校验 1：Go internal/ 目录零改动（核心）
cd /Users/jinli/Project/enterprise-agent-platform/go-platform
echo "--- Go internal/ diff (must be empty) ---"
git diff --name-only HEAD internal/
echo "--- End ---"

# Step 校验 2：Python Runtime 核心零改动
cd /Users/jinli/Project/enterprise-agent-platform/agent-service
echo "--- Python runtime diff (must be empty) ---"
git diff --name-only HEAD app/runtime/ app/registry/graph_registry.py | grep -v graph_registry.py || true
echo "--- End ---"
# 注：graph_registry.py 允许 1 行改动

# Step 校验 3：前端核心零改动（Route/Component 不该改）
cd /Users/jinli/Project/enterprise-agent-platform/frontend
echo "--- Frontend core diff (App.tsx + routes should be empty) ---"
git diff --name-only HEAD src/App.tsx src/router/ 2>/dev/null || echo "(no match)"
echo "--- End ---"
```

**验证标准**：除 `graph_registry.py`（允许 < 5 行）外，其它 diff 必须**绝对为空**。

---

## 附录 B · 填写人签名

```
执行人签名：_____________________   日期：___________
QA 复核签名：_____________________   日期：___________
Tech Lead 裁决：□ PASS  □ FAIL   签名：_____________________   日期：___________
产品总监确认：_____________________   日期：___________
```

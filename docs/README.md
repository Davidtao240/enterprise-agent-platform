# Enterprise Agent Platform 文档索引与权威层级

> 状态：Active
> 更新日期：2026-08-16

## 1. 为什么需要文档分层

本仓库同时包含 Finance V1 历史基线、当前平台规范、未来 Agentic Platform 路线和 Procurement 场景设计。它们服务于不同目的，不能把“未来场景设想”当作当前开发顺序，也不能用新架构覆盖已经用于回归的 V1 契约。

发生冲突时，先判断冲突属于“目标方向”“当前里程碑”“实现契约”还是“业务兼容行为”，再按下列层级处理。

## 2. 权威层级

| 层级 | 文档 | 决定什么 |
|---|---|---|
| L1 北极星 | [`05_FUTURE/ENTERPRISE_AGENTIC_PLATFORM_EVOLUTION.md`](05_FUTURE/ENTERPRISE_AGENTIC_PLATFORM_EVOLUTION.md) | 最终形态、架构原则、阶段依赖和禁止事项 |
| L2 当前 Roadmap | [`01_PROJECT/ROADMAP.md`](01_PROJECT/ROADMAP.md) | 当前执行哪个里程碑、完成定义和阶段顺序 |
| L3 实现规范 | `02_ARCHITECTURE/`、`03_PLATFORM_SPEC/` | 对象、状态、API、数据库、权限和跨服务契约 |
| L4 兼容基线 | `04_V1_FINANCE/` | Runtime 重构不能破坏的 Finance V1 外部行为 |
| L5 场景设计 | `06_PROCUREMENT/`、`05_FUTURE/FUTURE_SCENARIOS.md` | 未来业务约束和验收要求，不自动获得开发准入 |

优先级不是简单地“上层覆盖下层”：

- L1/L2 决定做什么和先做什么。
- 编码前必须把当前里程碑细化到 L3。
- L3 的变化不得无意破坏 L4。
- L5 只有通过 L1/L2 定义的平台 Gate 后才能进入实现。

## 3. 文档状态

| 状态 | 含义 |
|---|---|
| `Active North Star` | 当前目标路线，只有架构决策变化时才更新 |
| `Active Roadmap` | 当前执行入口，需要随里程碑验收更新 |
| `Active Specification` | 当前实现契约，代码变化必须同步 |
| `Compatibility Baseline` | 历史行为基线，原则上只补澄清和回归证据 |
| `Deferred Scenario` | 已设计但尚未取得实现或生产准入 |
| `Historical ADR` | 当时已经接受的决策；新决策用后续 ADR 修订，不抹除历史 |

## 4. 当前实施阶段

当前主线是 `M1 Durable Agent Run`：

```text
Finance V1 契约基线（已建立）
→ Shared Core / Versioned Finance Profile（第一批已完成，待验收）
→ M1 Durable Agent Run（当前）
→ M2 Tool Execution Gateway
→ M3 Connector Runtime
→ M4 Context / Skill / Memory
→ M5 Trace / Eval / Replay
→ M6 Workbench 与受控路由
→ 新业务场景
```

在 M1/M2 完成前，不以实现新的完整 Procurement、HR、Legal、IT 或 Customer Service Agent 为主线。

## 5. 新会话固定阅读顺序

新建 Codex 会话后按以下顺序读取：

1. 根目录 `AGENTS.md`。
2. 根目录 `下一会话开发交接.md`。
3. 本文件。
4. `05_FUTURE/ENTERPRISE_AGENTIC_PLATFORM_EVOLUTION.md`。
5. `01_PROJECT/ROADMAP.md`。
6. 当前里程碑对应的 `02_ARCHITECTURE/` 和 `03_PLATFORM_SPEC/` 文档。
7. 作为回归边界的 `04_V1_FINANCE/` 文档与 Fixture。

随后使用 CodeGraph 和 `git status` 核对当前代码与未提交改动，不得仅根据文档假设代码已经实现。

## 6. 文档维护规则

- 目标变化：先更新 L1，再更新 L2。
- 进入一个里程碑：先更新该阶段需要的 L3 契约，再编码。
- API、数据库、状态或协议变化：代码和 L3 同一个交付批次更新。
- Finance 行为变化：必须明确是兼容修复还是版本升级，并更新回归 Fixture。
- 场景文档中的“设计通过”不等于代码、真实数据或生产接入通过。
- 文档中的“当前代码事实”必须标注核对日期；过期后重新使用 CodeGraph 验证。

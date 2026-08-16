# ADR-006: Trace、Audit 与 Eval 分层

> 状态：Accepted
> 日期：2026-08-16

## 决策

- Trace 记录 Workflow、Run、Model Step、ToolCall、Checkpoint 和 Interrupt 的技术执行关系。
- Audit 记录谁基于什么权限和版本执行了什么关键行动。
- Eval 独立评估任务结果、过程、安全、成本和可靠性。

三者通过稳定 ID 关联，但不共用一张无限增长的日志表，也不默认持久化完整 Prompt、Secret 或敏感文档。

## 后果

Agent 是否成功不能只看最终文本或 HTTP 200；涉及企业系统时必须结合 Tool `verify` 和业务 Outcome Eval。

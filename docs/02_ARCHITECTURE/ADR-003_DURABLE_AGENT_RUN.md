# ADR-003: Durable Agent Run 与 Checkpointer

> 状态：Accepted
> 日期：2026-08-16

## 决策

引入 Thread、Run、Step、Checkpoint 和 Interrupt。Run 支持 Start、Resume、Cancel、Budget、lease、heartbeat 和 attempt；Python Graph 使用持久化 Checkpointer。

## 不采用

- 不继续把一次同步 HTTP Graph 调用当作长期 Runtime。
- 不用 `agent_run_logs` 代替 Checkpoint。
- 不在进程内存中保存唯一运行状态。

## 验证

必须通过进程终止、重复消息、重复 Resume、等待审批重启和迟到结果等故障实验。

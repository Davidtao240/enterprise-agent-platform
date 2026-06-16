# ADR-001: 技术选型决策

> 状态：Accepted
> 日期：2026-05-20
> 项目：企业级多智能体流程自动化平台 V1

## 决策结论

V1 技术栈如下：

| 层级 | 技术选型 | 主要职责 |
|---|---|---|
| 前端工作台 | React + TypeScript + Vite | 业务入口、任务看板、人工确认、日志展示 |
| 平台主后端 | Go + Gin | 用户、权限、工作流、Agent/Tool 注册、审计、异步任务 |
| Agent 服务 | Python + FastAPI + LangGraph | 多 Agent 编排、RAG、Excel/CSV 解析、财务分析、报表生成 |
| 数据库 | PostgreSQL | 结构化业务数据 |
| 缓存/队列 | Redis + Asynq | 异步任务、重试、任务状态 |
| 文件存储 | MinIO | 上传文件、报表归档 |
| 向量检索 | Qdrant | 知识库 / 财务制度 RAG |
| 部署 | Docker Compose | 本地一键启动和演示 |

## 平台化决策

V1 只落地财务经营数据填报，但架构必须面向多业务流程平台。

必须从 V1 开始实现或预留：

- Business App
- Workflow Template
- Workflow Instance
- Agent Registry
- Tool Registry
- Agent Gateway
- Approval Task
- Audit Log

后续 HR、采购、合同、IT、客服场景通过新增配置和领域 Agent 接入，不重写主后端。

Graph/Agent/Tool 隔离策略：

- Workflow Template 显式路由 Graph
- Graph 按流程隔离
- Agent 按能力复用
- Tool 按权限隔离
- Domain Policy 做业务域约束

## 为什么 Go 做平台主后端

Go 负责企业级平台能力：

- RBAC
- 工作流状态机
- Agent Gateway
- Tool 权限
- 审计日志
- 异步任务
- 限流、超时、重试

这些能力更能体现后端/平台工程竞争力。

## 为什么 Python 做 Agent 服务

Python 负责 Agent/RAG/LLM 逻辑：

- LangGraph 生态成熟
- 文档解析和数据处理效率高
- Prompt、RAG、评测迭代快

Python 不承载主平台后端，避免 Agent 实验逻辑和平台稳定逻辑混杂。

## 为什么 TS 做前端

React + TypeScript 适合企业工作台：

- 任务看板
- 流程状态
- 人工确认
- 日志追踪
- 管理后台

## V1 不采用的方案

### 不用 NestJS 做主后端

NestJS 适合快速做 AI 全栈产品，但本项目需要突出 Go 平台工程能力。

### 不用纯 FastAPI 做主后端

纯 FastAPI 能快速闭环，但后端岗位信号弱于 Go。

### 不用 Java 做主后端

Java 更适合后续模拟存量系统 Adapter，不进入 V1 主线。

## 最终原则

- Python 是 Agent 大脑
- Go 是平台骨架
- TypeScript 是产品界面
- Java 是后续存量系统适配

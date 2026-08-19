# ADR-008: 向量检索迁移 pgvector，退役 Qdrant

> 状态：Accepted
> 日期：2026-08-18
> 里程碑：M7-D（迁移）/ M8-D（知识库产品化）
> 相关：[ADR-001 技术选型](TECH_STACK_ADR.md)（历史 Qdrant 决策）、[AGENTIC_WORKBENCH_M7_M9_DESIGN.md §4.2](../05_FUTURE/AGENTIC_WORKBENCH_M7_M9_DESIGN.md)

## 背景

V1 使用 Qdrant 承载财务制度 RAG。M7-M9 知识库升级为共享能力层组件（部门知识空间、引用溯源、租户隔离），继续维护独立向量服务会引入额外运维面与租户数据双写漂移风险。

## 决策

向量存储统一迁移至 PostgreSQL pgvector 扩展；Qdrant 容器与 Python Qdrant 客户端退役。

### 理由

1. **规模匹配**：企业内部知识库通常 ≤ 百万级 chunk，pgvector HNSW 索引满足检索延迟要求。
2. **租户隔离复用**：向量与业务同库，`tenant_id` 列 + 复合索引即可实现行级隔离，避免 Qdrant collection 与业务表的双写同步。
3. **事务一致**：知识文档状态（解析中/可用/已删除）与向量写入同库事务，杜绝"文档已删、向量仍可检索"。
4. **组件收敛**：docker-compose 服务数下降，备份/恢复面缩小，部署文档简化。

### 代价与接受

- embedding 批量写入吞吐低于专用向量库：知识库写入是低频后台任务，可接受。
- PostgreSQL 升级需携带 pgvector 扩展版本：纳入部署文档与镜像构建。

## 表设计

权威定义见 [DATABASE_SCHEMA.md](../03_PLATFORM_SPEC/DATABASE_SCHEMA.md) M8 Target：

- `knowledge_documents`：文档元数据 + 状态机 + 来源文件关联（tenant_id 隔离）。
- `knowledge_chunks`：切片内容 + `embedding vector(N)` + `tenant_id`，HNSW 索引（`vector_cosine_ops`）；维度以所选 embedding 模型为准，在 migration 中固化。

## 迁移步骤（M7-D）

1. PostgreSQL 16 镜像启用 pgvector 扩展；migration 建表、建索引。
2. Python Agent Service 检索封装切换到 pgvector 客户端路径，对外检索接口签名不变。
3. 既有财务 RAG 数据按 fixture 重新向量化导入。
4. 移除 docker-compose `qdrant` 服务、`.env` 相关变量与 Python Qdrant 依赖。
5. 回归验收：财务报告链路 RAG 检索通过；跨租户检索返回空集。

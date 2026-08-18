# Memory & Context Architecture (M4)

> 文档状态：Active Specification
> 更新日期：2026-08-18
> 目标：为 Agent 引入动态上下文、分层记忆和技能版本控制能力。

## 1. 核心概念

*   **Memory (记忆)**：Agent 可以跨对话、跨运行保留的信息。分为五个层级，以支持不同范围的记忆能力。
*   **Context (上下文)**：传递给 LLM 的最终信息集合。Context Builder 负责将 Memory、系统指令、历史对话等来源聚合、裁剪并组装成最终的 Prompt。
*   **Skill (技能)**：可复用的 Agent 能力单元，包含配置、Prompt 模板和工具绑定。Skill 具有独立的版本和生命周期。

## 2. Memory 分层模型 (Memory Layering)

Memory 按作用域 (Scope) 划分为五个层级，决定了记忆的生命周期和可见性。在 Context 构建时，高优先级的 Memory 会被优先保留。

| 层级 (Scope) | Scope ID | 生命周期 | 可见性 (ACL 默认) | 典型用途 |
|---|---|---|---|---|
| **Run** | `run_id` | 单次 Agent Run 期间 | 仅当前 Run 的 Agent | 运行时临时变量、中间计算结果 |
| **Thread** | `thread_id` | 单次对话 (Thread) 期间 | 仅当前 Thread 的参与者 | 对话历史摘要、当前讨论主题 |
| **User** | `user_id` | 长期 (可配置过期) | 仅当前用户自己 | 用户偏好、常用操作习惯、个人知识库 |
| **Team** | `team_id` 或 `business_app_code` | 长期 | 团队内所有成员 | 团队共享的业务术语定义、标准操作流程 |
| **Domain** | `domain` (如 "finance") | 长期 | 全租户/系统级 | 领域知识、业务规则定义、全局配置 |

### 2.1. Memory 数据模型

对应 `agent_memory` 表 (待建)。

```sql
CREATE TABLE agent_memory (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope VARCHAR(32) NOT NULL,          -- 'run', 'thread', 'user', 'team', 'domain'
    scope_id VARCHAR(255) NOT NULL,      -- 对应的 ID，如 run_id, user_id
    tenant_id UUID NOT NULL,             -- 强制租户隔离
    content JSONB NOT NULL,              -- 记忆内容 (结构化数据)
    acl JSONB DEFAULT '[]',              -- 访问控制列表 (为空表示仅创建者可见)
    created_by VARCHAR(128) NOT NULL,    -- 创建者身份 (ACL 为空时可见性判定依据)
    expires_at TIMESTAMPTZ,              -- 过期时间 (NULL 表示不过期)
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ               -- 软删除
);

-- 索引用于快速查询
CREATE INDEX idx_agent_memory_scope ON agent_memory(scope, scope_id);
CREATE INDEX idx_agent_memory_tenant ON agent_memory(tenant_id);
CREATE INDEX idx_agent_memory_expires ON agent_memory(expires_at) WHERE expires_at IS NOT NULL;
```

### 2.2. ACL (访问控制列表)

`acl` 字段定义了谁可以读取这条记忆。
*   **格式**: JSON 数组，包含角色或用户 ID。
*   **示例**: `["role:finance_manager", "user:12345"]`
*   **逻辑**: 读取时，请求者的角色或用户 ID 必须在 `acl` 列表中。如果 `acl` 为空或 `null`，则只有创建者本人可见（私有）。

## 3. Context Builder 流程

Context Builder 是一个管道 (Pipeline)，负责将多个来源的信息处理成最终的 LLM Prompt。

### 3.1. 处理流程

1.  **来源聚合 (Source Aggregation)**:
    *   **系统指令 (System Prompt)**: 平台级指令，定义 Agent 的核心身份和行为。
    *   **领域记忆 (Domain Memory)**: 从 `agent_memory` 中加载 `scope='domain'` 的相关条目。
    *   **团队记忆 (Team Memory)**: 加载 `scope='team'` 的相关条目。
    *   **用户记忆 (User Memory)**: 加载 `scope='user'` 且 ACL 允许的条目。
    *   **线程记忆 (Thread Memory)**: 加载 `scope='thread'` 的当前对话摘要。
    *   **历史对话 (Chat History)**: 最近的 N 轮对话 (从持久化存储中加载)。

2.  **ACL 过滤 (ACL Filtering)**:
    *   根据当前请求的 Agent/User 身份，过滤掉 `acl` 不允许的 Memory 条目。

3.  **Token 预算裁剪 (Token Budget Trimming)**:
    *   **计算总 Token**: 估算所有聚合后内容的 Token 总数。
    *   **优先级裁剪**: 如果超出预算，按以下顺序从尾部裁剪：
        1.  历史对话 (最旧的消息优先被裁掉)
        2.  Thread Memory
        3.  User Memory
        4.  Team Memory
        5.  Domain Memory (尽量保留)
        6.  System Prompt (绝对保留，除非超出极端)

4.  **组装输出 (Assembly)**:
    *   将处理后的各部分内容组装成最终的 Prompt 字符串或 `messages` 数组，按标准的 System -> Domain -> Team -> User -> Thread -> History 顺序排列。

### 3.2. API 契约

`ContextBuilder` 服务将提供 gRPC 或 HTTP API，供 Agent Service 调用。

```go
// 请求
type BuildContextRequest struct {
    TenantID    string
    AgentID     string
    UserID      string
    ThreadID    string
    RunID       string
    Domain      string
    SystemPrompt   string    // 平台级系统指令 (聚合来源之一,绝对保留)
    ViewerRoles    []string  // 当前请求者角色,用于 ACL 过滤
    ChatHistory []Message
    TokenBudget int // Token 预算上限
}

// 响应
type BuildContextResponse struct {
    SystemPrompt string
    Messages     []Message // 最终组装的消息列表
    TokenUsed    int
    Truncated    bool // 是否发生了裁剪
}
```

## 4. Skill 生命周期

Skill 是可复用的能力单元，必须通过严格的版本和状态管理来保证稳定性。

### 4.1. 状态机

Skill 的生命周期由以下状态组成：

*   **`draft` (草稿)**: 初始状态，可随时修改。
*   **`review` (待审核)**: 提交审核，内容被锁定，等待人工或自动审核。
*   **`published` (已发布)**: 审核通过，作为稳定版本可供 Agent 引用。**此状态的配置不可修改。**
*   **`deprecated` (已废弃)**: 标记为不再推荐使用，但仍可被现有 Agent 引用以保证兼容性。

#### 状态流转

`draft` -> `review` -> `published`
`published` -> `deprecated` (废弃不可逆)

### 4.2. 数据模型

Skill 可以复用现有的 `tool_registry` 表或新建 `skill_registry` 表，这里以新建为例。

```sql
CREATE TABLE skill_registry (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    skill_code VARCHAR(64) NOT NULL,     -- 唯一业务标识，如 "finance_report_gen"
    version VARCHAR(32) NOT NULL,        -- 版本号，如 "1.0.0"
    status VARCHAR(32) NOT NULL,         -- 'draft', 'review', 'published', 'deprecated'
    config_json JSONB NOT NULL,          -- Skill 配置 (Prompt, Tools, Model Params)
    created_by VARCHAR(128) NOT NULL,    -- 创建者
    reviewed_by VARCHAR(128),            -- 审核者
    published_at TIMESTAMPTZ,            -- 发布时间
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (skill_code, version)        -- 版本不可变约束
);
```
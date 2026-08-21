# ADR-010: Graph Registry 动态加载

> 状态：Accepted
> 日期：2026-08-20
> 里程碑：M8 前置架构优化（P0-2，直接决定 M8 Gate Fail-6 判定）
> 相关：[GRAPH_ROUTING_AND_ISOLATION.md](GRAPH_ROUTING_AND_ISOLATION.md)、[M8 零代码接入验证](../05_FUTURE/M8_ZERO_CODE_ONBOARDING_VERIFICATION.md)、[ADR-009 Runtime PG 化](ADR-009_RUNTIME_STORE_POSTGRES.md)

## 背景

当前 `graph_registry.py` 以硬编码 import 注册 5 个内置 Graph，`configure_graphs()` 仅在 FastAPI lifespan 启动时编译一次。这意味着：

- **安装新 Agent Package 后必须改代码 + 重启 agent-service 才能生效**；
- 这直接命中 M8 Gate 的 **Fail-6**（"安装/卸载需重启服务即 FAIL"），零代码接入承诺不成立。

## 决策

Graph Registry 拆分为**内置注册 + 动态发现**两层，并暴露内部 reload 端点：

### 1. 内置层（不变）

Finance V1 的 5 个 Graph 仍以代码内声明注册，启动即编译，保证平台自身场景零依赖外部文件。

### 2. 动态层：`app.installed_graphs` 白名单包

新业务域 Graph 以 Python 模块形式放入 `app/installed_graphs/` 包，每个模块声明模块级契约：

```python
GRAPH_KEY = "procurement_quote_review_graph"   # 必填，str
GRAPH_VERSION = "1.0.0"                          # 必填，str

def build_graph(checkpointer=None): ...          # 必填，返回 compiled graph
def build_state(body: dict) -> dict: ...         # 可选，初始状态构造
```

发现机制：`pkgutil.iter_modules` 扫描白名单包并 `importlib` 导入（变更过的模块执行 `importlib.reload`）。**只允许该包前缀内的模块被加载**——不做任意路径 import，杜绝代码注入面。

契约校验失败的模块（缺 `GRAPH_KEY`、`build_graph` 不可调用等）：**跳过注册并记入 `invalid` 上报，不阻塞其它模块**——单个三方包损坏不应阻断整个市场安装。

### 3. Reload 端点

```
POST /internal/admin/graphs/reload     (Header: X-Internal-Service-Token)
→ 200 {"before": [...], "after": [...], "added": [...], "removed": [...], "invalid": [...]}
```

- 鉴权复用 `require_internal_service`（与 Start/Resume/Cancel 同一内部令牌）；
- 触发重新发现 + 以**当前活跃 checkpointer** 重编译全部（内置 + 已安装）Graph，`(key, version)` 幂等；
- Go 侧 Agent Gallery 在 Package 安装/卸载成功后调用该端点（M8 Step 1/5 的闭环，实现于 Go 侧接入时）。

### 4. 卸载语义

模块文件移除 + reload 后：注册表不再包含该 key → 新 Run 返回 `GRAPH_NOT_FOUND`（404）。已处于执行中的 Run 持有编译产物的引用，可跑完到终态——与 M8 Gate-4"卸载即消失、历史 Run 仍可查看"对齐。服务重启后仍未卸载模块的存量非终态 Run 由 `recover()` 正常接管。

## 不采用

- **不做 LLM 路由 / 按目录任意扫描**：保持显式 `graph_key` 路由原则（AGENTS.md），动态加载只改变"注册表内容的来源"，不改变路由机制。
- **不做热替换运行中的 Graph**：reload 只影响后续 Run 的解析结果，不追踪/终止进行中任务；版本升级走新 version 号 + 新编译产物，符合 ADR-003 不可变编译语义。
- **不引入插件子进程/沙箱**：Python 同进程 import 已是既有信任边界（业务 Graph 本就跑在 agent-service 内）；进程级隔离属 M9+ 的 Graph 隔离议题，不在本 ADR 范围。

## 验证

- 单元测试：向临时包目录写入合规/违约模块 → reload 后 `added`/`invalid` 断言；移除后 reload → `removed` 断言且 `get_graph` 抛 KeyError。
- HTTP 契约测试：无令牌 401；合法令牌返回前后差集。
- M8 Gate 复验：安装采购 Graph 后调用 reload，`list_graphs()` 出现新 key，Start Run 成功，全程无服务重启。

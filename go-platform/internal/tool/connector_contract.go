package tool

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ── M3-A: Connector Contract ──
//
// 供应商无关契约:Connector 只做适配,不做 Agent 规划、不解释 Prompt、
// 不自行扩大资源范围。供应商替换不改 Runtime 核心 —— 新实现只需满足
// 本接口并通过 connector_registry 注册。
//
//	Connector(实现)
//	  ├─ Manifest()     不可变标识与发布阶段
//	  ├─ Capabilities() 能力清单(read/write + 输入输出 schema)
//	  ├─ HealthCheck()  供应商健康探测(不产生副作用)
//	  ├─ Execute()      受 Tool Gateway 委托的适配执行
//	  └─ Verify()       外部副作用验证(M2 生命周期的 /verify 数据源)

// ConnectorCapabilityKind 能力类别:read 无外部副作用,write 有。
type ConnectorCapabilityKind string

const (
	CapabilityKindRead  ConnectorCapabilityKind = "read"
	CapabilityKindWrite ConnectorCapabilityKind = "write"
)

// ConnectorExecutionStatus Connector 执行结果状态。
// 与 ToolCall 状态机对齐:succeeded/failed 为确定态,
// indeterminate 表示结果未知(网络中断等),不计入熔断失败。
type ConnectorExecutionStatus string

const (
	ConnectorStatusSucceeded     ConnectorExecutionStatus = "succeeded"
	ConnectorStatusFailed        ConnectorExecutionStatus = "failed"
	ConnectorStatusIndeterminate ConnectorExecutionStatus = "indeterminate"
)

// 发布阶段(严格递进)与 Binding 环境(严格递进)。
// 写类 capability 仅当 registry release_stage 达到 human_approved_write
// 且 binding environment 达到 shadow 才被放行;mock/sandbox 只读。
var releaseStageOrder = map[string]int{
	"mock_fixture":         1,
	"sandbox_readonly":     2,
	"shadow":               3,
	"human_approved_write": 4,
	"limited_canary":       5,
	"production":           6,
}

var bindingEnvOrder = map[string]int{
	"mock":      1,
	"sandbox":   2,
	"shadow":    3,
	"production": 4,
}

// ErrCapabilityNotAllowed 能力不在 Binding 白名单或被发布阶段门禁拒绝。
var ErrCapabilityNotAllowed = errors.New("connector capability not allowed")

// ErrConnectorContractViolation Connector 返回值违反契约。
var ErrConnectorContractViolation = errors.New("connector contract violation")

// ConnectorManifest Connector 不可变标识。
type ConnectorManifest struct {
	ConnectorCode string `json:"connector_code"`
	Version       string `json:"version"`
	ConnectorType string `json:"connector_type"` // db_read / ticket / erp / mock
	AuthType      string `json:"auth_type"`      // none / api_key / oauth2 / basic
	ReleaseStage  string `json:"release_stage"`  // 发布阶段(见 releaseStageOrder)
}

// ConnectorCapability 能力声明:name 全局唯一,Kind 决定门禁。
type ConnectorCapability struct {
	Name         string                   `json:"name"`
	Kind         ConnectorCapabilityKind  `json:"kind"`
	InputSchema  map[string]any           `json:"input_schema,omitempty"`
	OutputSchema map[string]any           `json:"output_schema,omitempty"`
}

// ConnectorHealth 健康检查结果(不产生外部副作用)。
type ConnectorHealth struct {
	Healthy   bool      `json:"healthy"`
	Detail    string    `json:"detail,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// ConnectorRequest Execute 入参:身份来自 Tool Gateway 快照,非模型自报。
// Credential 仅执行瞬间由执行器解析传入(auth_type=none 时为 nil),
// 实现不得序列化/记录该结构。
type ConnectorRequest struct {
	TenantID   string              `json:"tenant_id"`
	ToolCallID string              `json:"tool_call_id"`
	TraceID    string              `json:"trace_id,omitempty"`
	Capability string              `json:"capability"`
	Input      map[string]any      `json:"input"`
	Credential *ResolvedCredential `json:"-"` // 敏感:永不序列化
}

// ConnectorError 结构化错误(写入 tool_calls.error_json)。
type ConnectorError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Retryable 连接类错误可重试;业务拒绝不可重试。
	Retryable bool `json:"retryable"`
}

func (e *ConnectorError) Error() string {
	return fmt.Sprintf("connector error %s: %s", e.Code, e.Message)
}

// ConnectorResult Execute 出参。
type ConnectorResult struct {
	Status            ConnectorExecutionStatus `json:"status"`
	Output            map[string]any           `json:"output,omitempty"`
	ExternalRequestID string                   `json:"external_request_id,omitempty"`
	ExternalObjectID  string                   `json:"external_object_id,omitempty"`
	Error             *ConnectorError          `json:"error,omitempty"`
}

// ConnectorVerifyRequest Verify 入参:对一次已完成执行做外部核验。
type ConnectorVerifyRequest struct {
	TenantID          string         `json:"tenant_id"`
	ToolCallID        string         `json:"tool_call_id"`
	ExternalRequestID string         `json:"external_request_id,omitempty"`
	ExternalObjectID  string         `json:"external_object_id,omitempty"`
}

// ConnectorVerifyResult Verify 出参:外部系统观测到的最终事实。
type ConnectorVerifyResult struct {
	Confirmed bool             `json:"confirmed"` // 外部侧确认副作用已发生
	Detail    string           `json:"detail,omitempty"`
	Observed  map[string]any   `json:"observed,omitempty"` // 外部观测快照(脱敏)
}

// Connector 供应商无关适配契约。实现必须是无状态或并发安全:
// 同一实例会被多个 Run 并发调用。
type Connector interface {
	// Manifest 返回不可变标识;必须与 connector_registry 注册内容一致。
	Manifest() ConnectorManifest
	// Capabilities 返回能力清单;write 类能力受发布阶段门禁。
	Capabilities() []ConnectorCapability
	// HealthCheck 探测供应商可用性;不得产生外部副作用。
	HealthCheck(ctx context.Context) ConnectorHealth
	// Execute 执行一次能力调用。实现必须:
	//   1. 只使用 req.Input 中 capability 声明的参数(白名单);
	//   2. 遵守传入 ctx 的超时/取消;
	//   3. 返回 indeterminate 而非 failed 当结果未知时;
	//   4. 绝不记录/序列化 req.Credential。
	Execute(ctx context.Context, req *ConnectorRequest) (*ConnectorResult, error)
	// Verify 核验外部副作用(写类必选,读类可返回 Confirmed=true 空观测)。
	Verify(ctx context.Context, req *ConnectorVerifyRequest) (*ConnectorVerifyResult, error)
}

// CapabilityAllowed 发布阶段门禁(M3-A 核心):
//   - read 能力:任意阶段/环境放行(只读无副作用);
//   - write 能力:registry release_stage ≥ human_approved_write
//     且 binding environment ≥ shadow 才放行。
func CapabilityAllowed(releaseStage, bindingEnv, capabilityName string, capabilities []ConnectorCapability) error {
	cap, found := lookupCapability(capabilities, capabilityName)
	if !found {
		return fmt.Errorf("%w: capability '%s' not declared by connector", ErrCapabilityNotAllowed, capabilityName)
	}
	if cap.Kind != CapabilityKindWrite {
		return nil
	}
	stageRank, ok := releaseStageOrder[releaseStage]
	if !ok {
		return fmt.Errorf("%w: unknown release stage '%s'", ErrCapabilityNotAllowed, releaseStage)
	}
	envRank, ok := bindingEnvOrder[bindingEnv]
	if !ok {
		return fmt.Errorf("%w: unknown binding environment '%s'", ErrCapabilityNotAllowed, bindingEnv)
	}
	if stageRank < releaseStageOrder["human_approved_write"] {
		return fmt.Errorf("%w: write capability '%s' requires release_stage >= human_approved_write (got %s)",
			ErrCapabilityNotAllowed, capabilityName, releaseStage)
	}
	if envRank < bindingEnvOrder["shadow"] {
		return fmt.Errorf("%w: write capability '%s' requires binding environment >= shadow (got %s)",
			ErrCapabilityNotAllowed, capabilityName, bindingEnv)
	}
	return nil
}

func lookupCapability(capabilities []ConnectorCapability, name string) (ConnectorCapability, bool) {
	for _, c := range capabilities {
		if c.Name == name {
			return c, true
		}
	}
	return ConnectorCapability{}, false
}

// ── M3-C: Outbox 契约扩展 ──
//
// 可选接口:Connector 声明部分 capability 经 Outbox 投递
// (跨系统 Saga 语义:至少一次投递 + 供应商幂等 + 补偿),
// 而非直连执行。未实现该接口的 Connector(如只读 Mock)不受影响。
// 平台不硬编码任何业务操作名 —— 是否走 Outbox 由 Connector 自声明。
type OutboxConnector interface {
	Connector

	// UseOutbox 该 capability 是否经 Outbox 投递(通常为有外部副作用的 write)。
	UseOutbox(capability string) bool

	// BuildCompensation 对一条 Outbox 记录构造补偿调用。
	// execResult 为触发补偿时已知的最近执行结果(可能为 nil,如人工触发);
	// 无补偿能力返回 ok=false(Dispatcher 将记录为不可补偿)。
	BuildCompensation(entry *OutboxEntry, execResult *ConnectorResult) (capability string, input map[string]any, ok bool)
}

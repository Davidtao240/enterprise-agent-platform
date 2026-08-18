package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
)

// ── M3-A: Connector Runtime ──
//
// 串联 connector_registry(治理) + connector_bindings(租户边界) +
// Connector 实现(适配),是 Tool Gateway 调用外部系统的唯一路径:
//
//	Tool Gateway → ConnectorRuntime.Execute
//	  1. FindBinding          租户/状态校验(身份来自网关快照,非模型自报)
//	  2. 解析 registry 版本    binding 固定版本 > active 最新
//	  3. 发布阶段门禁          release_stage + environment + write 拒绝
//	  4. 进程内解析凭证        执行瞬间解密,明文不出进程
//	  5. Connector.Execute    供应商适配(白名单/超时/幂等由实现保证)
//
// 供应商替换 = 新实现满足 Connector 接口 + registry 注册新版本,Runtime 不变。

// ConnectorRuntime Connector 运行时。
type ConnectorRuntime struct {
	registry    *ConnectorRegistryRepository
	credentials *CredentialService
	mu          sync.RWMutex
	connectors  map[string]Connector // connector_code → 实现
}

// NewConnectorRuntime 创建运行时。
func NewConnectorRuntime(registry *ConnectorRegistryRepository, credentials *CredentialService) *ConnectorRuntime {
	return &ConnectorRuntime{
		registry:    registry,
		credentials: credentials,
		connectors:  make(map[string]Connector),
	}
}

// RegisterConnector 注册 Connector 实现:实例入表 + manifest 幂等写入 registry。
func (rt *ConnectorRuntime) RegisterConnector(ctx context.Context, c Connector) error {
	m := c.Manifest()
	caps, err := json.Marshal(c.Capabilities())
	if err != nil {
		return fmt.Errorf("marshal capabilities: %w", err)
	}
	if _, err := rt.registry.Register(ctx, &RegistryEntry{
		ConnectorCode:    m.ConnectorCode,
		Version:          m.Version,
		ConnectorType:    m.ConnectorType,
		CapabilitiesJSON: string(caps),
		AuthType:         m.AuthType,
		ReleaseStage:     m.ReleaseStage,
		Status:           "active",
	}); err != nil {
		return fmt.Errorf("register connector '%s' in registry: %w", m.ConnectorCode, err)
	}
	rt.mu.Lock()
	rt.connectors[m.ConnectorCode] = c
	rt.mu.Unlock()
	log.Printf("[connector-runtime] registered %s@%s (stage=%s)", m.ConnectorCode, m.Version, m.ReleaseStage)
	return nil
}

// ShouldDeferToOutbox M3-C:判断 connector+capability 是否经 Outbox 投递
// (Connector 自声明,平台不硬编码业务操作)。未注册/未实现契约返回 false。
func (rt *ConnectorRuntime) ShouldDeferToOutbox(connectorCode, capability string) bool {
	rt.mu.RLock()
	c, ok := rt.connectors[connectorCode]
	rt.mu.RUnlock()
	if !ok {
		return false
	}
	oc, isOutbox := c.(OutboxConnector)
	return isOutbox && oc.UseOutbox(capability)
}

// OutboxConnectorFor 取实现 OutboxConnector 的实例(补偿构造用)。
func (rt *ConnectorRuntime) OutboxConnectorFor(connectorCode string) (OutboxConnector, bool) {
	rt.mu.RLock()
	c, ok := rt.connectors[connectorCode]
	rt.mu.RUnlock()
	if !ok {
		return nil, false
	}
	oc, isOutbox := c.(OutboxConnector)
	return oc, isOutbox
}

// RuntimeExecuteRequest Execute 入参(Tool Gateway 组装)。
type RuntimeExecuteRequest struct {
	TenantID    string
	BindingID   string
	ToolCallID  string
	TraceID     string
	Capability  string
	Input       map[string]any
}

// Execute 经契约执行一次外部调用(全链路唯一入口)。
func (rt *ConnectorRuntime) Execute(ctx context.Context, req *RuntimeExecuteRequest) (*ConnectorResult, error) {
	// 0. 依赖守卫:未注入 registry/credentials 时显式失败(而非 panic)。
	if rt.credentials == nil || rt.registry == nil {
		return nil, fmt.Errorf("connector runtime not fully configured (registry=%v credentials=%v)",
			rt.registry != nil, rt.credentials != nil)
	}

	// 1. Binding:租户边界 + 状态。
	binding, err := rt.credentials.FindBinding(ctx, req.BindingID)
	if err != nil {
		return nil, fmt.Errorf("find binding: %w", err)
	}
	if binding.TenantID != req.TenantID {
		return nil, ErrConnectorBindingNotFound // 跨租户视同不存在
	}
	if binding.Status != "active" {
		return nil, fmt.Errorf("binding %s is %s, not active", req.BindingID, binding.Status)
	}

	// 2. Connector 实例(进程内注册)。
	rt.mu.RLock()
	connector, ok := rt.connectors[binding.ConnectorCode]
	rt.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("connector implementation '%s' not registered in runtime", binding.ConnectorCode)
	}

	// 3. Registry 版本解析:binding 固定版本优先,否则 active 最新。
	var entry *RegistryEntry
	if binding.ConnectorVersion != nil && *binding.ConnectorVersion != "" {
		entry, err = rt.registry.FindByCodeAndVersion(ctx, binding.ConnectorCode, *binding.ConnectorVersion)
	} else {
		entry, err = rt.registry.FindActiveByCode(ctx, binding.ConnectorCode)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve registry entry: %w", err)
	}
	if entry.Status == "deprecated" {
		return nil, fmt.Errorf("connector %s@%s is deprecated", binding.ConnectorCode, entry.Version)
	}

	// 4a. Binding 级 capability 白名单(空数组=不限制,兼容存量绑定)。
	if err := bindingCapabilityAllowed(binding.AllowedCapabilities, req.Capability); err != nil {
		return nil, err
	}
	// 4b. 发布阶段门禁:write 需 registry ≥ human_approved_write 且 environment ≥ shadow。
	if err := CapabilityAllowed(entry.ReleaseStage, binding.Environment, req.Capability, connector.Capabilities()); err != nil {
		return nil, err
	}

	// 5. 进程内凭证解析(auth_type=none 跳过;明文不出进程)。
	var cred *ResolvedCredential
	if connector.Manifest().AuthType != "none" {
		cred, err = rt.credentials.ResolveCredential(ctx, req.TenantID, req.BindingID)
		if err != nil {
			return nil, fmt.Errorf("resolve credential: %w", err)
		}
	}

	// 6. 契约执行。
	return connector.Execute(ctx, &ConnectorRequest{
		TenantID:   req.TenantID,
		ToolCallID: req.ToolCallID,
		TraceID:    req.TraceID,
		Capability: req.Capability,
		Input:      req.Input,
		Credential: cred,
	})
}

// Verify 经契约核验外部副作用(M2 生命周期 /verify 的数据源)。
func (rt *ConnectorRuntime) Verify(ctx context.Context, connectorCode string, req *ConnectorVerifyRequest) (*ConnectorVerifyResult, error) {
	rt.mu.RLock()
	connector, ok := rt.connectors[connectorCode]
	rt.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("connector implementation '%s' not registered in runtime", connectorCode)
	}
	return connector.Verify(ctx, req)
}

// HealthCheck 经契约探测供应商健康(不产生副作用)。
func (rt *ConnectorRuntime) HealthCheck(ctx context.Context, connectorCode string) (ConnectorHealth, error) {
	rt.mu.RLock()
	connector, ok := rt.connectors[connectorCode]
	rt.mu.RUnlock()
	if !ok {
		return ConnectorHealth{}, fmt.Errorf("connector implementation '%s' not registered in runtime", connectorCode)
	}
	return connector.HealthCheck(ctx), nil
}

// bindingCapabilityAllowed Binding 白名单:非空数组时 capability 必须在列。
func bindingCapabilityAllowed(allowedJSON, capability string) error {
	if allowedJSON == "" || allowedJSON == "[]" || allowedJSON == "null" {
		return nil
	}
	var allowed []string
	if err := json.Unmarshal([]byte(allowedJSON), &allowed); err != nil {
		return fmt.Errorf("binding allowed_capabilities malformed: %w", err)
	}
	if len(allowed) == 0 {
		return nil
	}
	for _, a := range allowed {
		if a == capability {
			return nil
		}
	}
	return fmt.Errorf("%w: capability '%s' not in binding allowlist", ErrCapabilityNotAllowed, capability)
}

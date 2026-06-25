package workflow

import (
	"encoding/json"
	"fmt"
)

// Engine 工作流引擎核心：状态机校验 + 模板解释 + 节点调度。
//
// 引擎是纯逻辑层，不依赖数据库和 HTTP。
// 所有持久化由 Service 层在调用引擎前后完成。
type Engine struct{}

// NewEngine 创建引擎实例。
func NewEngine() *Engine {
	return &Engine{}
}

// ── 状态机校验 ──

// AllowedInstanceTransitions 定义实例状态合法流转规则。
// key = 当前状态，value = 允许的目标状态集合。
var AllowedInstanceTransitions = map[string]map[string]bool{
	StatusDraft:         {StatusRunning: true, StatusCancelled: true},
	StatusRunning:       {StatusWaitingReview: true, StatusFailed: true, StatusCancelled: true},
	StatusWaitingReview: {StatusApproved: true, StatusRejected: true, StatusCancelled: true},
	StatusApproved:      {StatusArchived: true},
	StatusRejected:      {}, // 终态，不允许再流转（可归档或结束）
	StatusArchived:      {}, // 终态
	StatusFailed:        {}, // 终态（但允许 retry → 重新 running）
	StatusCancelled:     {}, // 终态
}

// ValidateInstanceTransition 校验实例状态流转是否合法。
// 返回 error 表示非法流转。
func (e *Engine) ValidateInstanceTransition(current, next string) error {
	allowed, ok := AllowedInstanceTransitions[current]
	if !ok {
		return fmt.Errorf("unknown current status: %s", current)
	}
	if !allowed[next] {
		return fmt.Errorf("invalid transition: %s → %s", current, next)
	}
	return nil
}

// AllowedNodeTransitions 定义节点状态合法流转规则。
var AllowedNodeTransitions = map[string]map[string]bool{
	NodeStatusPending:       {NodeStatusRunning: true, NodeStatusSkipped: true, NodeStatusCancelled: true},
	NodeStatusRunning:       {NodeStatusSucceeded: true, NodeStatusFailed: true, NodeStatusWaitingReview: true, NodeStatusCancelled: true},
	NodeStatusWaitingReview: {NodeStatusSucceeded: true, NodeStatusFailed: true, NodeStatusCancelled: true},
	NodeStatusFailed:        {NodeStatusRunning: true}, // 允许重试 → 重新 running
	NodeStatusSucceeded:     {},                        // 终态
	NodeStatusSkipped:       {},                        // 终态
	NodeStatusCancelled:     {},                        // 终态
}

// ValidateNodeTransition 校验节点状态流转是否合法。
func (e *Engine) ValidateNodeTransition(current, next string) error {
	allowed, ok := AllowedNodeTransitions[current]
	if !ok {
		return fmt.Errorf("unknown current node status: %s", current)
	}
	if !allowed[next] {
		return fmt.Errorf("invalid node transition: %s → %s", current, next)
	}
	return nil
}

// ── 模板解释器 ──

// ParseDefinition 将 definition_json 字符串解析为 TemplateDefinition 结构体。
// 模板定义中声明了节点列表和边，引擎据此驱动流程。
func (e *Engine) ParseDefinition(definitionJSON string) (*TemplateDefinition, error) {
	var def TemplateDefinition
	if err := json.Unmarshal([]byte(definitionJSON), &def); err != nil {
		return nil, fmt.Errorf("parse template definition: %w", err)
	}
	if err := e.ValidateDefinition(&def); err != nil {
		return nil, err
	}
	return &def, nil
}

// ValidateDefinition 校验模板定义的结构完整性。
func (e *Engine) ValidateDefinition(def *TemplateDefinition) error {
	if def == nil {
		return fmt.Errorf("template definition is nil")
	}
	if len(def.Nodes) == 0 {
		return fmt.Errorf("template definition must contain at least one node")
	}

	nodeSet := make(map[string]bool, len(def.Nodes))
	for _, node := range def.Nodes {
		if node.ID == "" {
			return fmt.Errorf("template node id is required")
		}
		if nodeSet[node.ID] {
			return fmt.Errorf("duplicate template node id: %s", node.ID)
		}
		nodeSet[node.ID] = true
	}

	for _, edge := range def.Edges {
		if edge.From == "" || edge.To == "" {
			return fmt.Errorf("template edge must contain from and to")
		}
		if !nodeSet[edge.From] {
			return fmt.Errorf("template edge references unknown from node: %s", edge.From)
		}
		if !nodeSet[edge.To] {
			return fmt.Errorf("template edge references unknown to node: %s", edge.To)
		}
	}

	return nil
}

// GetEntryNodes 返回模板的入口节点（没有入边的节点）。
// 启动工作流时，从入口节点开始执行。
func (e *Engine) GetEntryNodes(def *TemplateDefinition) []TemplateNode {
	targetSet := make(map[string]bool)
	for _, edge := range def.Edges {
		targetSet[edge.To] = true
	}

	var entries []TemplateNode
	for _, node := range def.Nodes {
		if !targetSet[node.ID] {
			entries = append(entries, node)
		}
	}
	return entries
}

// GetNextNodes 根据当前完成的节点和边条件，找出下一个要执行的节点。
//
// 参数：
//   - def: 模板定义
//   - currentNodeKey: 刚完成节点的 id
//   - edgeWhen: 完成的边条件（succeeded / approved / rejected / failed）
//
// 返回：满足条件的所有下游节点。
func (e *Engine) GetNextNodes(def *TemplateDefinition, currentNodeKey, edgeWhen string) []TemplateNode {
	nodeMap := make(map[string]TemplateNode)
	for _, n := range def.Nodes {
		nodeMap[n.ID] = n
	}

	var next []TemplateNode
	for _, edge := range def.Edges {
		if edge.From == currentNodeKey {
			// 无边条件 → 匹配任意结果
			// 有边条件 → 必须等于当前 edgeWhen
			if edge.When == "" || edge.When == edgeWhen {
				if n, ok := nodeMap[edge.To]; ok {
					next = append(next, n)
				}
			}
		}
	}
	return next
}

// FindNodeByID 根据节点 id 在模板中查找节点定义。
func (e *Engine) FindNodeByID(def *TemplateDefinition, nodeID string) (*TemplateNode, error) {
	for _, n := range def.Nodes {
		if n.ID == nodeID {
			return &n, nil
		}
	}
	return nil, fmt.Errorf("node %s not found in template", nodeID)
}

// ── 重试校验 ──

// CanRetryNode 判断节点是否可以重试。
// 条件：节点状态为 failed，且已重试次数 < 最大重试次数。
func (e *Engine) CanRetryNode(node *NodeInstance) error {
	if node.Status != NodeStatusFailed {
		return fmt.Errorf("only failed nodes can be retried, current status: %s", node.Status)
	}
	if node.RetryCount >= node.MaxRetries {
		return fmt.Errorf("retry limit reached (%d/%d)", node.RetryCount, node.MaxRetries)
	}
	return nil
}

// CanStartInstance 判断工作流实例是否可以启动。
func (e *Engine) CanStartInstance(inst *Instance) error {
	if inst.Status != StatusDraft {
		return fmt.Errorf("only draft workflows can be started, current: %s", inst.Status)
	}
	return nil
}

// CanCancelInstance 判断实例是否可以取消。
// draft、running、waiting_review 状态允许取消。
func (e *Engine) CanCancelInstance(inst *Instance) error {
	switch inst.Status {
	case StatusArchived, StatusCancelled:
		return fmt.Errorf("cannot cancel workflow in status: %s", inst.Status)
	}
	return nil
}

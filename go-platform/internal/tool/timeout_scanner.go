package tool

import (
	"context"
	"log"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
)

// ── M2-C.8: Tool Call 超时扫描器 ──
//
// executing 且 timeout_at 已过 → indeterminate(不是 failed:
// 外部系统可能已实际执行,必须走 Verify/Reconcile 对账)。
// 与 RunConvergenceScanner 同构:周期扫描 + 守卫式状态迁移。

// maxTimeoutScanBatch 单轮最大扫描数量
const maxTimeoutScanBatch = 100

// maxTimeoutScansPerCycle 防止单次扫描时间过长的保护
const maxTimeoutScansPerCycle = 5

// ToolCallTimeoutScanner 超时扫描器。
type ToolCallTimeoutScanner struct {
	toolCalls timedOutCallStore
	audit     toolAuditLogger
	interval  time.Duration
}

// timedOutCallStore 超时扫描所需仓储接口。
type timedOutCallStore interface {
	ListTimedOutExecuting(ctx context.Context, now time.Time, limit int) ([]*ToolCall, error)
	UpdateStatusGuarded(ctx context.Context, id string, expectedFrom, status ToolCallStatus, fields map[string]any) error
}

// NewToolCallTimeoutScanner 创建超时扫描器。
func NewToolCallTimeoutScanner(toolCalls timedOutCallStore, audit toolAuditLogger, interval time.Duration) *ToolCallTimeoutScanner {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &ToolCallTimeoutScanner{toolCalls: toolCalls, audit: audit, interval: interval}
}

// ScanOnce 执行一轮扫描,返回本轮转 indeterminate 的数量。
// 单轮最多扫描 maxTimeoutScansPerCycle 批次,防止单次扫描过载。
func (s *ToolCallTimeoutScanner) ScanOnce(ctx context.Context) (int, error) {
	if s.toolCalls == nil {
		return 0, nil
	}
	totalConverted := 0
	for batch := 0; batch < maxTimeoutScansPerCycle; batch++ {
		timedOut, err := s.toolCalls.ListTimedOutExecuting(ctx, time.Now().UTC(), maxTimeoutScanBatch)
		if err != nil {
			return totalConverted, err
		}
		if len(timedOut) == 0 {
			break
		}
		converted := 0
		skipped := 0
		for _, tc := range timedOut {
			err := s.toolCalls.UpdateStatusGuarded(ctx, tc.ID, ToolCallStatusExecuting, ToolCallStatusIndeterminate, map[string]any{
				"error_json": mustJSON(map[string]any{
					"code":       "TOOL_TIMEOUT",
					"message":    "execution exceeded timeout_at; external result unknown, reconcile required",
					"timeout_at": tc.TimeoutAt.Format(time.RFC3339),
				}),
			})
			if err != nil {
				skipped++
				continue
			}
			converted++
			s.auditTimeout(ctx, tc)
		}
		totalConverted += converted
		log.Printf("[tool-timeout] batch %d: scanned=%d converted=%d skipped=%d",
			batch, len(timedOut), converted, skipped)
		if len(timedOut) < maxTimeoutScanBatch {
			break
		}
	}
	return totalConverted, nil
}

// Start 周期扫描,直到 ctx 取消。
func (s *ToolCallTimeoutScanner) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := s.ScanOnce(ctx); err != nil {
				log.Printf("[tool-timeout] scan failed: %v", err)
			} else if n > 0 {
				log.Printf("[tool-timeout] converted %d timed-out tool call(s) to indeterminate", n)
			}
		}
	}
}

func (s *ToolCallTimeoutScanner) auditTimeout(ctx context.Context, tc *ToolCall) {
	if s.audit == nil {
		return
	}
	detail := string(mustJSON(map[string]any{
		"tool_call_id": tc.ID,
		"tool_id":      tc.ToolID,
		"risk_level":   tc.RiskLevel,
		"timeout_at":   tc.TimeoutAt.Format(time.RFC3339),
	}))
	businessApp := tc.BusinessAppCode
	if _, _, err := s.audit.InsertLog(ctx, audit.AuditLogEntry{
		TraceID:         tc.TraceID,
		BusinessAppCode: &businessApp,
		Action:          "tool_call.timeout",
		ResourceType:    "tool_call",
		ResourceID:      tc.ID,
		Status:          "success",
		DetailJSON:      &detail,
	}); err != nil {
		log.Printf("[tool-timeout] audit failed: %v", err)
	}
}

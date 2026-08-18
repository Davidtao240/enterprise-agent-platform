package tool

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// ── M3-B: Webhook Inbox HTTP 入口 ──
//
// 外部系统事件统一接收端点。认证方式为 HMAC-SHA256 签名
// (区别于内部服务的 InternalServiceToken,故不挂在 /internal 前缀下):
//
//	POST /webhooks/:code
//	  Headers:
//	    X-Event-Id:   供应商事件 ID(去重键)
//	    X-Signature:  hex(hmac-sha256(raw_body, secret))
//	  Body: 原始事件体(payload 视为不可信数据)
//
// 签名校验失败的事件仍落库(signature_valid=false)供审计追溯,
// 但不进入消费处理 —— 先落库后校验消费是 Inbox 模式的可恢复基础。

// WebhookHandler 处理外部 Webhook 推送。
type WebhookHandler struct {
	store *WebhookEventRepository
	// secrets connector_code → webhook secret。未配置的 connector 拒绝接收。
	secrets map[string]string
}

// NewWebhookHandler 创建 WebhookHandler。
func NewWebhookHandler(store *WebhookEventRepository, secrets map[string]string) *WebhookHandler {
	return &WebhookHandler{store: store, secrets: secrets}
}

// Ingest 处理 POST /webhooks/:code。
func (h *WebhookHandler) Ingest(c *gin.Context) {
	connectorCode := c.Param("code")
	secret, ok := h.secrets[connectorCode]
	if !ok {
		platform.APIError(c, &apierror.APIError{
			Code:    "WEBHOOK_CONNECTOR_UNKNOWN",
			Message: "no webhook secret configured for connector",
			Status:  http.StatusNotFound,
		})
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20)) // 1MiB 上限(限流保护)
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code:    "WEBHOOK_BODY_READ_FAILED",
			Message: err.Error(),
			Status:  http.StatusBadRequest,
		})
		return
	}

	// HMAC-SHA256 签名校验(constant-time compare 防时序攻击)。
	signatureValid := verifyWebhookSignature(body, c.GetHeader("X-Signature"), secret)

	externalEventID := c.GetHeader("X-Event-Id")
	if externalEventID == "" {
		// 回退:从 payload 提取 event_id(不可信数据仅作去重键,消费时再校验)。
		var probe struct {
			EventID string `json:"event_id"`
			ID      string `json:"external_event_id"`
		}
		if json.Unmarshal(body, &probe) == nil {
			if probe.EventID != "" {
				externalEventID = probe.EventID
			} else if probe.ID != "" {
				externalEventID = probe.ID
			}
		}
	}
	if externalEventID == "" {
		platform.APIError(c, &apierror.APIError{
			Code:    "WEBHOOK_EVENT_ID_MISSING",
			Message: "X-Event-Id header or payload event_id required",
			Status:  http.StatusBadRequest,
		})
		return
	}

	stored, err := h.store.Ingest(c.Request.Context(), &WebhookEvent{
		ConnectorCode:   connectorCode,
		ExternalEventID: externalEventID,
		SignatureValid:  signatureValid,
		PayloadJSON:     string(body),
	})
	if err != nil && err != ErrWebhookDuplicate {
		platform.APIError(c, &apierror.APIError{
			Code:    "WEBHOOK_INGEST_FAILED",
			Message: err.Error(),
			Status:  http.StatusInternalServerError,
		})
		return
	}

	// 签名无效:已落库(审计追溯)但明确拒绝处理。
	if !signatureValid {
		platform.APIError(c, &apierror.APIError{
			Code:    "WEBHOOK_SIGNATURE_INVALID",
			Message: "signature verification failed; event stored but will not be processed",
			Status:  http.StatusUnauthorized,
		})
		return
	}

	// 重复投递:幂等成功(200),deduplicated 标识。
	platform.Success(c, gin.H{
		"id":           stored.ID,
		"deduplicated": err == ErrWebhookDuplicate,
	})
}

// verifyWebhookSignature constant-time HMAC-SHA256 校验。
func verifyWebhookSignature(body []byte, signatureHex, secret string) bool {
	if signatureHex == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	got, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}
	return hmac.Equal(expected, got)
}

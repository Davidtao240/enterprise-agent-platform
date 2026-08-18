package tool

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── M2-D: CredentialRef 边界 ──
//
// Secret 生命周期:
//
//	创建:明文 → AES-256-GCM 加密 → credential_secrets.cipher_text
//	引用:connector_bindings.credential_ref = 'secret:<uuid>'(只有引用,无明文)
//	解析:仅执行瞬间 ResolveCredential 解密到内存,用后即弃;
//	      绝不写日志/审计/DB/模型上下文,审计只记 resolved=true + ref。
//
// 密钥:TOOL_SECRET_ENCRYPTION_KEY(环境变量)经 SHA-256 派生 32 字节,
//       key_hint 记录版本便于轮换,密钥本身永不存在于任何表中。

// ErrConnectorBindingNotFound 绑定不存在。
var ErrConnectorBindingNotFound = errors.New("connector binding not found")

// ErrConnectorBindingDisabled 绑定已停用。
var ErrConnectorBindingDisabled = errors.New("connector binding disabled")

// ErrSecretNotConfigured 加密密钥未配置(拒绝解析而非暴露明文)。
var ErrSecretNotConfigured = errors.New("tool secret encryption key not configured")

// ErrCredentialRefInvalid credential_ref 格式非法。
var ErrCredentialRefInvalid = errors.New("credential ref invalid (expect secret:<uuid>)")

// ConnectorBinding 对应 connector_bindings 表。
type ConnectorBinding struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	BusinessAppCode string     `json:"business_app_code"`
	ConnectorCode   string     `json:"connector_code"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	ConfigJSON      string     `json:"config_json"`
	CredentialRef   *string    `json:"credential_ref,omitempty"` // 'secret:<uuid>',永不回传明文
	// M3-A:发布阶段门禁列
	Environment         string  `json:"environment"`                    // mock / sandbox / shadow / production
	AllowedCapabilities string  `json:"allowed_capabilities"`           // JSON 数组;空数组=不限制(兼容存量)
	ConnectorVersion    *string `json:"connector_version,omitempty"`    // 固定版本;空=取 active 最新
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ResolvedCredential 执行瞬间解析出的凭证。
// SecretPlaintext 仅存在于内存,调用方用后即弃;
// 任何序列化/日志/审计都不得包含此结构。
type ResolvedCredential struct {
	BindingID  string
	Ref        string
	Plaintext  string // 敏感:仅执行器使用
	ResolvedAt time.Time
}

// secretCipher AES-256-GCM 加解密(密钥派生自环境变量)。
type secretCipher struct {
	aead cipher.AEAD
}

// newSecretCipher 从密钥材料派生 AES-GCM。
func newSecretCipher(keyMaterial string) (*secretCipher, error) {
	if keyMaterial == "" {
		return nil, ErrSecretNotConfigured
	}
	key := sha256.Sum256([]byte("eap-tool-secret:" + keyMaterial)) // 域分隔派生
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &secretCipher{aead: aead}, nil
}

// encrypt 明文 → base64(nonce + ciphertext + tag)。
func (c *secretCipher) encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// decrypt 解密 base64(nonce + ciphertext + tag)。
func (c *secretCipher) decrypt(encoded string) (string, error) {
	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	if len(sealed) < c.aead.NonceSize() {
		return "", errors.New("cipher text too short")
	}
	nonce, ct := sealed[:c.aead.NonceSize()], sealed[c.aead.NonceSize():]
	plain, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt credential: %w", err)
	}
	return string(plain), nil
}

// CredentialService Connector 凭证管理 + 解析边界。
type CredentialService struct {
	pool       *pgxpool.Pool
	cipher     *secretCipher
	audit      toolAuditLogger
}

// NewCredentialService 创建凭证服务;keyMaterial 为空时解析能力禁用(仍可创建引用)。
func NewCredentialService(pool *pgxpool.Pool, keyMaterial string, audit toolAuditLogger) (*CredentialService, error) {
	svc := &CredentialService{pool: pool, audit: audit}
	if keyMaterial != "" {
		c, err := newSecretCipher(keyMaterial)
		if err != nil {
			return nil, err
		}
		svc.cipher = c
	}
	return svc, nil
}

// CreateBindingRequest 创建绑定请求(credential_plaintext 仅此一次出现,
// 加密后立即丢弃,不写任何日志)。
type CreateBindingRequest struct {
	TenantID            string `json:"tenant_id" binding:"required"`
	BusinessAppCode     string `json:"business_app_code" binding:"required"`
	ConnectorCode       string `json:"connector_code" binding:"required"`
	Name                string `json:"name" binding:"required"`
	ConfigJSON          string `json:"config_json"`
	CredentialPlaintext string `json:"credential_plaintext"` // 敏感:只在请求体中存在一次
	// M3-A:发布阶段门禁
	Environment         string  `json:"environment"`              // mock / sandbox / shadow / production;空=mock 默认
	AllowedCapabilities string  `json:"allowed_capabilities"`     // JSON 数组字符串;空=不限制
	ConnectorVersion    *string `json:"connector_version"`        // 固定版本;空=active 最新
}

// CreateBinding 创建 Connector 绑定;明文凭证加密存储,返回的绑定只含 credential_ref。
// credential_secrets INSERT + connector_bindings INSERT 在同一事务中,
// 保证不会产生孤儿凭证(绑定创建失败时回滚密文)。
func (s *CredentialService) CreateBinding(ctx context.Context, req *CreateBindingRequest) (*ConnectorBinding, error) {
	if req.ConfigJSON == "" {
		req.ConfigJSON = "{}"
	}
	var credentialRef any
	var secretID string
	var cipherText string
	if req.CredentialPlaintext != "" {
		if s.cipher == nil {
			return nil, ErrSecretNotConfigured
		}
		var err error
		cipherText, err = s.cipher.encrypt(req.CredentialPlaintext)
		if err != nil {
			return nil, fmt.Errorf("encrypt credential: %w", err)
		}
		secretID = uuid.NewString()
		credentialRef = "secret:" + secretID
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if secretID != "" {
		if _, err := tx.Exec(ctx,
			`INSERT INTO credential_secrets (id, cipher_text) VALUES ($1, $2)`,
			secretID, cipherText); err != nil {
			return nil, fmt.Errorf("insert credential secret: %w", err)
		}
	}

	now := time.Now().UTC()
	if req.Environment == "" {
		req.Environment = "mock"
	}
	if req.AllowedCapabilities == "" {
		req.AllowedCapabilities = "[]"
	}
	binding := &ConnectorBinding{}
	err = tx.QueryRow(ctx,
		`INSERT INTO connector_bindings
		 (tenant_id, business_app_code, connector_code, name, config_json, credential_ref,
		  environment, allowed_capabilities, connector_version, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8::jsonb,$9,$10,$10)
		 RETURNING id, tenant_id, business_app_code, connector_code, name, status,
		           config_json::text, credential_ref, environment, allowed_capabilities::text,
		           connector_version, created_at, updated_at`,
		req.TenantID, req.BusinessAppCode, req.ConnectorCode, req.Name, req.ConfigJSON, credentialRef,
		req.Environment, req.AllowedCapabilities, req.ConnectorVersion, now,
	).Scan(&binding.ID, &binding.TenantID, &binding.BusinessAppCode, &binding.ConnectorCode,
		&binding.Name, &binding.Status, &binding.ConfigJSON, &binding.CredentialRef,
		&binding.Environment, &binding.AllowedCapabilities, &binding.ConnectorVersion,
		&binding.CreatedAt, &binding.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert connector binding: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}
	s.auditCredential(ctx, binding, "connector_binding.created", map[string]any{
		"has_credential": credentialRef != nil,
	})
	return binding, nil
}

// FindBinding 按 ID 查找绑定。
func (s *CredentialService) FindBinding(ctx context.Context, id string) (*ConnectorBinding, error) {
	binding := &ConnectorBinding{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, business_app_code, connector_code, name, status,
		        config_json::text, credential_ref, environment, allowed_capabilities::text,
		        connector_version, created_at, updated_at
		 FROM connector_bindings WHERE id = $1`, id,
	).Scan(&binding.ID, &binding.TenantID, &binding.BusinessAppCode, &binding.ConnectorCode,
		&binding.Name, &binding.Status, &binding.ConfigJSON, &binding.CredentialRef,
		&binding.Environment, &binding.AllowedCapabilities, &binding.ConnectorVersion,
		&binding.CreatedAt, &binding.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrConnectorBindingNotFound
	}
	if err != nil {
		return nil, err
	}
	return binding, nil
}

// ListBindings 列出租户绑定(永不返回明文;cipher_text 不在本表)。
func (s *CredentialService) ListBindings(ctx context.Context, tenantID string) ([]*ConnectorBinding, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, business_app_code, connector_code, name, status,
		        config_json::text, credential_ref, environment, allowed_capabilities::text,
		        connector_version, created_at, updated_at
		 FROM connector_bindings WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*ConnectorBinding
	for rows.Next() {
		b := &ConnectorBinding{}
		if err := rows.Scan(&b.ID, &b.TenantID, &b.BusinessAppCode, &b.ConnectorCode,
			&b.Name, &b.Status, &b.ConfigJSON, &b.CredentialRef,
			&b.Environment, &b.AllowedCapabilities, &b.ConnectorVersion,
			&b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, nil
}

// ResolveCredential 执行瞬间解析凭证(边界核心)。
// - 仅 internal 调用(执行器);审计只记 ref 与 resolved,不含明文;
// - binding 必须 active 且 tenant 匹配;
// - 解密失败视为凭证损坏,返回错误(不降级、不返回部分明文)。
func (s *CredentialService) ResolveCredential(ctx context.Context, tenantID, bindingID string) (*ResolvedCredential, error) {
	binding, err := s.FindBinding(ctx, bindingID)
	if err != nil {
		return nil, err
	}
	if binding.TenantID != tenantID {
		return nil, ErrConnectorBindingNotFound // 跨租户访问视同不存在,不泄露存在性
	}
	if binding.Status != "active" {
		return nil, ErrConnectorBindingDisabled
	}
	if binding.CredentialRef == nil || !strings.HasPrefix(*binding.CredentialRef, "secret:") {
		return nil, ErrCredentialRefInvalid
	}
	if s.cipher == nil {
		return nil, ErrSecretNotConfigured
	}
	secretID := strings.TrimPrefix(*binding.CredentialRef, "secret:")
	if _, err := uuid.Parse(secretID); err != nil {
		return nil, ErrCredentialRefInvalid
	}
	var cipherText string
	err = s.pool.QueryRow(ctx,
		`SELECT cipher_text FROM credential_secrets WHERE id = $1`, secretID,
	).Scan(&cipherText)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCredentialRefInvalid
	}
	if err != nil {
		return nil, err
	}
	plaintext, err := s.cipher.decrypt(cipherText)
	if err != nil {
		return nil, err
	}
	// 审计:只有 ref + resolved 标记,零明文
	s.auditCredential(ctx, binding, "connector_credential.resolved", map[string]any{
		"credential_ref": *binding.CredentialRef,
		"resolved":       true,
	})
	return &ResolvedCredential{
		BindingID:  binding.ID,
		Ref:        *binding.CredentialRef,
		Plaintext:  plaintext,
		ResolvedAt: time.Now().UTC(),
	}, nil
}

// auditCredential 凭证审计(结构上不可能包含明文:payload 只有引用与标记)。
func (s *CredentialService) auditCredential(ctx context.Context, binding *ConnectorBinding, action string, extra map[string]any) {
	if s.audit == nil {
		return
	}
	payload := map[string]any{
		"connector_binding_id": binding.ID,
		"connector_code":       binding.ConnectorCode,
		"business_app_code":    binding.BusinessAppCode,
	}
	for k, v := range extra {
		payload[k] = v
	}
	detail := string(mustJSON(payload))
	businessApp := binding.BusinessAppCode
	if _, _, err := s.audit.InsertLog(ctx, audit.AuditLogEntry{
		BusinessAppCode: &businessApp,
		Action:          action,
		ResourceType:    "connector_binding",
		ResourceID:      binding.ID,
		Status:          "success",
		DetailJSON:      &detail,
	}); err != nil {
		// 日志失败不阻塞(但不能记明文——结构上已排除)
		log.Printf("[credential] audit %s failed: %v", action, err)
	}
}

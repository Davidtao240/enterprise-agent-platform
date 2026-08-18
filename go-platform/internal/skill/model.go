package skill

import (
	"encoding/json"
	"errors"
	"time"
)

// Status Skill 生命周期状态。
type Status string

const (
	StatusDraft      Status = "draft"
	StatusReview     Status = "review"
	StatusPublished  Status = "published"
	StatusDeprecated Status = "deprecated"
)

// ErrInvalidTransition 非法状态流转。
var ErrInvalidTransition = errors.New("invalid skill status transition")

// CanTransition 状态机转换表 (Spec §4.1):
// draft -> review -> published -> deprecated;废弃不可逆。
var transitions = map[Status][]Status{
	StatusDraft:     {StatusReview},
	StatusReview:    {StatusPublished},
	StatusPublished: {StatusDeprecated},
	// deprecated 为终态,无出边
}

// CanTransition 判定 from -> to 是否为合法流转。
func CanTransition(from, to Status) bool {
	for _, next := range transitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// Skill 对应 skill_registry 表。
type Skill struct {
	ID          string          `json:"id"`
	SkillCode   string          `json:"skill_code"`
	Version     string          `json:"version"`
	Status      Status          `json:"status"`
	ConfigJSON  json.RawMessage `json:"config_json"`
	CreatedBy   string          `json:"created_by"`
	ReviewedBy  *string         `json:"reviewed_by,omitempty"`
	PublishedAt *time.Time      `json:"published_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// CreateRequest 创建 Skill 草稿请求。
type CreateRequest struct {
	SkillCode  string          `json:"skill_code" binding:"required"`
	Version    string          `json:"version" binding:"required"`
	ConfigJSON json.RawMessage `json:"config_json" binding:"required"`
	CreatedBy  string          `json:"created_by" binding:"required"`
}

package policy

import "time"

type DomainPolicy struct {
	ID                      string    `json:"id"`
	BusinessAppCode         string    `json:"business_app_code"`
	AllowedAgentDomainsJSON string    `json:"allowed_agent_domains_json"`
	AllowedToolDomainsJSON  string    `json:"allowed_tool_domains_json"`
	AllowSharedAgents       bool      `json:"allow_shared_agents"`
	AllowSharedTools        bool      `json:"allow_shared_tools"`
	HighRiskRequiresReview  bool      `json:"high_risk_requires_review"`
	Status                  string    `json:"status"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

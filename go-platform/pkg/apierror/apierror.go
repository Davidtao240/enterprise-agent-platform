// Package apierror defines unified error codes shared across the Go platform.
// These codes align with API_CONTRACT.md error codes.
package apierror

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *APIError) Error() string {
	return e.Message
}

// Standard error codes.
var (
	ErrUnauthorized         = &APIError{Code: "UNAUTHORIZED", Message: "Missing or invalid token", Status: 401}
	ErrForbidden            = &APIError{Code: "FORBIDDEN", Message: "Permission denied", Status: 403}
	ErrValidationFailed     = &APIError{Code: "VALIDATION_FAILED", Message: "Invalid request body", Status: 400}
	ErrResourceNotFound     = &APIError{Code: "RESOURCE_NOT_FOUND", Message: "Resource does not exist", Status: 404}
	ErrWorkflowInvalidState = &APIError{Code: "WORKFLOW_INVALID_STATE", Message: "Invalid state transition", Status: 400}
	ErrWorkflowAlreadyStarted = &APIError{Code: "WORKFLOW_ALREADY_STARTED", Message: "Workflow has already been started", Status: 400}
	ErrNodeRetryExhausted   = &APIError{Code: "NODE_RETRY_EXHAUSTED", Message: "Node retry limit reached", Status: 400}
	ErrWorkflowCannotCancel = &APIError{Code: "WORKFLOW_CANNOT_CANCEL", Message: "Workflow cannot be cancelled in current state", Status: 400}
	ErrInvalidCredentials   = &APIError{Code: "UNAUTHORIZED", Message: "Invalid username or password", Status: 401}
	ErrUserDisabled         = &APIError{Code: "FORBIDDEN", Message: "User account is disabled", Status: 403}
	ErrGraphNotFound        = &APIError{Code: "GRAPH_NOT_FOUND", Message: "Graph not found", Status: 404}
	ErrAgentNotAllowed      = &APIError{Code: "AGENT_NOT_ALLOWED", Message: "Agent is not allowed in this domain", Status: 403}
	ErrToolNotAllowed       = &APIError{Code: "TOOL_NOT_ALLOWED", Message: "Tool is not allowed in this domain", Status: 403}
	ErrDomainPolicyViol     = &APIError{Code: "DOMAIN_POLICY_VIOLATION", Message: "Domain policy violation", Status: 403}
	ErrApprovalNotPending   = &APIError{Code: "APPROVAL_NOT_PENDING", Message: "Approval task is not pending", Status: 400}
	ErrAgentRunFailed       = &APIError{Code: "AGENT_RUN_FAILED", Message: "Agent graph execution failed", Status: 500}
	ErrInternalError        = &APIError{Code: "INTERNAL_ERROR", Message: "Unexpected server error", Status: 500}
)

package gpa

type ClientResult struct {
	ID         string    `json:"id"`
	Label      string    `json:"label"`
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
	Path       string    `json:"path"`
	Storage    string    `json:"storage"`
	Process    ProcState `json:"process"`
	SharedWith []string  `json:"shared_with,omitempty"`
	Email      string    `json:"email,omitempty"`
	Detail     string    `json:"detail,omitempty"`
}

type Result struct {
	Status         string         `json:"status"`
	RecoveryStatus string         `json:"recovery_status,omitempty"`
	Account        string         `json:"account,omitempty"`
	Email          string         `json:"email,omitempty"`
	Plan           string         `json:"plan,omitempty"`
	WorkspaceID    string         `json:"workspace_id,omitempty"`
	UserID         string         `json:"user_id,omitempty"`
	OperationID    string         `json:"operation_id,omitempty"`
	Clients        []ClientResult `json:"clients,omitempty"`
	Done           []string       `json:"done,omitempty"`
	Todo           []string       `json:"todo,omitempty"`
	Next           string         `json:"next,omitempty"`
	Shared         []string       `json:"shared,omitempty"`
	Error          string         `json:"error,omitempty"`
	Written        []string       `json:"written,omitempty"`
	Adopted        []string       `json:"adopted,omitempty"`
}

func (r Result) ExitCode() int {
	switch r.Status {
	case "completed":
		return int(StatusCompleted)
	case "pending":
		return int(StatusPending)
	case "blocked":
		return int(StatusBlocked)
	default:
		return int(StatusFailed)
	}
}

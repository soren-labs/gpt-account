package app

import "github.com/soren-labs/gpt-account/internal/gpa"

type Envelope struct {
	SchemaVersion int            `json:"schema_version"`
	RequestID     string         `json:"request_id,omitempty"`
	Status        string         `json:"status"`
	OperationID   string         `json:"operation_id,omitempty"`
	AccountID     string         `json:"account_id,omitempty"`
	Targets       []string       `json:"targets,omitempty"`
	ReasonCode    string         `json:"reason_code,omitempty"`
	Message       string         `json:"message,omitempty"`
	NextAction    *NextAction    `json:"next_action,omitempty"`
	Verification  map[string]any `json:"verification,omitempty"`
	Error         string         `json:"error,omitempty"`
	Demo          bool           `json:"demo,omitempty"`
}

type NextAction struct {
	Type        string `json:"type"`
	OperationID string `json:"operation_id,omitempty"`
}

type AccountView struct {
	ID                 string         `json:"id"`
	Slot               string         `json:"slot"`
	DisplayName        string         `json:"display_name"`
	LegacyAliases      []string       `json:"legacy_aliases,omitempty"`
	EmailHint          string         `json:"email_hint"`
	Email              string         `json:"email,omitempty"`
	Plan               string         `json:"plan"`
	Archived           bool           `json:"archived"`
	CredentialRevision int            `json:"credential_revision"`
	ConfiguredOn       []string       `json:"configured_on,omitempty"`
	CurrentOn          []string       `json:"current_on,omitempty"`
	Verification       map[string]any `json:"verification"`
}

type TargetView struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Members     []string `json:"members"`
	SharedNote  string   `json:"shared_note,omitempty"`
	Available   bool     `json:"available"`
	Presence    string   `json:"presence"`
	ReasonCode  string   `json:"reason_code,omitempty"`
	Detail      string   `json:"detail,omitempty"`
	CurrentHint string   `json:"current_hint,omitempty"`
}

type Plan struct {
	ID             string            `json:"id"`
	Account        string            `json:"account"`
	AccountID      string            `json:"account_id"`
	Target         string            `json:"target"`
	Members        []string          `json:"members"`
	Revision       int               `json:"revision"`
	ExpiresAt      string            `json:"expires_at"`
	Decision       string            `json:"decision"`
	ReasonCode     string            `json:"reason_code,omitempty"`
	Message        string            `json:"message,omitempty"`
	SharedNote     string            `json:"shared_note,omitempty"`
	Version        int               `json:"version"`
	Presence       map[string]string `json:"presence,omitempty"`
	AlreadyCurrent bool              `json:"already_current,omitempty"`
}

type OpRecord struct {
	ID             string            `json:"id"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at,omitempty"`
	Kind           string            `json:"kind"`
	Account        string            `json:"account"`
	AccountID      string            `json:"account_id,omitempty"`
	Target         string            `json:"target"`
	Targets        []string          `json:"targets,omitempty"`
	Status         string            `json:"status"`
	Phase          string            `json:"phase,omitempty"`
	ReasonCode     string            `json:"reason_code,omitempty"`
	Message        string            `json:"message,omitempty"`
	RequestID      string            `json:"request_id,omitempty"`
	PlanID         string            `json:"plan_id,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
	Actor          string            `json:"actor,omitempty"`
	Requests       map[string]string `json:"requests,omitempty"`
	Attempt        int               `json:"attempt,omitempty"`
	Recovery       map[string]any    `json:"recovery,omitempty"`
	Result         map[string]any    `json:"result,omitempty"`
}

type LoginRecord struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	DisplayName      string `json:"display_name,omitempty"`
	Slot             string `json:"slot,omitempty"`
	AccountID        string `json:"account_id,omitempty"`
	EmailHint        string `json:"email_hint,omitempty"`
	Plan             string `json:"plan,omitempty"`
	VerificationURL  string `json:"verification_url,omitempty"`
	UserCode         string `json:"user_code,omitempty"`
	UpdatedExisting  bool   `json:"updated_existing,omitempty"`
	IdentityMismatch bool   `json:"identity_mismatch,omitempty"`
	Message          string `json:"message,omitempty"`
	CreatedAt        string `json:"created_at"`
}

type ImportPreview struct {
	Sources   []string `json:"sources"`
	New       []string `json:"new"`
	Mergeable []string `json:"mergeable"`
	Conflicts []string `json:"conflicts"`
}

type StatusView struct {
	Demo           bool          `json:"demo,omitempty"`
	Connected      bool          `json:"connected"`
	Store          string        `json:"store"`
	SelectedTarget string        `json:"selected_target"`
	MixedCurrent   bool          `json:"mixed_current,omitempty"`
	Accounts       []AccountView `json:"accounts"`
	Targets        []TargetView  `json:"targets"`
	CurrentLabel   string        `json:"current_label,omitempty"`
	Operations     []OpRecord    `json:"operations,omitempty"`
}

type ProbeFunc func(c gpa.Client) gpa.ClientState
type SwitchFunc func(store *gpa.Store, name, target string, restartApp bool) gpa.Result
type LoginFunc func(store *gpa.Store, name string) (map[string]any, error)

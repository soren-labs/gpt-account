package gpa

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var slotNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type Identity struct {
	AuthMode     string   `json:"auth_mode"`
	WorkspaceID  string   `json:"workspace_id"`
	UserID       string   `json:"user_id"`
	Sub          string   `json:"sub"`
	Email        string   `json:"email"`
	Plan         string   `json:"plan"`
	Orgs         []string `json:"orgs"`
	HasRefresh   bool     `json:"has_refresh"`
	HasAccess    bool     `json:"has_access"`
	HasID        bool     `json:"has_id"`
	LastRefresh  string   `json:"last_refresh"`
	CredVersion  int      `json:"cred_version,omitempty"`
}

func (id Identity) Label() string {
	who := id.Email
	if who == "" {
		who = id.UserID
	}
	if who == "" {
		who = id.WorkspaceID
	}
	if who == "" {
		who = "?"
	}
	plan := id.Plan
	if plan == "" {
		plan = "?"
	}
	return who + "  " + plan
}

func (id Identity) PlanLabel() string {
	switch strings.ToLower(id.Plan) {
	case "plus":
		return "Plus"
	case "pro":
		return "Pro"
	case "free":
		return "Free"
	case "team", "business":
		return "Business"
	case "enterprise":
		return "Enterprise"
	case "":
		return "?"
	default:
		return id.Plan
	}
}

func ValidSlotName(name string) bool {
	if !slotNameRe.MatchString(name) {
		return false
	}
	if name == "." || name == ".." || strings.Contains(name, "..") {
		return false
	}
	return true
}

func b64URLJSON(segment string) map[string]any {
	pad := strings.Repeat("=", (4-len(segment)%4)%4)
	raw, err := base64.URLEncoding.DecodeString(segment + pad)
	if err != nil {
		return nil
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil {
		return nil
	}
	return obj
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return ""
	}
}

func InspectAuth(auth map[string]any) Identity {
	if auth == nil {
		return Identity{}
	}
	tokens := asMap(auth["tokens"])
	if tokens == nil {
		tokens = map[string]any{}
	}
	workspaceID := strings.TrimSpace(asString(tokens["account_id"]))
	idToken := asString(tokens["id_token"])
	var payload map[string]any
	if parts := strings.Split(idToken, "."); len(parts) >= 2 {
		payload = b64URLJSON(parts[1])
	}
	if payload == nil {
		payload = map[string]any{}
	}
	openaiAuth := asMap(payload["https://api.openai.com/auth"])
	if openaiAuth == nil {
		openaiAuth = map[string]any{}
	}
	var orgs []string
	if raw, ok := openaiAuth["organizations"].([]any); ok {
		for _, item := range raw {
			if org := asMap(item); org != nil {
				if title := asString(org["title"]); title != "" {
					orgs = append(orgs, title)
				}
			}
		}
	}
	userID := strings.TrimSpace(asString(openaiAuth["chatgpt_user_id"]))
	if userID == "" {
		userID = strings.TrimSpace(asString(openaiAuth["user_id"]))
	}
	return Identity{
		AuthMode:    asString(auth["auth_mode"]),
		WorkspaceID: workspaceID,
		UserID:      userID,
		Sub:         strings.TrimSpace(asString(payload["sub"])),
		Email:       strings.TrimSpace(asString(payload["email"])),
		Plan:        strings.TrimSpace(asString(openaiAuth["chatgpt_plan_type"])),
		Orgs:        orgs,
		HasRefresh:  asString(tokens["refresh_token"]) != "",
		HasAccess:   asString(tokens["access_token"]) != "",
		HasID:       idToken != "",
		LastRefresh: asString(auth["last_refresh"]),
	}
}

func IsChatGPTBundle(id Identity) bool {
	return id.AuthMode == "chatgpt" && id.HasRefresh
}

func SameSeat(a, b Identity) bool {
	if a.UserID != "" && b.UserID != "" {
		if a.WorkspaceID != "" && b.WorkspaceID != "" {
			return a.UserID == b.UserID && a.WorkspaceID == b.WorkspaceID
		}
		return a.UserID == b.UserID
	}
	if a.Sub != "" && b.Sub != "" {
		if a.WorkspaceID != "" && b.WorkspaceID != "" {
			return a.Sub == b.Sub && a.WorkspaceID == b.WorkspaceID
		}
		return a.Sub == b.Sub
	}
	ae := strings.ToLower(a.Email)
	be := strings.ToLower(b.Email)
	if ae != "" && be != "" {
		if a.WorkspaceID != "" && b.WorkspaceID != "" {
			return ae == be && a.WorkspaceID == b.WorkspaceID
		}
		return ae == be
	}
	return false
}

func DefaultSlotName(id Identity, taken map[string]bool) string {
	plan := strings.ToLower(id.Plan)
	switch plan {
	case "plus", "pro", "free", "team", "business", "enterprise":
		if !taken[plan] {
			return plan
		}
		local := "account"
		if id.Email != "" {
			local = strings.Split(id.Email, "@")[0]
		}
		candidate := plan + "-" + local
		if !taken[candidate] {
			return candidate
		}
	}
	raw := id.Email
	if raw == "" {
		raw = id.UserID
	}
	if raw == "" {
		raw = "account"
	}
	var b strings.Builder
	for _, r := range raw {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "account"
	}
	if !taken[slug] {
		return slug
	}
	for n := 2; ; n++ {
		cand := slug + "-" + strconv.Itoa(n)
		if !taken[cand] {
			return cand
		}
	}
}

func MakeIDToken(email, userID, plan, name string) string {
	header := strings.TrimRight(base64.URLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)), "=")
	payload := map[string]any{
		"email": email,
		"name":  name,
		"sub":   "sub-" + userID,
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_plan_type": plan,
			"chatgpt_user_id":   userID,
			"organizations":     []any{map[string]any{"title": "Personal"}},
		},
	}
	raw, _ := json.Marshal(payload)
	body := strings.TrimRight(base64.URLEncoding.EncodeToString(raw), "=")
	return header + "." + body + ".sig"
}

func FakeAuth(email, userID, plan, workspaceID, refresh string) map[string]any {
	if refresh == "" {
		refresh = "refresh-token"
	}
	return map[string]any{
		"auth_mode":    "chatgpt",
		"last_refresh": "2026-01-01T00:00:00Z",
		"tokens": map[string]any{
			"account_id":    workspaceID,
			"access_token":  "access-token",
			"refresh_token": refresh,
			"id_token":      MakeIDToken(email, userID, plan, ""),
		},
	}
}

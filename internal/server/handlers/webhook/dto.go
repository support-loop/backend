package webhook

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Request mirrors the OmniDesk webhook payload authored by us in the rule
// template (real capture 2026-10-08, see omnidesk-contract.md). All ids
// arrive as strings; CaseID is parsed explicitly. OmniDesk does not send an
// event type, so the rule template injects the event_type literal; the
// allowlist in ingest.Config validates it.
type Request struct {
	EventType       string            `json:"event_type"       validate:"required"`
	CaseID          CaseID            `json:"case_id"          validate:"required,gt=0"`
	CaseNumber      string            `json:"case_number"`
	CaseSubject     string            `json:"case_subject"`
	CaseDescription string            `json:"case_description"`
	CaseURL         string            `json:"case_url"`
	NoteText        string            `json:"note_text"`
	LastMessage     string            `json:"last_message"`
	LastMessageID   string            `json:"last_message_id"`
	CaseGroup       string            `json:"case_group"`
	CasePriority    string            `json:"case_priority"`
	CaseStatus      string            `json:"case_status"`
	CaseTags        string            `json:"case_tags"`
	StaffID         string            `json:"staff_id"`
	StaffFullName   string            `json:"staff_full_name"`
	UserFullName    string            `json:"user_full_name"`
	UserEmail       string            `json:"user_email"`
	UserID          string            `json:"user_id"`
	UserLang        string            `json:"user_lang"`
	CustomFields    map[string]string `json:"-"`
}

// CaseID is the OmniDesk case identifier. The webhook delivers it as a JSON
// string; it is parsed explicitly and validated with gt=0.
type CaseID int64

// UnmarshalJSON parses the string form delivered by OmniDesk. A non-string
// token yields a stdlib json error; a non-numeric string yields a
// [strconv.NumError]. Both are mapped to 400 by the webhook group's error
// handler.
func (id *CaseID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("unmarshal case id: %w", err)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("parse case id: %w", err)
	}
	*id = CaseID(n)
	return nil
}

// UnmarshalJSON collects the dynamic custom_field_<id> keys into
// CustomFields while decoding every fixed field via the alias type.
func (r *Request) UnmarshalJSON(data []byte) error {
	type alias Request
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return fmt.Errorf("unmarshal request: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal request keys: %w", err)
	}
	custom := make(map[string]string, len(raw))
	for key, value := range raw {
		if !strings.HasPrefix(key, "custom_field_") {
			continue
		}
		var s string
		if err := json.Unmarshal(value, &s); err == nil {
			custom[key] = s
		}
	}
	*r = Request(a)
	r.CustomFields = custom
	return nil
}

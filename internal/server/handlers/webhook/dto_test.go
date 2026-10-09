package webhook_test

import (
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/support-loop/backend/internal/server/handlers/webhook"
)

func validateRequest(t *testing.T, req webhook.Request) error {
	t.Helper()
	return validator.New().Var(&req, "required,dive")
}

// fullRequest returns a Request with every field set; validation cases
// mutate a copy. Keeping the literal exhaustive also satisfies exhaustruct.
func fullRequest() webhook.Request {
	return webhook.Request{
		EventType:       "ticket.message.created",
		CaseID:          123,
		CaseNumber:      "130-809267",
		CaseSubject:     "image-07-10-26-09-51.png",
		CaseDescription: "<p>description</p>",
		CaseURL:         "https://help.evotor.tech/staff/cases/record/130-809267",
		NoteText:        "",
		LastMessage:     "<p>latest message</p>",
		LastMessageID:   "638929422",
		CaseGroup:       "Поддержка",
		CasePriority:    "Низкий",
		CaseStatus:      "открытое",
		CaseTags:        "",
		StaffID:         "41878",
		StaffFullName:   "Роман",
		UserFullName:    "Яна Голованова",
		UserEmail:       "book174@bk.ru",
		UserID:          "81815346",
		UserLang:        "1",
		CustomFields:    nil,
	}
}

func TestRequestValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mutate  func(*webhook.Request)
		wantErr bool
	}{
		{name: "valid full payload", mutate: func(*webhook.Request) {}, wantErr: false},
		{
			name:    "valid without last message id",
			mutate:  func(r *webhook.Request) { r.LastMessageID = "" },
			wantErr: false,
		},
		{name: "missing event type", mutate: func(r *webhook.Request) { r.EventType = "" }, wantErr: true},
		{name: "missing case id", mutate: func(r *webhook.Request) { r.CaseID = 0 }, wantErr: true},
		{name: "negative case id", mutate: func(r *webhook.Request) { r.CaseID = -1 }, wantErr: true},
		{
			name:    "empty payload",
			mutate:  func(r *webhook.Request) { r.EventType = ""; r.CaseID = 0 },
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := fullRequest()
			tc.mutate(&req)
			err := validateRequest(t, req)
			if tc.wantErr && err == nil {
				t.Fatalf("validation = nil, want error for %+v", req)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validation = %v, want nil for %+v", err, req)
			}
		})
	}
}

// realPayload mirrors the owner's captured OmniDesk webhook payload (see
// omnidesk-contract.md): every value is a string, custom fields arrive as
// custom_field_<id>, and there is no event_type field (our rule template
// injects it).
const realPayload = `{
	"case_number": "130-809267",
	"case_id": "423208679",
	"case_subject": "image-07-10-26-09-51.png",
	"case_description": "<p>description</p>",
	"case_url": "https://help.evotor.tech/staff/cases/record/130-809267",
	"note_text": "",
	"last_message": "<p>latest message</p>",
	"case_group": "Поддержка",
	"case_priority": "Низкий",
	"case_status": "открытое",
	"case_tags": "",
	"last_message_id": "638929422",
	"last_note_id": "",
	"assigned_full_name": "Роман",
	"staff_full_name": "Роман",
	"staff_id": "41878",
	"last_staff_id": "0",
	"user_full_name": "Яна Голованова",
	"user_email": "book174@bk.ru",
	"user_id": "81815346",
	"user_lang": "1",
	"custom_field_6260": "Электронный платеж",
	"category": ""
}`

// TestRequestUnmarshalRealPayload proves the DTO tolerates the full real
// OmniDesk payload: string ids parse, unknown fields are ignored, and custom
// fields land in the map.
func TestRequestUnmarshalRealPayload(t *testing.T) {
	t.Parallel()
	var req webhook.Request
	if err := json.Unmarshal([]byte(realPayload), &req); err != nil {
		t.Fatalf("unmarshal real payload: %v", err)
	}
	if req.CaseID != 423208679 {
		t.Fatalf("CaseID = %d, want 423208679", req.CaseID)
	}
	if req.LastMessageID != "638929422" {
		t.Fatalf("LastMessageID = %q, want 638929422", req.LastMessageID)
	}
	if req.StaffID != "41878" {
		t.Fatalf("StaffID = %q, want 41878", req.StaffID)
	}
	if req.UserID != "81815346" {
		t.Fatalf("UserID = %q, want 81815346", req.UserID)
	}
	if req.UserLang != "1" {
		t.Fatalf("UserLang = %q, want 1", req.UserLang)
	}
	if req.CasePriority != "Низкий" {
		t.Fatalf("CasePriority = %q, want Низкий", req.CasePriority)
	}
	if got := req.CustomFields["custom_field_6260"]; got != "Электронный платеж" {
		t.Fatalf("custom_field_6260 = %q, want Электронный платеж", got)
	}
}

// TestRequestStringIDs proves ids parse from their string form and that an
// empty last_message_id is tolerated.
func TestRequestStringIDs(t *testing.T) {
	t.Parallel()
	var req webhook.Request
	if err := json.Unmarshal(
		[]byte(`{"event_type":"ticket.closed","case_id":"42","last_message_id":""}`),
		&req,
	); err != nil {
		t.Fatalf("unmarshal string ids: %v", err)
	}
	if req.CaseID != 42 {
		t.Fatalf("CaseID = %d, want 42", req.CaseID)
	}
	if req.LastMessageID != "" {
		t.Fatalf("LastMessageID = %q, want empty", req.LastMessageID)
	}
}

// TestRequestNonNumericCaseIDRejected proves a non-numeric case_id yields a
// [strconv.NumError] (mapped to 400 by the webhook group's error handler).
func TestRequestNonNumericCaseIDRejected(t *testing.T) {
	t.Parallel()
	var req webhook.Request
	err := json.Unmarshal([]byte(`{"event_type":"ticket.closed","case_id":"abc"}`), &req)
	if err == nil {
		t.Fatalf("unmarshal = nil, want error")
	}
	var numErr *strconv.NumError
	if !errors.As(err, &numErr) {
		t.Fatalf("err = %v, want *strconv.NumError", err)
	}
}

// TestRequestNumericTokenCaseIDRejected proves a JSON number (not the
// documented string form) is rejected too.
func TestRequestNumericTokenCaseIDRejected(t *testing.T) {
	t.Parallel()
	var req webhook.Request
	err := json.Unmarshal([]byte(`{"event_type":"ticket.closed","case_id":42}`), &req)
	if err == nil {
		t.Fatalf("unmarshal = nil, want error")
	}
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		t.Fatalf("err = %v, want *json.UnmarshalTypeError", err)
	}
}

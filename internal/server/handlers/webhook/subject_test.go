package webhook_test

import (
	"testing"

	"github.com/support-loop/backend/internal/server/handlers/webhook"
)

// TestResolveSubjectID proves the subject resolution: the subject_id our rule
// template injects wins, the native last_message_id is the fallback, and
// without both the subject stays empty (case-level dedup).
func TestResolveSubjectID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		req  webhook.Request
		want string
	}{
		{
			name: "injected subject id wins",
			req:  webhook.Request{SubjectID: "injected", LastMessageID: "638929422"},
			want: "injected",
		},
		{
			name: "falls back to the native last message id",
			req:  webhook.Request{LastMessageID: "638929422"},
			want: "638929422",
		},
		{
			name: "empty without any source",
			req:  webhook.Request{},
			want: "",
		},
		{
			name: "empty when the native field is empty too",
			req:  webhook.Request{SubjectID: "", LastMessageID: ""},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := webhook.ResolveSubjectIDForTest(&tc.req); got != tc.want {
				t.Fatalf("resolveSubjectID = %q, want %q", got, tc.want)
			}
		})
	}
}

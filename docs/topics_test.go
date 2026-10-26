package docs_test

import (
	"strings"
	"testing"

	"github.com/typelate/muxt/docs"
)

func TestFind(t *testing.T) {
	for _, tt := range []struct {
		id      string
		wantID  string
		wantErr string
	}{
		{id: "template-names", wantID: "template-names"},
		{id: "reference/template-names", wantID: "template-names"},
		{id: "commands/generate", wantID: "commands/generate"},
		{id: "how-to/template-functions", wantID: "how-to/template-functions"},
		{id: "template-names.md", wantID: "template-names"},
		{id: "template-nam", wantErr: "did you mean: template-names"},
		{id: "generate", wantErr: "commands/generate"},
		{id: "templ-names", wantErr: "did you mean: template-names"},
		{id: "zzz", wantErr: `unknown documentation topic "zzz"`},
	} {
		topic, _, err := docs.Find(tt.id)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Find(%q) error = %v, want containing %q", tt.id, err, tt.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("Find(%q) error = %v", tt.id, err)
			continue
		}
		if topic.ID != tt.wantID {
			t.Errorf("Find(%q).ID = %q, want %q", tt.id, topic.ID, tt.wantID)
		}
	}
}

func TestTopics(t *testing.T) {
	seen := map[string]bool{}
	for _, topic := range docs.Topics() {
		if seen[topic.ID] {
			t.Errorf("duplicate topic id %q", topic.ID)
		}
		seen[topic.ID] = true
		if topic.Title == "" || topic.Section == "" {
			t.Errorf("topic %q has title %q section %q", topic.ID, topic.Title, topic.Section)
		}
		if strings.Contains(topic.ID, "examples/") || strings.Contains(topic.ID, "superpowers") {
			t.Errorf("topic %q should not be embedded", topic.ID)
		}
	}
	if !seen["call-parameters"] {
		t.Error("missing call-parameters topic")
	}
}

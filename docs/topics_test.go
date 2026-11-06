package docs_test

import (
	"strings"
	"testing"

	"github.com/typelate/muxt/docs"
)

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

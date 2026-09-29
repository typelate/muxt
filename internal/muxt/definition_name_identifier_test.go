package muxt

import (
	"strings"
	"testing"
)

func TestGenerateEndpointPatternIdentifierParts(t *testing.T) {
	for _, tt := range []struct {
		name string
		def  Definition
		want string
	}{
		{name: "remainder wildcard drops its dots", def: Definition{method: "GET", path: "/files/{path...}"}, want: "ReadFilesByPath"},
		{name: "root wildcard", def: Definition{method: "GET", path: "/{id}"}, want: "ReadByID"},
		{name: "empty segments are skipped", def: Definition{method: "GET", path: "/a//b"}, want: "ReadAB"},
		{name: "no method", def: Definition{path: "/x"}, want: "X"},
		{name: "three parameters", def: Definition{method: "GET", path: "/{a}/{b}/{c}"}, want: "ReadByABAndC"},
		{name: "host wildcard", def: Definition{method: "GET", host: "example.com", path: "/x/{id}"}, want: "ReadExampleComXByID"},
		{name: "exact after literal", def: Definition{method: "GET", path: "/x/{$}"}, want: "ReadXExact"},
		{name: "index with a host", def: Definition{method: "POST", host: "a.b", path: "/"}, want: "CreateABIndex"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var sb strings.Builder
			sb.WriteString("stale")
			if got := tt.def.generateEndpointPatternIdentifier(&sb); got != tt.want {
				t.Errorf("generateEndpointPatternIdentifier(%q %q %q) = %q, want %q", tt.def.method, tt.def.host, tt.def.path, got, tt.want)
			}
		})
	}
}

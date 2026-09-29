package muxt

import (
	"html/template"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzNewDefinition(f *testing.F) {
	seeds := []string{
		"GET /",
		"GET /{id} GetUser(ctx, id)",
		"POST /users CreateUser(ctx, form)",
		"GET /{id} 201 GetUser(ctx, id)",
		"GET /{id} http.StatusCreated GetUser(ctx, id)",
		"PATCH /a/{b}/c/{d...} F(ctx, b, d)",
		"GET example.com/path Handler()",
		"",
		"GET",
		"GET /{}",
		"GET //double",
		"BOGUS /path F()",
		"GET /{id} F(",
		"GET /{id} F(ctx, id",
		"GET /{id} 999999999999999999999 F()",
		"GET /{id} http.StatusBogus F()",
		"GET /{id id} F()",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, name string) {
		tmpl, err := template.New(name).Parse("")
		if err != nil {
			t.Skip()
		}
		def, err, matched := newDefinition(tmpl)
		if !matched {
			return
		}
		if err != nil {
			return
		}
		// Invariants on a successfully parsed definition.
		require.NotEmpty(t, def.Path(), "parsed definition has empty path: %q", name)
		// Note: status-code range is NOT validated by the parser today —
		// "/ 00" yields 0 and "/ 700" yields 700. Worth a separate fix;
		// intentionally not asserted here so the fuzzer keeps hunting
		// for panics and inconsistent error paths.
		_ = def.DefaultStatusCode()
		// Wildcard segments must name unique path parameters.
		seen := make(map[string]struct{})
		for _, segment := range def.Segments {
			require.NotEqual(t, SegmentKindUnknown, segment.Kind(), "segment of unknown kind %q in %q", segment.Value(), name)
			if !segment.IsWildcard() {
				continue
			}
			_, dup := seen[segment.Value()]
			require.False(t, dup, "duplicate path value identifier %q in %q", segment.Value(), name)
			seen[segment.Value()] = struct{}{}
		}
	})
}

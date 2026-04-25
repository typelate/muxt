package muxt

import (
	"html/template"
	"testing"
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
		if def.Path() == "" {
			t.Fatalf("parsed definition has empty path: %q", name)
		}
		// Note: status-code range is NOT validated by the parser today —
		// "/ 00" yields 0 and "/ 700" yields 700. Worth a separate fix;
		// intentionally not asserted here so the fuzzer keeps hunting
		// for panics and inconsistent error paths.
		_ = def.DefaultStatusCode()
		// PathValueIdentifiers must not panic and must be unique.
		seen := make(map[string]struct{})
		for _, id := range def.PathValueIdentifiers() {
			if _, dup := seen[id]; dup {
				t.Fatalf("duplicate path value identifier %q in %q", id, name)
			}
			seen[id] = struct{}{}
		}
	})
}

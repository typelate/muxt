package typestest_test

import (
	"go/types"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/typestest"
)

func TestStubsTypeCheck(t *testing.T) {
	for _, path := range typestest.Paths() {
		if _, ok := typestest.Lookup(path); !ok {
			t.Errorf("Lookup(%q) found no package", path)
		}
	}
}

func TestCheck(t *testing.T) {
	t.Run("imports resolve to the stubs", func(t *testing.T) {
		pkg := typestest.MustCheck(t, "example.com/server", `package server

import (
	"context"
	"net/http"
	"time"
)

type Server struct{}

func (Server) Get(ctx context.Context, request *http.Request, at time.Time) string { return "" }
`)
		obj, _, _ := types.LookupFieldOrMethod(pkg.Scope().Lookup("Server").Type(), true, pkg, "Get")
		sig := obj.Type().(*types.Signature)
		if got := sig.Params().At(1).Type(); !types.Identical(got, types.NewPointer(typestest.Type("net/http", "Request"))) {
			t.Errorf("request parameter has type %s, want the stub *http.Request", got)
		}
	})
	t.Run("the stub time.Time is a TextUnmarshaler", func(t *testing.T) {
		unmarshaler := typestest.Type("encoding", "TextUnmarshaler").Underlying().(*types.Interface)
		if !types.Implements(types.NewPointer(typestest.Type("time", "Time")), unmarshaler) {
			t.Error("*time.Time does not implement encoding.TextUnmarshaler")
		}
	})
	t.Run("a package with no stub is an error", func(t *testing.T) {
		_, err := typestest.Check("example.com/server", map[string]string{
			"source.go": "package server\n\nimport _ \"os\"\n",
		})
		if err == nil || !strings.Contains(err.Error(), `no stub for package "os"`) {
			t.Errorf("Check error = %v, want one naming the missing stub", err)
		}
	})
}

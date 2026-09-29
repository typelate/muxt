package muxt

import (
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/source"
)

type scopeChecker map[string]types.Type

func (c scopeChecker) ScopeType(identifier string) (types.Type, error) {
	if tp, ok := c[identifier]; ok {
		return tp, nil
	}
	return nil, fmt.Errorf("no type bound to %s", identifier)
}

func (scopeChecker) FileHeader() (types.Type, error) { return nil, errors.New("no file header") }
func (scopeChecker) RawJSON() (types.Type, error)    { return nil, errors.New("no raw json") }
func (scopeChecker) TextUnmarshaler(types.Type) bool { return false }
func (scopeChecker) TextMarshaler(types.Type) bool   { return false }

const resolveCallSource = `package p

type Context interface{ Done() }

type Server struct{}

func (Server) Method(Context) any { return nil }
func (Server) Two(Context, Context) any { return nil }
func Function(Context) any { return nil }

var NotAFunction int
`

func TestResolveCall(t *testing.T) {
	pkg := checkSource(t, resolveCallSource)
	checker := scopeChecker{TemplateNameScopeIdentifierContext: pkg.Scope().Lookup("Context").Type()}
	for _, tt := range []struct {
		name         string
		call         string
		wantSig      string
		wantIsMethod bool
		wantArgs     int
		wantSynth    []string
		wantErr      string
	}{
		{name: "method", call: `Method(ctx)`, wantSig: "func(Context) any", wantIsMethod: true, wantArgs: 1},
		{name: "package function", call: `Function(ctx)`, wantSig: "func(Context) any", wantArgs: 1},
		{name: "package variable is not a function", call: `NotAFunction(ctx)`, wantSig: "func(ctx Context) any", wantIsMethod: true, wantArgs: 1, wantSynth: []string{"NotAFunction(ctx Context) any"}},
		{name: "missing method is synthesized", call: `Missing(ctx)`, wantSig: "func(ctx Context) any", wantIsMethod: true, wantArgs: 1, wantSynth: []string{"Missing(ctx Context) any"}},
		{name: "nested call", call: `Method(Function(ctx))`, wantSig: "func(Context) any", wantIsMethod: true, wantArgs: 1},
		{name: "argument count mismatch", call: `Two(ctx)`, wantErr: "handler func Two(Context, Context) any expects 2 arguments but call Two(ctx) has 1"},
		{name: "unbound identifier", call: `Method(request)`, wantErr: "no type bound to request"},
		{name: "synthesized method with a repeated argument", call: `Missing(ctx, ctx)`, wantErr: "cannot infer a signature for Missing: the ctx argument is passed more than once; define the method on the receiver to use repeated arguments"},
		{name: "synthesized method with the execute callback", call: `Missing(execute)`, wantErr: "method Missing using the execute callback must be defined on the receiver type"},
		{name: "synthesized method with an unknown identifier", call: `Missing(other)`, wantErr: "could not determine a type for other"},
		{name: "synthesized method with an unmarshalJSON body and no raw json type", call: `Missing(unmarshalJSON(body))`, wantErr: "no raw json"},
		{name: "not an identifier", call: `a.B(ctx)`, wantErr: "expected a function identifier, got: a.B"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := pkg.Scope().Lookup("Server").Type().(*types.Named)
			receiver := types.NewNamed(server.Obj(), server.Underlying(), nil)
			for method := range server.Methods() {
				receiver.AddMethod(method)
			}
			call := mustParseCall(t, tt.call)
			def := &Definition{call: call, fun: ast.NewIdent("x")}
			sig, isMethod, args, err := resolveCall(def, call, source.Package{Types: pkg}, receiver, checker)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr, "resolveCall(%s)", tt.call)
				return
			}
			require.NoError(t, err, "resolveCall(%s)", tt.call)
			assert.Equal(t, tt.wantSig, types.TypeString(sig, typeQualifier(pkg)), "resolveCall(%s) signature", tt.call)
			assert.Equal(t, tt.wantIsMethod, isMethod, "resolveCall(%s) isMethod", tt.call)
			assert.Len(t, args, tt.wantArgs, "resolveCall(%s) arguments", tt.call)
			assert.Equal(t, fmt.Sprint(tt.wantSynth), fmt.Sprint(def.synthesizedMethods), "resolveCall(%s) synthesized methods", tt.call)
		})
	}
}

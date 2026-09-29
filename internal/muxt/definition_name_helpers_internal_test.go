package muxt

import (
	"errors"
	"go/ast"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileNameToPrivateIdentifier(t *testing.T) {
	for _, tt := range []struct {
		filename string
		want     string
	}{
		{filename: "", want: ""},
		{filename: "index.gohtml", want: "index"},
		{filename: "user-profile.gohtml", want: "userProfile"},
		{filename: "Admin_Users.tmpl", want: "adminUsers"},
		{filename: "noextension", want: "noextension"},
		{filename: ".gohtml", want: ""},
	} {
		t.Run(tt.filename, func(t *testing.T) {
			assert.Equal(t, tt.want, FileNameToPrivateIdentifier(tt.filename), "FileNameToPrivateIdentifier(%q)", tt.filename)
		})
	}
}

func TestIsRouteDefinitionName(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{name: "GET /", want: true},
		{name: "/article/{id}", want: true},
		{name: "POST /article 201 Create(ctx)", want: true},
		{name: "GET example.com/ Home()", want: true},
		{name: "page.gohtml"},
		{name: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsRouteDefinitionName(tt.name), "IsRouteDefinitionName(%q)", tt.name)
		})
	}
}

func TestExportedPathIdentifier(t *testing.T) {
	for _, tt := range []struct {
		identifier string
		want       string
		wantErr    bool
	}{
		{identifier: "readIndex", want: "ReadIndex"},
		{identifier: "ReadIndex", want: "ReadIndex"},
		{identifier: "élan", want: "Élan"},
		{identifier: "_private", wantErr: true},
		{identifier: "9lives", wantErr: true},
	} {
		t.Run(tt.identifier, func(t *testing.T) {
			got, err := Definition{identifier: tt.identifier}.ExportedPathIdentifier()
			if tt.wantErr {
				assert.Error(t, err, "ExportedPathIdentifier(%q)", tt.identifier)
			} else {
				assert.NoError(t, err, "ExportedPathIdentifier(%q)", tt.identifier)
			}
			assert.Equal(t, tt.want, got, "ExportedPathIdentifier(%q)", tt.identifier)
		})
	}
}

func TestMethodNameCollisionErrorMultiLineError(t *testing.T) {
	for _, tt := range []struct {
		name      string
		locations [2]string
		want      string
	}{
		{
			name:      "both locations",
			locations: [2]string{"a.gohtml:1:2", "b.gohtml:3:4"},
			want: `TemplateRoutePaths method name collision: handlers "list" and "List" both produce method "List"
a.gohtml:1:2: "list" is defined here
b.gohtml:3:4: "List" is defined here`,
		},
		{
			name:      "second location only",
			locations: [2]string{"", "b.gohtml:3:4"},
			want: `TemplateRoutePaths method name collision: handlers "list" and "List" both produce method "List"
b.gohtml:3:4: "List" is defined here`,
		},
		{
			name: "no locations",
			want: `TemplateRoutePaths method name collision: handlers "list" and "List" both produce method "List"`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := &MethodNameCollisionError{Method: "List", Handlers: [2]string{"list", "List"}, Locations: tt.locations}
			assert.Equal(t, tt.want, err.MultiLineError(), "MultiLineError()")
		})
	}
}

func TestErrorUnwrap(t *testing.T) {
	cause := errors.New("cause")
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "NameError", err: &NameError{err: cause}},
		{name: "positionedError", err: &positionedError{err: cause}},
		{name: "ErrorList", err: ErrorList{errors.New("other"), cause}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, tt.err, cause)
		})
	}
}

func TestErrAtNode(t *testing.T) {
	node := ast.NewIdent("x")
	cause := errors.New("cause")

	t.Run("no error stays nil", func(t *testing.T) {
		assert.NoError(t, errAtNode(node, nil), "errAtNode(node, nil)")
	})
	t.Run("a positioned error is returned as it is", func(t *testing.T) {
		positioned := errAt(ast.NewIdent("y"), "already")
		assert.Same(t, positioned, errAtNode(node, positioned), "errAtNode(node, positioned)")
	})
	t.Run("a name error is returned as it is", func(t *testing.T) {
		nameError := &NameError{err: cause}
		assert.Same(t, nameError, errAtNode(node, nameError), "errAtNode(node, nameError)")
	})
	t.Run("any other error is positioned at the node", func(t *testing.T) {
		wrapped := errAtNode(node, cause)
		pe, ok := wrapped.(*positionedError)
		require.True(t, ok, "errAtNode(node, cause) = %#v, want a *positionedError", wrapped)
		assert.Equal(t, node.Pos(), pe.pos, "errAtNode(node, cause) position")
		assert.ErrorIs(t, pe, cause, "errAtNode(node, cause)")
	})
}

func TestSSECallbackSignature(t *testing.T) {
	assert.Equal(t, "func(any) error", types.TypeString(sseCallbackSignature(), nil), "sseCallbackSignature()")
}

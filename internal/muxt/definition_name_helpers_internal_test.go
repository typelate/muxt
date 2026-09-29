package muxt

import (
	"errors"
	"go/ast"
	"go/types"
	"testing"
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
		if got := FileNameToPrivateIdentifier(tt.filename); got != tt.want {
			t.Errorf("FileNameToPrivateIdentifier(%q) = %q, want %q", tt.filename, got, tt.want)
		}
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
		if got := IsRouteDefinitionName(tt.name); got != tt.want {
			t.Errorf("IsRouteDefinitionName(%q) = %t, want %t", tt.name, got, tt.want)
		}
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
		got, err := Definition{identifier: tt.identifier}.ExportedPathIdentifier()
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ExportedPathIdentifier(%q) = %q, %v, want %q, error %t", tt.identifier, got, err, tt.want, tt.wantErr)
		}
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
			if got := err.MultiLineError(); got != tt.want {
				t.Errorf("MultiLineError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorUnwrap(t *testing.T) {
	cause := errors.New("cause")
	if !errors.Is(&NameError{err: cause}, cause) {
		t.Error("errors.Is(NameError, cause) = false, want true")
	}
	if !errors.Is(&positionedError{err: cause}, cause) {
		t.Error("errors.Is(positionedError, cause) = false, want true")
	}
	if !errors.Is(ErrorList{errors.New("other"), cause}, cause) {
		t.Error("errors.Is(ErrorList, cause) = false, want true")
	}
}

func TestErrAtNode(t *testing.T) {
	node := ast.NewIdent("x")
	cause := errors.New("cause")
	if got := errAtNode(node, nil); got != nil {
		t.Errorf("errAtNode(node, nil) = %v, want nil", got)
	}
	positioned := errAt(ast.NewIdent("y"), "already")
	if got := errAtNode(node, positioned); got != positioned {
		t.Errorf("errAtNode(node, positioned) = %v, want the same error", got)
	}
	nameError := &NameError{err: cause}
	if got := errAtNode(node, nameError); got != error(nameError) {
		t.Errorf("errAtNode(node, nameError) = %v, want the same error", got)
	}
	wrapped := errAtNode(node, cause)
	if pe, ok := wrapped.(*positionedError); !ok || pe.pos != node.Pos() || !errors.Is(pe, cause) {
		t.Errorf("errAtNode(node, cause) = %#v, want a positioned error wrapping cause at the node", wrapped)
	}
}

func TestSSECallbackSignature(t *testing.T) {
	if got, want := types.TypeString(sseCallbackSignature(), nil), "func(any) error"; got != want {
		t.Errorf("sseCallbackSignature() = %s, want %s", got, want)
	}
}

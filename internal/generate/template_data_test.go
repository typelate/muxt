package generate

import (
	"go/ast"
	"go/token"
	"testing"

	"github.com/typelate/muxt/internal/astgen"
)

func TestTemplateDataMethods(t *testing.T) {
	file := scalarTestFile(t)
	config := testConfig()
	config.OutputMuxtVersion = true
	config.MuxtVersion = "v1.2.3"

	for _, tt := range []struct {
		name string
		decl *ast.FuncDecl
		want string
	}{
		{
			name: "result",
			decl: templateDataResultMethod("TemplateData"),
			want: "func (data *TemplateData[R, T]) Result() T {\n\treturn data.result\n}",
		},
		{
			name: "status code",
			decl: templateDataStatusCodeMethod("TemplateData"),
			want: "func (data *TemplateData[R, T]) StatusCode(statusCode int) *TemplateData[R, T] {\n\tdata.statusCode = statusCode\n\treturn data\n}",
		},
		{
			name: "header setter",
			decl: htmxHeaderSetterMethod("TemplateData", "HXTrigger", "HX-Trigger", "eventName"),
			want: "func (data *TemplateData[R, T]) HXTrigger(eventName string) *TemplateData[R, T] {\n\treturn data.Header(\"HX-Trigger\", eventName)\n}",
		},
		{
			name: "refresh",
			decl: htmxRefreshMethod("TemplateData"),
			want: "func (data *TemplateData[R, T]) HXRefresh() *TemplateData[R, T] {\n\treturn data.Header(\"HX-Refresh\", \"true\")\n}",
		},
		{
			name: "request header string",
			decl: htmxRequestHeaderStringMethod("TemplateData", "HXPrompt", "HX-Prompt"),
			want: "func (data *TemplateData[R, T]) HXPrompt() string {\n\treturn data.Request().Header.Get(\"HX-Prompt\")\n}",
		},
		{
			name: "request header non empty",
			decl: htmxRequestHeaderBoolNonEmptyMethod("TemplateData", "HXBoosted", "HX-Boosted"),
			want: "func (data *TemplateData[R, T]) HXBoosted() bool {\n\treturn data.Request().Header.Get(\"HX-Boosted\") != \"\"\n}",
		},
		{
			name: "request header true",
			decl: htmxRequestHeaderBoolTrueMethod("TemplateData", "HXRequest", "HX-Request"),
			want: "func (data *TemplateData[R, T]) HXRequest() bool {\n\treturn data.Request().Header.Get(\"HX-Request\") == \"true\"\n}",
		},
		{
			name: "path",
			decl: templateDataPathMethod(config),
			want: "func (data *TemplateData[R, T]) Path() TemplateRoutePaths {\n\treturn TemplateRoutePaths{pathsPrefix: data.pathsPrefix}\n}",
		},
		{
			name: "version",
			decl: templateDataMuxtVersionMethod(config),
			want: "func (data *TemplateData[R, T]) MuxtVersion() string {\n\tconst muxtVersion = \"v1.2.3\"\n\treturn muxtVersion\n}",
		},
		{
			name: "redirect helper",
			decl: templateRedirectHelperMethod(file, config, "RedirectFound", 302),
			want: "func (data *TemplateData[R, T]) RedirectFound(url string) (*TemplateData[R, T], error) {\n\treturn data.Redirect(url, http.StatusFound)\n}",
		},
		{
			name: "sse setter",
			decl: sseTemplateDataPointerSetterMethod("SSETemplateData", "Event", "event", "string", "event"),
			want: "func (m *SSETemplateData[R, T]) Event(event string) *SSETemplateData[R, T] {\n\tm.event = &event\n\treturn m\n}",
		},
		{
			name: "sse bool setter",
			decl: sseTemplateDataBoolSetterMethod("SSETemplateData", "UseViewTransition", "useViewTransition"),
			want: "func (m *SSETemplateData[R, T]) UseViewTransition(value bool) *SSETemplateData[R, T] {\n\tm.useViewTransition = value\n\treturn m\n}",
		},
		{
			name: "sse path",
			decl: sseTemplateDataPathMethod(config),
			want: "func (m *SSETemplateData[R, T]) Path() TemplateRoutePaths {\n\treturn TemplateRoutePaths{pathsPrefix: m.pathsPrefix}\n}",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := astgen.Format(tt.decl); got != tt.want {
				t.Errorf("%s =\n%s\nwant\n%s", tt.name, got, tt.want)
			}
		})
	}
}

func TestHTMXRequestHeaderBoolMethod(t *testing.T) {
	decl := htmxRequestHeaderBoolMethod("TemplateData", "Flag", "X-Flag", token.NEQ, "yes")
	want := "func (data *TemplateData[R, T]) Flag() bool {\n\treturn data.Request().Header.Get(\"X-Flag\") != \"yes\"\n}"
	if got := astgen.Format(decl); got != want {
		t.Errorf("htmxRequestHeaderBoolMethod = %s, want %s", got, want)
	}
}

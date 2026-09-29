package generate

import "testing"

func TestMarshalJSONHandlerFuncRejectsTheExecuteCallback(t *testing.T) {
	pkg, defs := routesTestDefinitions(t, map[string]string{
		"a.gohtml": `{{define "GET /e marshalJSON(Run(execute))"}}{{end}}{{define "GET /b marshalJSON(B())"}}{{end}}`,
	})
	for _, def := range defs {
		_, err := marshalJSONHandlerFunc(newFile(pkg), testConfig(), def, "td", "RoutesReceiver", "buf", "statusCode")
		switch def.RawPattern() {
		case "GET /e":
			if want := "marshalJSON does not support the execute callback"; err == nil || err.Error() != want {
				t.Errorf("marshalJSONHandlerFunc(%s) error = %v, want %q", def.RawPattern(), err, want)
			}
		default:
			if err != nil {
				t.Errorf("marshalJSONHandlerFunc(%s) = %v, want no error", def.RawPattern(), err)
			}
		}
	}
}

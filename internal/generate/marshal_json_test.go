package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarshalJSONHandlerFuncRejectsTheExecuteCallback(t *testing.T) {
	pkg, defs := routesTestDefinitions(t, map[string]string{
		"a.gohtml": `{{define "GET /e marshalJSON(Run(execute))"}}{{end}}{{define "GET /b marshalJSON(B())"}}{{end}}`,
	})
	for _, def := range defs {
		t.Run(def.RawPattern(), func(t *testing.T) {
			_, err := marshalJSONHandlerFunc(newFile(pkg), testConfig(), def, "td", "RoutesReceiver", "buf", "statusCode")
			if def.RawPattern() == "GET /e" {
				assert.EqualError(t, err, "marshalJSON does not support the execute callback", "marshalJSONHandlerFunc(%s)", def.RawPattern())
				return
			}
			assert.NoError(t, err, "marshalJSONHandlerFunc(%s)", def.RawPattern())
		})
	}
}

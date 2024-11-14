package source_test

import (
	"fmt"
	"go/ast"
	"go/types"
	"html/template"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/crhntr/dom"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/crhntr/muxt/internal/source"
)

func Test_inputValidations(t *testing.T) {
	for _, tt := range []struct {
		Name     string
		Type     types.Type
		Template string
		Result   string
		Error    string
	}{
		{
			Type:     types.Universe.Lookup("int").Type(),
			Name:     "no attributes",
			Template: `<input type="number" name="field">`,
			Result: `{
}`,
		},
		{
			Type:     types.Universe.Lookup("int").Type(),
			Name:     "min",
			Template: `<input type="number" name="field" min="100">`,
			Result: `{
	if v < 100 {
		http.Error(response, "field must not be less than 100", http.StatusBadRequest)
		return
	}
}`,
		},
		{
			Type:     types.Universe.Lookup("int").Type(),
			Name:     "negative min",
			Template: `<input type="number" name="field" min="-5">`,
			Result: `{
	if v < -5 {
		http.Error(response, "field must not be less than -5", http.StatusBadRequest)
		return
	}
}`,
		},
		{
			Type:     types.Universe.Lookup("int").Type(),
			Name:     "zero min",
			Template: `<input type="number" name="field" min="0">`,
			Result: `{
	if v < 0 {
		http.Error(response, "field must not be less than 0", http.StatusBadRequest)
		return
	}
}`,
		},
		{
			Type:     types.Universe.Lookup("int8").Type(),
			Name:     "zero min",
			Template: `<input type="number" name="field" min="0">`,
			Result: `{
	if v < 0 {
		http.Error(response, "field must not be less than 0", http.StatusBadRequest)
		return
	}
}`,
		},
		{
			Type:     types.Universe.Lookup("int16").Type(),
			Name:     "zero min",
			Template: `<input type="number" name="field" min="0">`,
			Result: `{
	if v < 0 {
		http.Error(response, "field must not be less than 0", http.StatusBadRequest)
		return
	}
}`,
		},
		{
			Type:     types.Universe.Lookup("int32").Type(),
			Name:     "zero min",
			Template: `<input type="number" name="field" min="0">`,
			Result: `{
	if v < 0 {
		http.Error(response, "field must not be less than 0", http.StatusBadRequest)
		return
	}
}`,
		},
		{
			Type:     types.Universe.Lookup("int64").Type(),
			Name:     "zero min",
			Template: `<input type="number" name="field" min="0">`,
			Result: `{
	if v < 0 {
		http.Error(response, "field must not be less than 0", http.StatusBadRequest)
		return
	}
}`,
		},
		{
			Type:     types.Universe.Lookup("uint").Type(),
			Name:     "zero min",
			Template: `<input type="number" name="field" min="0">`,
			Result: `{
	if v < 0 {
		http.Error(response, "field must not be less than 0", http.StatusBadRequest)
		return
	}
}`,
		},
		{
			Type:     types.Universe.Lookup("uint8").Type(),
			Name:     "zero min",
			Template: `<input type="number" name="field" min="0">`,
			Result: `{
	if v < 0 {
		http.Error(response, "field must not be less than 0", http.StatusBadRequest)
		return
	}
}`,
		},
		{
			Type:     types.Universe.Lookup("uint16").Type(),
			Name:     "zero min",
			Template: `<input type="number" name="field" min="0">`,
			Result: `{
	if v < 0 {
		http.Error(response, "field must not be less than 0", http.StatusBadRequest)
		return
	}
}`,
		},
		{
			Type:     types.Universe.Lookup("uint32").Type(),
			Name:     "zero min",
			Template: `<input type="number" name="field" min="0">`,
			Result: `{
	if v < 0 {
		http.Error(response, "field must not be less than 0", http.StatusBadRequest)
		return
	}
}`,
		},
		{
			Type:     types.Universe.Lookup("uint64").Type(),
			Name:     "zero min",
			Template: `<input type="number" name="field" min="0">`,
			Result: `{
	if v < 0 {
		http.Error(response, "field must not be less than 0", http.StatusBadRequest)
		return
	}
}`,
		},
		{
			Type:     types.Universe.Lookup("int").Type(),
			Name:     "out of range",
			Template: `<input type="number" name="field" min="18446744073709551616">`,
			Error:    `strconv.ParseInt: parsing "18446744073709551616": value out of range`,
		},
		{
			Type:     types.Universe.Lookup("int8").Type(),
			Name:     "out of range",
			Template: `<input type="number" name="field" min="256">`,
			Error:    `strconv.ParseInt: parsing "256": value out of range`,
		},
		{
			Type:     types.Universe.Lookup("int16").Type(),
			Name:     "out of range",
			Template: `<input type="number" name="field" min="32768">`,
			Error:    `strconv.ParseInt: parsing "32768": value out of range`,
		},
		{
			Type:     types.Universe.Lookup("int32").Type(),
			Name:     "out of range",
			Template: `<input type="number" name="field" min="2147483648">`,
			Error:    `strconv.ParseInt: parsing "2147483648": value out of range`,
		},
		{
			Type:     types.Universe.Lookup("int64").Type(),
			Name:     "out of range",
			Template: `<input type="number" name="field" min="9223372036854775808">`,
			Error:    `strconv.ParseInt: parsing "9223372036854775808": value out of range`,
		},
		{
			Type:     types.Universe.Lookup("uint").Type(),
			Name:     "out of range",
			Template: `<input type="number" name="field" min="-10">`,
			Error:    `strconv.ParseUint: parsing "-10": invalid syntax`,
		},
		{
			Type:     types.Universe.Lookup("uint8").Type(),
			Name:     "out of range",
			Template: `<input type="number" name="field" min="256">`,
			Error:    `strconv.ParseUint: parsing "256": value out of range`,
		},
		{
			Type:     types.Universe.Lookup("uint16").Type(),
			Name:     "out of range",
			Template: `<input type="number" name="field" min="65536">`,
			Error:    `strconv.ParseUint: parsing "65536": value out of range`,
		},
		{
			Type:     types.Universe.Lookup("uint32").Type(),
			Name:     "out of range",
			Template: `<input type="number" name="field" min="4294967296">`,
			Error:    `strconv.ParseUint: parsing "4294967296": value out of range`,
		},
		{
			Type:     types.Universe.Lookup("uint64").Type(),
			Name:     "out of range",
			Template: `<input type="number" name="field" min="18446744073709551616">`,
			Error:    `strconv.ParseUint: parsing "18446744073709551616": value out of range`,
		},
		{
			Type:     types.Universe.Lookup("int").Type(),
			Name:     "not a number",
			Template: `<input type="number" name="field" min="NaN">`,
			Error:    `strconv.ParseInt: parsing "NaN": invalid syntax`,
		},
		{
			Type:     types.Universe.Lookup("int").Type(),
			Name:     "wrong tag",
			Template: `<form type="number" name="field" min="32"></form>`,
			Error:    `expected element to have tag <input> got <form>`,
		},
		{
			Type:     types.Universe.Lookup("uint32").Type(),
			Name:     "zero max",
			Template: `<input type="number" name="field" max="0">`,
			Result: `{
	if v > 0 {
		http.Error(response, "field must not be more than 0", http.StatusBadRequest)
		return
	}
}`,
		},
	} {
		t.Run(fmt.Sprintf("cromulent attribute type %s %s", tt.Type, tt.Name), func(t *testing.T) {
			v := ast.NewIdent("v")
			ts := template.Must(template.New("").Parse(tt.Template))
			nodes, err := html.ParseFragment(strings.NewReader(ts.Tree.Root.String()), &html.Node{
				Type:     html.ElementNode,
				DataAtom: atom.Body,
				Data:     atom.Body.String(),
			})
			fragment := dom.NewDocumentFragment(nodes)
			imports := source.NewImports(nil)
			statements, err, ok := source.GenerateValidations(imports, v, tt.Type, `[name="field"]`, "field", "response", fragment)
			require.True(t, ok)
			if tt.Error != "" {
				require.Error(t, err)
				assert.Equal(t, tt.Error, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.Result, source.Format(&ast.BlockStmt{List: statements}))
			}
		})
	}
}

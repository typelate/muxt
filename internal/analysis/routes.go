package analysis

import (
	"bytes"
	"go/types"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/typelate/muxt/internal/muxt"
)

type DefinitionsConfiguration struct {
	Verbose            bool
	ReceiverPackage    string
	PackageName        string
	PackagePath        string
	ReceiverType       string
	TemplatesVariables []string
}

type Function struct {
	Name      string
	Signature string
}

type Definition struct {
	String    string
	Separator string
	Source    string
}

type ReceiverMethod struct {
	Name      string
	Signature string
}

type Routes struct {
	Functions       []Function
	Definitions     []Definition
	Receiver        *types.Named
	ReceiverMethods []ReceiverMethod
}

func (result *Routes) WriteTo(w io.Writer) (int64, error) {
	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, "routes.txt.template", result)
	if err != nil {
		return 0, err
	}
	return io.Copy(w, &buf)
}

// NewRoutes lists each templates variable's route definitions and
// functions, and the receiver's methods when a receiver type was named.
func NewRoutes(src muxt.Source) ([]*Routes, error) {
	receiver := src.Receiver

	var results []*Routes

	for _, tv := range src.Templates {
		if tv.Err != nil {
			return nil, tv.Err
		}
		functions := tv.Functions

		definitions, err := muxt.Definitions(tv)
		if err != nil {
			return nil, err
		}

		var funcList []Function
		names := slices.Collect(maps.Keys(functions))
		for _, name := range names {
			s := strings.TrimPrefix(functions[name].String(), "func")
			funcList = append(funcList, Function{Name: name, Signature: s})
		}

		var defList []Definition
		for _, def := range definitions {
			t := def.Template()
			if t == nil || t.Tree == nil || t.Tree.Root == nil {
				continue
			}
			src := t.Tree.Root.String()
			defList = append(defList, Definition{
				String:    def.String(),
				Separator: strings.Repeat("=", 40),
				Source:    src,
			})
		}

		result := Routes{
			Functions:   funcList,
			Definitions: defList,
		}

		if receiver != nil {
			result.Receiver = receiver
			for i := 0; i < receiver.NumMethods(); i++ {
				m := receiver.Method(i)
				result.ReceiverMethods = append(result.ReceiverMethods, ReceiverMethod{
					Name:      m.Name(),
					Signature: strings.TrimPrefix(m.Signature().String(), "func"),
				})
			}
		}
		results = append(results, &result)
	}

	return results, nil
}

package analysis

import (
	"bytes"
	"go/types"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

type DefinitionsConfiguration struct {
	Verbose            bool
	ReceiverPackage    string
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

// Override is a name the templates variable's chain defined in more than one
// place, one of them in another package's files. Replaced and Kept are
// file:line:col of the define clause the set dropped and the one it kept.
type Override struct {
	Name     string
	Replaced string
	Kept     string
}

type Routes struct {
	Functions       []Function
	Definitions     []Definition
	Receiver        *types.Named
	ReceiverMethods []ReceiverMethod
	Overrides       []Override `json:",omitempty"`
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
// functions, and the receiver's methods when a receiver type was named. Files
// in the overrides are named by the absolute path the templates carry, so an
// editor can open them.
func NewRoutes(pkg source.Package, receiver *types.Named) ([]*Routes, error) {
	var results []*Routes

	for _, tv := range pkg.Variables {
		functions := tv.Funcs

		definitions, err := muxt.Definitions(tv)
		if err != nil {
			return nil, err
		}

		var funcList []Function
		for _, name := range slices.Sorted(maps.Keys(functions)) {
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
		for _, o := range tv.Overrides {
			result.Overrides = append(result.Overrides, Override{
				Name:     o.Name,
				Replaced: o.Replaced.Position.String(),
				Kept:     o.Kept.Position.String(),
			})
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

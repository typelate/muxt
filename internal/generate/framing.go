package generate

import (
	"go/ast"

	"github.com/typelate/muxt/internal/muxt"
)

// A framingSpec supplies everything frontend-specific the generator emits:
// the render template-data declarations (type + helper methods) and the SSE
// event template-data declarations, whose WriteTo method is the event
// marshaler that frames stream events on the wire. Adding a frontend means
// registering a new spec here — the SSE transport and handler assembly are
// shared and never change per frontend.
type framingSpec struct {
	templateDataDecls func(file *File, config RoutesFileConfiguration, receiverInterface ast.Expr) []ast.Decl
	sseEventDecls     func(file *File, config RoutesFileConfiguration) []ast.Decl
}

// framingFor resolves the framingSpec for a framing. Only the default
// (unframed) spec exists today.
func framingFor(muxt.Framing) framingSpec { return defaultFraming }

var defaultFraming = framingSpec{
	templateDataDecls: defaultTemplateDataDecls,
	sseEventDecls:     sseTemplateDataDecls,
}

func defaultTemplateDataDecls(file *File, config RoutesFileConfiguration, receiverInterface ast.Expr) []ast.Decl {
	decls := []ast.Decl{
		templateDataType(file, config.TemplateDataType, receiverInterface),
		templateDataMuxtVersionMethod(config),
		templateDataPathMethod(config),
		templateDataResultMethod(config.TemplateDataType),
		templateDataRequestMethod(file, config.TemplateDataType),
		templateDataStatusCodeMethod(config.TemplateDataType),
		templateDataHeaderMethod(config.TemplateDataType),
		templateDataOkay(config.TemplateDataType),
		templateDataError(file, config.TemplateDataType),
		templateDataReceiver(receiverInterface, config.TemplateDataType),
		templateRedirect(file, config),
	}
	for _, method := range templateRedirectHelperMethods(file, config) {
		decls = append(decls, method)
	}
	decls = append(decls, templateDataStringMethod(config.TemplateDataType))
	if config.HTMXHelpers {
		for _, method := range templateDataHTMXHelperMethods(config.TemplateDataType) {
			decls = append(decls, method)
		}
	}
	return decls
}

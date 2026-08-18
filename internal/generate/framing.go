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

// framingFor resolves the framingSpec for a framing.
func framingFor(f muxt.Framing) framingSpec {
	switch f {
	case muxt.FramingHTMX:
		return htmxFraming
	case muxt.FramingDatastar:
		return datastarFraming
	default:
		return defaultFraming
	}
}

var defaultFraming = framingSpec{
	templateDataDecls: defaultTemplateDataDecls,
	sseEventDecls:     sseTemplateDataDecls,
}

// htmxFraming renders routes with the dedicated HTMX template-data type: the
// base helper surface plus the HX* response-header setters and request-header
// readers. The SSE event framing stays the generic shared one.
var htmxFraming = framingSpec{
	templateDataDecls: htmxTemplateDataDecls,
	sseEventDecls:     sseTemplateDataDecls,
}

// datastarFraming renders routes with the dedicated Datastar template-data
// type: the base helper surface under the datastar type name. The SSE event
// framing stays the generic shared one.
var datastarFraming = framingSpec{
	templateDataDecls: datastarTemplateDataDecls,
	sseEventDecls:     sseTemplateDataDecls,
}

func htmxTemplateDataDecls(file *File, config RoutesFileConfiguration, receiverInterface ast.Expr) []ast.Decl {
	htmxConfig := config
	htmxConfig.TemplateDataType = config.HTMXTemplateDataType
	decls := defaultTemplateDataDecls(file, htmxConfig, receiverInterface)
	for _, method := range templateDataHTMXHelperMethods(htmxConfig.TemplateDataType) {
		decls = append(decls, method)
	}
	return decls
}

func datastarTemplateDataDecls(file *File, config RoutesFileConfiguration, receiverInterface ast.Expr) []ast.Decl {
	dsConfig := config
	dsConfig.TemplateDataType = config.DatastarTemplateDataType
	return defaultTemplateDataDecls(file, dsConfig, receiverInterface)
}

// configForFraming returns config with the render template-data type swapped
// for the definition's framing, so handler assembly and template execution
// reference the framing's type without any assembler changes.
func configForFraming(config RoutesFileConfiguration, def muxt.Definition) RoutesFileConfiguration {
	switch def.Framing {
	case muxt.FramingHTMX:
		config.TemplateDataType = config.HTMXTemplateDataType
	case muxt.FramingDatastar:
		config.TemplateDataType = config.DatastarTemplateDataType
	}
	return config
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
	return decls
}

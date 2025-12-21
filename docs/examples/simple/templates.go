package main

import (
	"embed"
	"html/template"
)

//go:generate go run github.com/typelate/muxt generate --use-receiver-type Backend --use-receiver-type-package github.com/typelate/muxt/docs/examples/simple
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o internal/fake/routes_receiver.go -fake-name FakeBackend . RoutesReceiver

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.ParseFS(templateSource, "*"))

type Row struct {
	ID    int
	Name  string
	Value int
}

type EditRowPage struct {
	Row   Row
	Error error
}

type EditRow struct {
	Value int `name:"count" template:"count-input"`
}

// ChangeTemplateDataResult is a utility function I have found helpful for handling routes that use HTMX with re-targeting.
// It allows you to change the type of the result in the TemplateData struct based on the result of a previous step.
// It permits pushing control flow to templates. Make sure you test hypermedia functionality if you use this.
// Any generated identifier (including fields on public types) that default to private (lower case first character) may be changed in patch releases of muxt.
func ChangeTemplateDataResult[Receiver RoutesReceiver, T1, T2 any](td *TemplateData[Receiver, T1], result T2, okay bool, errList ...error) *TemplateData[Receiver, T2] {
	return &TemplateData[Receiver, T2]{receiver: td.receiver, response: td.response, request: td.request, redirectURL: td.redirectURL, statusCode: td.statusCode, result: result, okay: okay, errList: errList}
}

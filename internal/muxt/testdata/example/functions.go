package example

import (
	"context"
	"mime/multipart"
	"net/http"
	"net/url"
)

func Function() any                                      { return nil }
func FunctionHTTPRequest(*http.Request) any              { return nil }
func FunctionHTTPResponseWriter(http.ResponseWriter) any { return nil }
func FunctionContext(context.Context) any                { return nil }
func FunctionString(string) any                          { return nil }
func FunctionAny(any) any                                { return nil }
func FunctionURLValues(url.Values) any                   { return nil }
func FunctionMultipartForm(multipart.Form) any           { return nil }

func FunctionExecute(func() error) any                             { return nil }
func FunctionAnyExecute(func(any) error) any                       { return nil }
func FunctionStringExecute(func(string) error) any                 { return nil }
func FunctionIntExecute(func(int) error) any                       { return nil }
func Function2Executes(func(string) error, func(string) error) any { return nil }

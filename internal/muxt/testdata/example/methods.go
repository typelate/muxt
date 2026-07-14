package example

import (
	"context"
	"mime/multipart"
	"net/http"
	"net/url"
)

type Empty struct{}

type Server struct{}

func (srv *Server) M() any                                     { return nil }
func (srv *Server) HTTPRequest(*http.Request) any              { return nil }
func (srv *Server) HTTPResponseWriter(http.ResponseWriter) any { return nil }
func (srv *Server) Context(context.Context) any                { return nil }
func (srv *Server) String(string) any                          { return nil }
func (srv *Server) Any(any) any                                { return nil }
func (srv *Server) URLValues(url.Values) any                   { return nil }
func (srv *Server) MultipartForm(multipart.Form) any           { return nil }
func (srv *Server) MultipartFormPtr(*multipart.Form) any       { return nil }
func (srv *Server) PtrServer(*Server) any                      { return nil }

type In struct{ Name string }

func (srv *Server) FormStruct(In) any { return nil }

func (srv *Server) NoParams() error { return nil }

func (srv *Server) FieldList(ctx context.Context, postID, commentID string) any { return nil }

func (srv *Server) Function(func() error) any                            { return nil }
func (srv *Server) AnyFunction(func(any) error) any                      { return nil }
func (srv *Server) StringFunction(func(string) error) any                { return nil }
func (srv *Server) IntFunction(func(int) error) any                      { return nil }
func (srv *Server) Functions(func(string) error, func(string) error) any { return nil }

package example

import (
	"context"
	"encoding"
	"encoding/json"
	"io"
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
func (srv *Server) Reader(io.Reader) any                       { return nil }
func (srv *Server) RawJSON(json.RawMessage) any                { return nil }

// CustomError implements error to prove signals callbacks require the exact
// error result type.
type CustomError struct{}

func (CustomError) Error() string { return "custom" }

func (srv *Server) StreamBadSignals(countsSignals func(int) CustomError) {}

func (srv *Server) StreamGoodSignals(countsSignals func(int) error) {}

type In struct{ Name string }

func (srv *Server) FormStruct(In) any { return nil }

func (srv *Server) NoParams() error { return nil }

func (srv *Server) FieldList(ctx context.Context, postID, commentID string) any { return nil }

func (srv *Server) NoResults()                                     {}
func (srv *Server) TwoResultsSecondNotErrorOrBool() (int, float64) { return 0, 0 }
func (srv *Server) StringOK() (string, bool)                       { return "", false }
func (srv *Server) StringError() (string, error)                   { return "", nil }
func (srv *Server) ExecuteReturnsValue(func() error) (int, error)  { return 0, nil }
func (srv *Server) SSEReturnsValue(func(string) error) int         { return 0 }
func (srv *Server) SSEEvents(func(string) error)                   {}

type TD struct{ Value int }

func (srv *Server) ExecuteTD(func(TD) error) error             { return nil }
func (srv *Server) ExecuteNoArg(func() error) error            { return nil }
func (srv *Server) ExecuteNotFunc(string) error                { return nil }
func (srv *Server) ExecuteMultiArg(func(int, int) error) error { return nil }
func (srv *Server) SSECallbackNotFunc(string)                  {}
func (srv *Server) SSECallbackMultiArg(func(int, int) error)   {}

func (srv *Server) SSETwoCallbacks(func(string) error, func(string) error) {}

func (srv *Server) ThreeResults() (int, int, error) { return 0, 0, nil }

func (srv *Server) TwoErrors() (error, error) { return nil, nil }

func (srv *Server) Float64(float64) any  { return nil }
func (srv *Server) URLParam(url.URL) any { return nil }

// ID implements encoding.TextUnmarshaler; the interface assertion also keeps
// the encoding package in the load graph for classification.
type ID [16]byte

func (id *ID) UnmarshalText([]byte) error { return nil }

var _ encoding.TextUnmarshaler = (*ID)(nil)

func (srv *Server) TextUnmarshalerParam(ID) any { return nil }

type FormWithURL struct{ href url.URL }

func (srv *Server) FormUnsupportedField(FormWithURL) any { return nil }

type UploadForm struct {
	Name  string
	Tags  []string
	File  *multipart.FileHeader
	Files []*multipart.FileHeader
}

func (srv *Server) Upload(UploadForm) any { return nil }

type BadUploadForm struct{ File multipart.File }

func (srv *Server) BadUpload(BadUploadForm) any { return nil }

type TaggedForm struct {
	Count int      `name:"count-input" template:"count-template"`
	Tags  []string `name:"tag"`
}

func (srv *Server) TaggedForm(TaggedForm) any { return nil }

func (srv *Server) Function(func() error) any                            { return nil }
func (srv *Server) AnyFunction(func(any) error) any                      { return nil }
func (srv *Server) StringFunction(func(string) error) any                { return nil }
func (srv *Server) IntFunction(func(int) error) any                      { return nil }
func (srv *Server) Functions(func(string) error, func(string) error) any { return nil }

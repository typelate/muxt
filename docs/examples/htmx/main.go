package main

import (
	"cmp"
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync/atomic"
)

func main() {
	mux := http.NewServeMux()
	srv := new(Server)
	TemplateRoutes(mux, srv)
	log.Fatal(http.ListenAndServe(":"+cmp.Or(os.Getenv("PORT"), "8000"), mux))
}

//go:generate go run github.com/typelate/muxt generate --use-receiver-type=Server

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.ParseFS(templateSource, "*.gohtml"))

type Server struct {
	count int64
}

func (s *Server) Count() int64     { return atomic.LoadInt64(&s.count) }
func (s *Server) Decrement() int64 { return atomic.AddInt64(&s.count, -1) }
func (s *Server) Increment() int64 { return atomic.AddInt64(&s.count, 1) }

func newTemplateData[R, T any](receiver R, response http.ResponseWriter, request *http.Request, result T, okay bool) *TemplateData[R, T] {
	return &TemplateData[R, T]{receiver: receiver, response: response, request: request, result: result, okay: okay, redirectURL: ""}
}

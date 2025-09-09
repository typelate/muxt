package hypertext

import (
	"embed"
	"html/template"
	"sync/atomic"
)

//go:generate go run github.com/typelate/muxt generate --receiver-type=Server

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.ParseFS(templateSource, "*.gohtml"))

type Server struct {
	count int64
}

func (s *Server) Count() int64     { return atomic.LoadInt64(&s.count) }
func (s *Server) Decrement() int64 { return atomic.AddInt64(&s.count, -1) }
func (s *Server) Increment() int64 { return atomic.AddInt64(&s.count, 1) }

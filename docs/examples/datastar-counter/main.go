package main

import (
	"cmp"
	"context"
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
	StaticRoutes(mux)
	log.Fatal(http.ListenAndServe(":"+cmp.Or(os.Getenv("PORT"), "8001"), mux))
}

//go:generate go run github.com/typelate/muxt generate --use-receiver-type=Server --output-datastar

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.ParseFS(templateSource, "*.gohtml"))

//go:embed styles.css
var staticFS embed.FS

// StaticRoutes serves the embedded static assets alongside the generated
// template routes.
func StaticRoutes(mux *http.ServeMux) {
	mux.Handle("GET /styles.css", http.FileServerFS(staticFS))
}

// Delta is the signal patch each click streams: data-text="$delta" renders it.
type Delta struct {
	Delta string `json:"delta"`
}

type Server struct {
	count atomic.Int64
}

func (s *Server) Home(_ context.Context) (int64, error) {
	return s.count.Load(), nil
}

func (s *Server) Increment(_ context.Context, sseCount func(int64) error, deltaSignals func(Delta) error) {
	_ = sseCount(s.count.Add(1))
	_ = deltaSignals(Delta{Delta: "+1"})
}

func (s *Server) Decrement(_ context.Context, sseCount func(int64) error, deltaSignals func(Delta) error) {
	_ = sseCount(s.count.Add(-1))
	_ = deltaSignals(Delta{Delta: "-1"})
}

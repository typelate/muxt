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
	log.Fatal(http.ListenAndServe(":"+cmp.Or(os.Getenv("PORT"), "8001"), mux))
}

//go:generate go run github.com/typelate/muxt generate --use-receiver-type=Server --use-datastar

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.ParseFS(templateSource, "*.gohtml"))

// GreetSignals is the client-side signal state Datastar sends with @post: the
// reserved signals argument decodes it from the request.
type GreetSignals struct {
	Name string `json:"name"`
}

type Server struct {
	count int64
}

func (s *Server) Home(_ context.Context) (int64, error) {
	return atomic.LoadInt64(&s.count), nil
}

// Increment bumps the counter and streams one datastar-patch-elements event
// rendering the Count template; Datastar swaps it in by element id.
func (s *Server) Increment(_ context.Context, sendCount func(int64) error) {
	_ = sendCount(atomic.AddInt64(&s.count, 1))
}

// Greet reads the client's signals and streams a greeting fragment.
func (s *Server) Greet(_ context.Context, signals GreetSignals, sendGreeting func(string) error) {
	_ = sendGreeting(cmp.Or(signals.Name, "stranger"))
}

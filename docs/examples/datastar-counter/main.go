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
	"time"
)

func main() {
	mux := http.NewServeMux()
	TemplateRoutes(mux, new(Server))
	addr := ":" + cmp.Or(os.Getenv("PORT"), "8000")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

//go:generate go run github.com/typelate/muxt generate --use-receiver-type=Server --use-datastar

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.ParseFS(templateSource, "*.gohtml"))

// Server holds the counter state shared across requests.
type Server struct {
	count atomic.Int64
}

// Count is marshaled as the datastar-patch-signals body, updating the $count signal.
type Count struct {
	Count int64 `json:"count"`
}

// Index renders the page. It uses no result; the template reads .Actions.
func (s *Server) Index() any { return nil }

// Increment bumps the server counter and emits the new value as a
// datastar-patch-signals JSON response, which Datastar merges into $count.
func (s *Server) Increment(ctx context.Context, signal func(Count, bool) error) {
	_ = signal(Count{Count: s.count.Add(1)}, false)
}

// Clock streams the current time as datastar-patch-elements frames, patching the
// inner text of #clock once per second until the client disconnects.
func (s *Server) Clock(ctx context.Context, elements func(string) error) {
	for {
		if err := elements(time.Now().Format("15:04:05")); err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// Hello responds with a text/javascript body that Datastar executes.
func (s *Server) Hello(ctx context.Context, script func() error) { _ = script() }

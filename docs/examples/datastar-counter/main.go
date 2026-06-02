package main

import (
	"cmp"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"
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

// The json template function renders a value as a JSON literal. It returns
// template.HTML (trusted) because encoding/json already escapes <, >, and & —
// this lets a text/javascript (script) template embed server data safely.
var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"json": func(v any) (template.HTML, error) {
		b, err := json.Marshal(v)
		return template.HTML(b), err
	},
}).ParseFS(templateSource, "*.gohtml"))

// Server holds the demo state shared across requests.
type Server struct {
	count atomic.Int64

	mu   sync.Mutex
	feed []string
}

// Count is marshaled for datastar-patch-signals, updating the $count signal.
type Count struct {
	Count int64 `json:"count"`
}

// Status is marshaled for an inline datastar-patch-signals frame.
type Status struct {
	Status string `json:"status"`
}

// AppConfig is embedded as JSON inside the /config.js script response.
type AppConfig struct {
	Version string `json:"version"`
	Count   int64  `json:"count"`
}

// GreetForm binds the posted form fields (datastar contentType: 'form').
type GreetForm struct {
	Name string `name:"name"`
}

// Index renders the showcase page. It returns the current time so the page can
// show when it was server-rendered; the template mostly reads .Actions/.Path.
func (s *Server) Index() string { return time.Now().Format(time.RFC1123) }

// --- signals (datastar-patch-signals, application/json) across HTTP verbs ---

// Increment (@post) bumps the counter and returns the new value as a signal.
func (s *Server) Increment(ctx context.Context, signal func(Count, bool) error) {
	_ = signal(Count{Count: s.count.Add(1)}, false)
}

// Decrement (@post) lowers the counter.
func (s *Server) Decrement(ctx context.Context, signal func(Count, bool) error) {
	_ = signal(Count{Count: s.count.Add(-1)}, false)
}

// Reset (@put) sets the counter to zero.
func (s *Server) Reset(ctx context.Context, signal func(Count, bool) error) {
	s.count.Store(0)
	_ = signal(Count{Count: 0}, false)
}

// Clear (@delete) also zeroes the counter.
func (s *Server) Clear(ctx context.Context, signal func(Count, bool) error) {
	s.count.Store(0)
	_ = signal(Count{Count: 0}, false)
}

// Adjust (@patch) changes the counter by a signed path parameter (typed int).
func (s *Server) Adjust(ctx context.Context, delta int, signal func(Count, bool) error) {
	_ = signal(Count{Count: s.count.Add(int64(delta))}, false)
}

// --- elements (datastar-patch-elements, text/event-stream) ---

// Clock streams the current time as patch-elements frames into #clock (inner
// mode, with a view transition) once per second. lastEventID is accepted to
// exercise SSE resume wiring.
func (s *Server) Clock(ctx context.Context, lastEventID string, elements func(string) error) {
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

// Feed appends a few list items to #feed-list (append mode) and reports progress
// on an inline datastar-patch-signals frame, demonstrating multiple render
// callbacks on one stream plus onlyIfMissing on the final status.
func (s *Server) Feed(ctx context.Context, elements func(string) error, signalStatus func(Status, bool) error) {
	for i := 0; i < 3; i++ {
		s.mu.Lock()
		n := len(s.feed) + 1
		item := fmt.Sprintf("item %d at %s", n, time.Now().Format("15:04:05"))
		s.feed = append(s.feed, item)
		s.mu.Unlock()
		if err := elements(item); err != nil {
			return
		}
		if err := signalStatus(Status{Status: fmt.Sprintf("appended %d", n)}, false); err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(400 * time.Millisecond):
		}
	}
	// onlyIfMissing: only set $status if the client has not defined it.
	_ = signalStatus(Status{Status: "done"}, true)
}

// Greet binds the posted form (contentType: 'form') and patches #greeting.
func (s *Server) Greet(ctx context.Context, form GreetForm, elements func(string) error) {
	name := cmp.Or(form.Name, "stranger")
	_ = elements(name)
}

// --- script (text/javascript) with a JSON body ---

// Config responds with JavaScript that embeds server data as JSON.
func (s *Server) Config(ctx context.Context, script func(AppConfig) error) {
	_ = script(AppConfig{Version: "1.0", Count: s.count.Load()})
}

// --- fragment (text/html), status code, and request access ---

// Fragment returns an HTML fragment (the default representation). The route
// pattern sets a 200 status; Datastar morphs the returned #fragment-target by id.
func (s *Server) Fragment(ctx context.Context, request *http.Request) string {
	return "loaded " + request.Method + " at " + time.Now().Format("15:04:05")
}

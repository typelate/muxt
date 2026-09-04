// Package main is a TEMPORARY example for reviewing --wire-typelate-sse
// output; it is reverted on the same branch so muxt keeps no dependency on
// github.com/typelate/sse.
package main

import (
	"cmp"
	"context"
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/typelate/sse"
)

//go:generate go run github.com/typelate/muxt generate --use-receiver-type=Server --wire-typelate-sse

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

func (Server) Index() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// Clock streams one frame per second. The variadic sse.MessageOption
// parameter is forwarded by the generated closure, so the receiver can set
// per-frame metadata the template does not know, like the event id.
func (Server) Clock(ctx context.Context, execute func(time.Time, ...sse.MessageOption) error) error {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for id := 1; ; id++ {
		select {
		case <-ctx.Done():
			return nil
		case now := <-tick.C:
			if err := execute(now, sse.WithIntID(id)); err != nil {
				return err
			}
		}
	}
}

func main() {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	addr := ":" + cmp.Or(os.Getenv("PORT"), "8080")
	log.Println("using addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalln(err)
	}
}

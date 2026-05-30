package main

import (
	"cmp"
	"context"
	"embed"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"
)

//go:generate go run github.com/typelate/muxt generate --use-receiver-type=clock

//go:embed *.gohtml
var source embed.FS

var templates = template.Must(template.ParseFS(source, "*"))

type clock struct {
	location *time.Location
}

func (c clock) Time(ctx context.Context, lastEventID string, updateTime func(data string) error) {
	slog.Info("starting sse handler", slog.String("lastEventID", lastEventID))
	defer slog.Info("closed sse handler", slog.String("lastEventID", lastEventID))
	wait := time.Second
	t := time.NewTicker(wait)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s := now.In(c.location).Format(time.RFC3339)
			slog.Info("sending timestamp", slog.String("lastEventID", lastEventID), slog.String("timestamp", s))
			if err := updateTime(s); err != nil {
				slog.Error("got error while sending sse timestamp", slog.String("timestamp", s), slog.String("error", err.Error()))
				return
			}
		}
	}
}

func (c clock) Index() string {
	return time.Now().In(c.location).Format(time.RFC3339)
}

func main() {
	c := clock{location: time.UTC}
	mux := http.NewServeMux()
	TemplateRoutes(mux, c)
	addr := ":" + cmp.Or(os.Getenv("PORT"), "8080")
	log.Println("using addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalln(err)
	}
}

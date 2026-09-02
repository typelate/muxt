package main

import (
	"cmp"
	"context"
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	StaticRoutes(mux)
	log.Fatal(http.ListenAndServe(":"+cmp.Or(os.Getenv("PORT"), "8003"), mux))
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

// SearchSignals is the Datastar signal state the browser sends with @post:
// data-bind:query creates the query signal and unmarshalJSON(body) decodes it.
type SearchSignals struct {
	Query string `json:"query"`
}

// APIForm binds the same query as a URL parameter for the JSON route.
type APIForm struct {
	Query string `name:"query"`
}

type Match struct {
	Prefix string `json:"-"`
	Match  string `json:"-"`
	Suffix string `json:"-"`
	Text   string `json:"text"`
}

type SearchResults struct {
	Query   string  `json:"query"`
	Matches []Match `json:"matches"`
	Total   int     `json:"total"`
}

type Server struct{}

func (Server) Index(_ context.Context) (SearchResults, error) {
	return search(""), nil
}

func (Server) SearchProverbs(_ context.Context, signals SearchSignals, sseResults func(SearchResults) error) {
	_ = sseResults(search(signals.Query))
}

func (Server) ProverbsAPI(_ context.Context, form APIForm) (SearchResults, error) {
	return search(form.Query), nil
}

// search filters the proverbs by case-insensitive substring and splits each
// hit around its first occurrence so the template can highlight it.
func search(query string) SearchResults {
	results := SearchResults{Query: query, Total: len(goProverbs)}
	needle := strings.ToLower(strings.TrimSpace(query))
	for _, proverb := range goProverbs {
		i := strings.Index(strings.ToLower(proverb), needle)
		if i < 0 {
			continue
		}
		results.Matches = append(results.Matches, Match{
			Prefix: proverb[:i],
			Match:  proverb[i : i+len(needle)],
			Suffix: proverb[i+len(needle):],
			Text:   proverb,
		})
	}
	return results
}

// https://go-proverbs.github.io
var goProverbs = []string{
	"Don't communicate by sharing memory, share memory by communicating.",
	"Concurrency is not parallelism.",
	"Channels orchestrate; mutexes serialize.",
	"The bigger the interface, the weaker the abstraction.",
	"Make the zero value useful.",
	"interface{} says nothing.",
	"Gofmt's style is no one's favorite, yet gofmt is everyone's favorite.",
	"A little copying is better than a little dependency.",
	"Syscall must always be guarded with build tags.",
	"Cgo must always be guarded with build tags.",
	"Cgo is not Go.",
	"With the unsafe package there are no guarantees.",
	"Clear is better than clever.",
	"Reflection is never clear.",
	"Errors are values.",
	"Don't just check errors, handle them gracefully.",
	"Design the architecture, name the components, document the details.",
	"Documentation is for users.",
	"Don't panic.",
}

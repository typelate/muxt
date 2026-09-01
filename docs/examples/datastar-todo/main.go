package main

import (
	"cmp"
	"context"
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
)

func main() {
	mux := http.NewServeMux()
	srv := new(Server)
	TemplateRoutes(mux, srv)
	log.Fatal(http.ListenAndServe(":"+cmp.Or(os.Getenv("PORT"), "8002"), mux))
}

//go:generate go run github.com/typelate/muxt generate --use-receiver-type=Server --output-datastar

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.ParseFS(templateSource, "*.gohtml"))

type Todo struct {
	ID    int
	Title string
	Done  bool
}

// Todos is the snapshot every route renders: the page on load and each
// patch event after a mutation.
type Todos struct {
	Items     []Todo
	Remaining int
	Total     int
}

type TodoForm struct {
	Title string `name:"title"`
}

type Server struct {
	mu     sync.Mutex
	nextID int
	items  []Todo
}

func (s *Server) List(_ context.Context) (Todos, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot(), nil
}

func (s *Server) CreateTodo(_ context.Context, form TodoForm, sseTodos func(Todos) error) {
	s.mu.Lock()
	if title := strings.TrimSpace(form.Title); title != "" {
		s.nextID++
		s.items = append(s.items, Todo{ID: s.nextID, Title: title})
	}
	snapshot := s.snapshot()
	s.mu.Unlock()
	_ = sseTodos(snapshot)
}

func (s *Server) ToggleTodo(_ context.Context, id int, sseTodos func(Todos) error) {
	s.mu.Lock()
	if i := slices.IndexFunc(s.items, func(todo Todo) bool { return todo.ID == id }); i >= 0 {
		s.items[i].Done = !s.items[i].Done
	}
	snapshot := s.snapshot()
	s.mu.Unlock()
	_ = sseTodos(snapshot)
}

func (s *Server) DeleteTodo(_ context.Context, id int, sseTodos func(Todos) error) {
	s.mu.Lock()
	s.items = slices.DeleteFunc(s.items, func(todo Todo) bool { return todo.ID == id })
	snapshot := s.snapshot()
	s.mu.Unlock()
	_ = sseTodos(snapshot)
}

// snapshot copies the list so a patch renders state the mutex no longer guards.
func (s *Server) snapshot() Todos {
	todos := Todos{Items: slices.Clone(s.items), Total: len(s.items)}
	for _, todo := range s.items {
		if !todo.Done {
			todos.Remaining++
		}
	}
	return todos
}

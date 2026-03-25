package main

import (
	"cmp"
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"
)

func main() {
	var (
		dataFile string
		port     string
	)
	flag.StringVar(&dataFile, "data", "todos.json", "path to the JSON file for persisting todos")
	flag.StringVar(&port, "port", cmp.Or(os.Getenv("PORT"), "8000"), "port to listen on")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	srv := new(Server)
	srv.Load(dataFile)

	mux := http.NewServeMux()
	TemplateRoutes(mux, srv)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}
	go func() {
		<-ctx.Done()
		srv.Save(dataFile)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	log.Fatal(server.ListenAndServe())
}

//go:generate go run github.com/typelate/muxt generate --use-receiver-type=Server --output-htmx-helpers

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.ParseFS(templateSource, "*.gohtml"))

type Server struct {
	mu     sync.Mutex
	todos  []Todo
	nextID int
}

type Todo struct {
	ID    int
	Title string
	Done  bool
}

type NewTodo struct {
	Title string `name:"todo"`
}

type TodoFilter struct {
	Filter string `name:"filter"`
}

const (
	FilterActive    = "active"
	FilterCompleted = "completed"
)

type ListInfo struct {
	ItemsLeft    int
	HasCompleted bool
	AllDone      bool
}

type TodoPage struct {
	Todos  []Todo
	Filter string
	ListInfo
}

type TodoChange struct {
	Todo   *Todo
	Filter string
	ListInfo
}

type TodoListChange struct {
	Todos  []Todo
	Filter string
	ListInfo
}

func (s *Server) listInfo() ListInfo {
	info := ListInfo{AllDone: len(s.todos) > 0}
	for _, t := range s.todos {
		if t.Done {
			info.HasCompleted = true
		} else {
			info.ItemsLeft++
			info.AllDone = false
		}
	}
	return info
}

func (s *Server) ListTodos(filter TodoFilter) TodoPage {
	s.mu.Lock()
	defer s.mu.Unlock()
	info := s.listInfo()
	var filtered []Todo
	for _, t := range s.todos {
		switch filter.Filter {
		case FilterActive:
			if !t.Done {
				filtered = append(filtered, t)
			}
		case FilterCompleted:
			if t.Done {
				filtered = append(filtered, t)
			}
		default:
			filtered = append(filtered, t)
		}
	}
	return TodoPage{
		Todos:    filtered,
		Filter:   filter.Filter,
		ListInfo: info,
	}
}

func (s *Server) CreateTodo(form NewTodo) TodoChange {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	todo := Todo{ID: s.nextID, Title: form.Title, Done: false}
	s.todos = append(s.todos, todo)
	return TodoChange{Todo: &todo, ListInfo: s.listInfo()}
}

func (s *Server) ToggleTodo(id int) (TodoChange, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.todos {
		if s.todos[i].ID == id {
			s.todos[i].Done = !s.todos[i].Done
			return TodoChange{Todo: &s.todos[i], ListInfo: s.listInfo()}, nil
		}
	}
	return TodoChange{}, fmt.Errorf("todo %d not found", id)
}

func (s *Server) DeleteTodo(id int) TodoChange {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.todos {
		if s.todos[i].ID == id {
			s.todos = append(s.todos[:i], s.todos[i+1:]...)
			break
		}
	}
	return TodoChange{ListInfo: s.listInfo()}
}

func (s *Server) ToggleAll() TodoListChange {
	s.mu.Lock()
	defer s.mu.Unlock()
	info := s.listInfo()
	for i := range s.todos {
		s.todos[i].Done = !info.AllDone
	}
	todos := make([]Todo, len(s.todos))
	copy(todos, s.todos)
	return TodoListChange{Todos: todos, ListInfo: s.listInfo()}
}

func (s *Server) ClearCompleted() TodoListChange {
	s.mu.Lock()
	defer s.mu.Unlock()
	var remaining []Todo
	for _, t := range s.todos {
		if !t.Done {
			remaining = append(remaining, t)
		}
	}
	s.todos = remaining
	todos := make([]Todo, len(s.todos))
	copy(todos, s.todos)
	return TodoListChange{Todos: todos, ListInfo: s.listInfo()}
}

func (s *Server) Save(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(s.todos, "", "\t")
	if err != nil {
		log.Printf("failed to marshal todos: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("failed to save todos: %v", err)
	}
}

func (s *Server) Load(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var todos []Todo
	if err := json.Unmarshal(data, &todos); err != nil {
		log.Printf("failed to load todos: %v", err)
		return
	}
	s.todos = todos
	for _, t := range s.todos {
		if t.ID >= s.nextID {
			s.nextID = t.ID
		}
	}
}

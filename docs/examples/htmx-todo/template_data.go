package main

// TodoItem is the dot of the todo-item template: one todo and the route paths
// its controls request.
type TodoItem struct {
	Todo
	Path TemplateRoutePaths
}

// Item pairs todo with the route paths, so the todo-item template can build its
// hx-patch and hx-delete URLs with .Path instead of writing them by hand.
func (data *TemplateData[R, T]) Item(todo Todo) TodoItem {
	return TodoItem{Todo: todo, Path: data.Path()}
}

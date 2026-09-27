# Motivation

The [manifesto](manifesto.md) states the principles.

## The problem

I want to ship features without writing boilerplate or learning a frontend framework. Years in regulated environments, where every dependency update triggers a compliance review, taught me to take fewer dependencies.

## htmx

[htmx](https://htmx.org/) showed that a dynamic interface does not need a frontend framework: attributes on the HTML supply the interactivity.

## The server side was still boilerplate

Every route looked like this:

```go
func handleGetArticle(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    article, err := getArticle(r.Context(), id)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    tmpl.ExecuteTemplate(w, "article.html", article)
}
```

Fifty routes means fifty near-copies. [sqlc](https://sqlc.dev) showed the pattern for SQL; muxt declares the route in the template name and generates the handler.

## Why not reflection

The first version was a [reflection-based handler](https://github.com/typelate/muxt/blob/33f2eb69d84d6bf2c2ad87c5ddfee9fb2e0fea31/handler.go). [Reflection is never clear](https://youtu.be/PAAkCSZUG1c?si=gT_ga16SMOKNshqp&t=922), and debugging it meant holding the runtime's behavior in your head. [Decision 2](decisions/00002_use_code_generation_instead_of_reflection.md) records the switch.

## Why not an LLM

An LLM writes one handler well enough. Across a team, each person prompts differently and fifty handlers drift. A generator makes them identical, and improving it improves all of them.

## The stack

In production: Go, htmx and `html/template`. In most projects: sqlc for queries, muxt for routes and [counterfeiter](https://github.com/maxbrunsfeld/counterfeiter) for test doubles.

# The Muxt Manifesto

The principles that decide what muxt does and does not do. [Motivation](motivation.md) covers where they came from.

## Impose no dependencies on users

Generated code imports only the standard library, plus the types your methods take and return. A dependency costs something on every upgrade.

## Generate simple, readable code

Generated code uses meaningful identifiers and inlines its logic. You can read `template_routes.go`, step through it, and edit it into a hand-written handler ([decision 2](decisions/00002_use_code_generation_instead_of_reflection.md)).

## Reduce package pollution

Generated code lives in the same package as your templates and helpers. When a generated identifier collides with one of yours, rename either; the `--output-*` flags ([CLI](../reference/cli.md)) rename the generated one.

## Collaboration, not isolation

The model is templates plus Go methods. Anyone on the team can review and refactor either, so nobody has to be the person who understands the framework.

## Enable ruthless refactoring

Rename a result field and `muxt check` names the templates that still use it. Change a parameter list and `muxt generate` reports the template name whose call no longer matches. Edit the template and refresh ([hot reload with Air](../tutorials/hot-reload-with-air.md)).

## Keep the complexity budget small

Rendering `<div>Hello, {{.Name}}</div>` should not require a build pipeline or a frontend framework. Templates map to routes, routes call methods, methods return data. The budget goes to the domain.

## Locality of behavior

htmx's principle is that reading the HTML tells you what happens, without jumping between files. Muxt extends it to the route. The template name states the HTTP method, the path, the parameters and the Go method:

`GET /article/{id} GetArticle(ctx, id)`

## HTML is a fine interface

Browsers render HTML and the server holds the state. Sending HTML removes client-side routing, JSON marshalling and state synchronization. [HTML is the API](html-is-the-api.md) covers testing that contract.

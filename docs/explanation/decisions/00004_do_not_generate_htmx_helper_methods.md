# 4 - Do not Generate HTMX Helper Methods on TemplateData

## Context

Generated `TemplateData` methods let template actions reach the receiver, the request and redirects. I did not know what the htmx method signatures should be, or what calling them from templates would do to template maintainability.

## Decision

Do not generate htmx helper methods on `TemplateData`. Document copyable helper methods to add to a package by hand.

## Status

Superseded by `--output-htmx` (`--output-htmx-helpers` is a deprecated alias).

## Consequences

Once I learn how templates should use htmx headers, I might add a `--htmx` flag that adds the documented `htmx*.go` files to the target package.

## Update

`--output-htmx` ([CLI](../../reference/cli.md)) adds `HX*` methods to `TemplateData`; the [htmx-counter example](../../examples/htmx-counter/template_routes.go) has the full list.

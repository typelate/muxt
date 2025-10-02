# 4 - Do not Generate HTMX Helper Methods on TemplateData

## Context

Generated `TemplateData` methods now include helpers to interact with the Receiver, Request, and Redirect
from template actions.
I exclusively use Muxt with HTMX (although it can work well with standard Web 2.0 Hypermedia templates).
I am not sure what the method signatures for HTMX should be or what the implication of having those
methods called in templates is on long term template maintainability.

## Decision

Do not Generate HTMX Helper Methods on TemplateData; document (copyable) helper methods to add to packages manually.  

## Status

Decided

## Consequences

Once I learn about how to properly interact with HTMX headers from templates, I might add a `--htmx` flag to add the
existing documented `htmx*.go` files to the target package.

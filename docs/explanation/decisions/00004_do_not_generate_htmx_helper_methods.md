# 4 - Do not Generate HTMX Helper Methods on TemplateData

## Context

Generated `TemplateData` methods now include helpers to interact with the Receiver, Request, and Redirect
from template actions.
I exclusively use Muxt with HTMX (although it can work well with fixi).
I am not sure what the method signatures for HTMX should be or what the implication of having those
methods called in templates is on long term template maintainability.

## Decision

Do not Generate HTMX Helper Methods on TemplateData; document (copyable) helper methods to add to packages manually.  

## Status

Superseded. Muxt now generates HTMX helper methods, opt-in via the `--output-htmx-helpers` flag.

## Consequences

*(Written before the flag shipped — see Update below.)*

Once I learn about how to properly interact with HTMX headers from templates, I might add a `--htmx` flag to add the
existing documented `htmx*.go` files to the target package.

## Update

The flag shipped as `--output-htmx-helpers` (not `--htmx`). When set, generation adds `HX*` helper methods to
`TemplateData` for writing response headers (`HX-Location`, `HX-Trigger`, etc.) and reading request headers
(`HX-Request`, `HX-Boosted`, etc.). It is off by default, so the original decision still holds unless you opt in.
See [reference_htmx_helpers.txt](../../../cmd/muxt/testdata/reference_htmx_helpers.txt).

---
name: muxt-htmx
description: "Muxt: Use when exploring, developing, or testing HTMX interactions in a Muxt codebase. Covers finding hx-* attributes, using --output-htmx-helpers, testing fragment chains, and verifying inter-route coupling. Distinct from muxt_datastar (data-* attributes / Datastar SSE) and muxt_forms (standard HTML form submission)."
---

# HTMX with Muxt

Develop and test HTMX interactions in a Muxt codebase: discovery, generated helpers, fragment chains, inline validation.

## When to use this skill

- Templates contain `hx-*` attributes (`hx-get`, `hx-post`, `hx-swap`, `hx-target`, `hx-trigger`, `hx-confirm`, `hx-boost`).
- Building features with HTMX-driven partial swaps or inline validation.
- Writing fragment-chain tests for inter-route coupling.

## Discovering HTMX in a codebase

```bash
grep -rn 'hx-get\|hx-post\|hx-put\|hx-patch\|hx-delete' --include='*.gohtml' .
grep -rn 'hx-target\|hx-swap\|hx-select'                --include='*.gohtml' .
grep -rn 'hx-trigger\|hx-confirm\|hx-boost'             --include='*.gohtml' .
```

For each `hx-get="/some/path"`:
1. Find the route handling it: `muxt list-template-calls --match "/some/path"`.
2. Read that template — what fragment is returned?
3. Check the triggering element's `hx-target`/`hx-swap` for where the fragment lands.
4. Check if the route uses `.HXRequest` to branch fragment vs full page.

See [HTMX attributes reference](https://htmx.org/reference/#attributes).

## Generated helpers (`--output-htmx-helpers`)

Enable the flag to expose helper methods on `TemplateData`:

- **Response header helpers** — `{{.HXRedirect "/path"}}`, `{{.HXTrigger "event"}}`, `{{.HXRetarget "#id"}}`, etc. Set HTMX-specific response headers from templates.
- **Request header readers** — `{{.HXRequest}}`, `{{.HXBoosted}}`, `{{.HXPrompt}}`, etc. Branch template output based on what the client sent.

Full helper tables and the progressive-enhancement pattern (`.HXRequest` to return fragments for HTMX, full pages for direct nav): see `references/examples.md`.

## Template fragments — locality of behaviour

Go's `html/template` provides fragments natively. Every `{{define}}...{{end}}` is a fragment renderable with `{{template "name" .}}`. Muxt route templates are themselves fragments. No special syntax is needed — see [Template Fragments essay](https://htmx.org/essays/template-fragments/).

## Testing fragment chains

HTMX interactions form chains: page A's `hx-get` points at route B, whose response swaps into a target element on A. Tests must verify the inter-route coupling stays consistent.

**Do NOT use Given/When/Then table-driven tests for fragment chains.** Use a single test that exercises the request sequence and asserts the chain (page → fragment → submit → response). Full pattern with `domtest.ParseResponseDocumentFragment` and `HX-Request` headers in `references/examples.md`.

For exhaustive coverage, also assert that **all** `hx-*` attributes in the rendered page are accounted for in tests — see the `TestAllHTMXEndpointsAreTested` pattern in `references/examples.md`. New `hx-get` in a template fails this test, forcing a corresponding fragment chain test.

When templates set HTMX response headers, assert them via `rec.Header().Get("HX-Trigger")`, etc.

## Islands with chromedp

For HTMX-loaded islands (content swapped after initial render), `domtest` cannot verify (no JS execution). Use chromedp behind a build tag (`//go:build chromedp`) so `go test` stays fast by default. Navigate to the *parent* page (which loads htmx.js), not the fragment URL directly. Full example in `references/examples.md`.

## Inline field validation

Per-field validation as the user types/blurs. `hx-post` on an input's wrapping `<div>`, `hx-target="this"`, `hx-swap="outerHTML"`. The validation endpoint returns the same container with error or success styling. Default trigger is `change`; use `hx-trigger="blur"` or `keyup delay:500ms`. Full pattern, template + handler examples in `references/examples.md`.

For standard form validation without HTMX, see [`muxt_forms`](../muxt_forms/SKILL.md#re-rendering-after-validation-errors).

## Status codes and HTMX

HTMX swaps only on 2xx by default. Non-2xx silent unless you use the [response-targets extension](https://htmx.org/extensions/response-targets/) (`hx-target-404`, `hx-target-5*`). Alternatives: return 200 with error content in body, or use `HX-Reswap`/`HX-Retarget` headers. See `references/examples.md` for the pattern.

## Reference files

- `references/examples.md` — full helper tables, progressive enhancement, fragment-chain test, exhaustive coupling test, response-header testing, chromedp islands, inline validation pattern, status-code/extension setup.

## External reference

- [`muxt generate` flags](../../reference/commands/generate.md) — `--output-htmx-helpers`
- [HTMX example](../../examples/counter-htmx/) — counter app with helpers
- [Template Name Syntax](../../reference/template-names.md)
- [chromedp](https://github.com/chromedp/chromedp) — headless Chrome for islands

### Test cases (`cmd/muxt/testdata/`)

- `reference_htmx_helpers.txt` — `--output-htmx-helpers`, HX-Request detection, response-header assertions
- `howto_form_basic.txt` — form binding (relevant to HTMX POST patterns)
- `reference_status_codes.txt` — relevant to HTMX response handling

### HTMX docs

[Documentation](https://htmx.org/docs/) · [Attributes](https://htmx.org/reference/#attributes) · [Request headers](https://htmx.org/reference/#request_headers) · [Response headers](https://htmx.org/reference/#response_headers) · [Examples](https://htmx.org/examples/) · [Essays](https://htmx.org/essays/)

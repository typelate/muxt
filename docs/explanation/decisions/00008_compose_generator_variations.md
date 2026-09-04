# 8 - Compose Generator Variations with Dispatch Values

## Context

The generate package grew two axes of variation. The frontend flags
(`--output-htmx`, `--output-datastar`) shape the generated template data
types — helper methods, setters, and the SSE framing `WriteTo` writes. The
wire flags ([decision 7](00007_wire_sse_libraries.md)) select which
implementation writes the SSE protocol, replacing stream setup and frame
emission. The axes compose: a datastar package can wire through an external
SSE implementation.

Variation is currently expressed as configuration conditionals whose branch
bodies are whole alternative emissions — the SSE closure builder carries one
body per wire implementation, the SSE template data builder picks between
decl lists per frontend. Each new variant adds an arm to every one of these
sites instead of adding a file. Around the branches, the assembly file for
routes has grown past 1,600 lines of mixed concerns, and the astgen package
is thin enough that generate hand-rolls the same statement shapes (error
checks, define-and-assign, deferred calls) at dozens of sites, inflating
every branch body.

Forking github.com/dave/jennifer for a fluent code-generation API was
considered. Jennifer renders text with its own import manager, while this
pipeline is go/ast end to end — type-aware via go/types, printed by
go/printer, and pinned byte-exact by the txtar goldens. Swapping renderers
risks output drift for ergonomics that a richer astgen can provide over
go/ast directly.

## Decision

Express each variation axis as a dispatch value — a struct of emission
functions populated once from the generate configuration — with one file per
variant inside the generate package. Grow astgen with the composable
builders generate keeps hand-rolling, keeping go/ast as the intermediate
representation. Move request-semantics the generator re-derives (which
request source feeds each argument, callback contracts) into the muxt
package so generate maps resolved definitions to syntax.

Land the change as a series of small refactors, each leaving every test
outside the generate and muxt packages untouched and the generated output
byte-identical:

1. astgen builders for the recurring statement shapes, swapped in one
   generate file at a time.
2. The SSE wire variants extracted into a wire dispatch value, one file per
   implementation.
3. The frontend decl variants extracted into a frontend dispatch value.
4. Argument-source semantics moved onto the resolved argument type in the
   muxt package.
5. The routes assembly file split by concern — file moves without signature
   churn.
6. Only if the seams prove clean: promote the dispatch variants to
   sub-packages under the generate package.

## Status

Decided

## Consequences

- Adding a wire or frontend variant becomes adding a file that populates the
  dispatch value, not threading a new arm through shared builders.
- Dispatch values are plain structs of functions: no interface ceremony now,
  and promoting an axis to an interface or sub-package later is mechanical.
- The txtar goldens are the refactoring harness — byte-identical generated
  output is the definition of done for every step.
- astgen stays a go/ast builder library; if its API matures into a general
  fluent layer it can graduate to its own module on its own merits, which is
  the fork-insurance jennifer would have provided.
- The muxt package accumulates the semantic vocabulary (argument sources,
  callback contracts), narrowing generate toward pure syntax emission.

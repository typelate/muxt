# 2 - Use Generation Instead of Reflection

## Context

I am superstitious about the potential performance costs incurred by a handler fully using reflection.
I don't like how the entrypoint to a web-service is convoluted by reflection.
I want a simple code file that I can easily read.

## Decision

Re-write the package to generate a handler instead of using reflection.

## Status

Decided

## Consequences

I won't be able to use [jba/templatecheck](https://github.com/jba/templatecheck),
so I will need to write my own template checker to get the safety of pre-execution template validation.

While reflection is not clear, code generation code can be much more convoluted so muxt will get much more difficult to iterate on.

Testing `muxt` requires tests that run `go test` or `go build` and cannot just test an `http.Handler`. 
# 2 - Use Generation Instead of Reflection

## Context

I am superstitious about the performance cost of reflection, and I dislike a convoluted entry point to a web service. I want one code file I can read.

## Decision

Rewrite the package to generate a handler instead of using reflection.

## Status

Decided

## Consequences

[jba/templatecheck](https://github.com/jba/templatecheck) no longer fits, so muxt needs its own template checker.

Code that generates code can be harder to read than reflection, so muxt itself becomes harder to iterate on.

Testing muxt requires running `go test` or `go build` on generated output; testing an `http.Handler` directly is not enough.

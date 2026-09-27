# 3 - Use Single File Tests Based on rsc/script

## Context

The unit tests were coupled to the implementation, so every change to code generation meant updating all of them. [Wrapping the result data in a struct](https://github.com/typelate/muxt/commit/9306e6d4b37e343d4c84f3d70e04025c77e4c0db) is a small generation change that forced a huge test change.

## Decision

Migrate all code generation unit tests to command-level tests.

## Status

Decided

## Consequences

Tests may take longer to run, so they need to run in parallel.

Changing generated signatures and identifiers gets cheaper, so code importing them can break without notice.

## References

The migration: https://github.com/typelate/muxt/compare/v0.12.0...v0.13.0

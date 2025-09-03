# 3 - Use Single File Tests Based on rsc/script

## Context

It was getting really expensive to maintain both the unit and high level code generation tests;
The tests are coupled tightly to the implementation.
Whenever I changed code generation, I would need to update all the tests.     
[This change where I wrapped the result data in a struct is a good example of a huge test change required by a smaller generation change.](https://github.com/typelate/muxt/commit/9306e6d4b37e343d4c84f3d70e04025c77e4c0db).

## Decision

Migrate all code generation unit tests to command level tests.

## Status

Decided

## Consequences

Tests may take longer to run.
Unit tests need to be fast, and care needs to be taken to make sure they can run in parallel.

There will be much less friction in changing signatures/identifiers of generated code.
This could break importer code.

## References

The relevant changes were made here: https://github.com/typelate/muxt/compare/v0.12.0...v0.13.0
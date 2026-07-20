# 5 - Rename Flags

## Context

I had not really considered the design around flag names.
I noticed how Claude grouped the flags in a generated document.
I thought I could express that division so that the purpose of the flags was more intuitive.

## Decision

Prefix flags that identify source queries with `--use-`
Prefix flags that specify generated identifiers with `--output-`

## Status

Decided

## Consequences

I need to maintain deprecated flags for a while.

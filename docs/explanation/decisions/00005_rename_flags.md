# 5 - Rename Flags

## Context

I had not really considered the design around flag names.
I noticed in how Claude separated the flags in groups in a generated document file.
I thought I could express that division so that the purpose of the flags was more intuitive.

## Decision

Prefix flags that identify source queries with `--use-`
Prefix flags that specify generated identifiers `--output-`

## Status

Decided

## Consequences

I need to maintain deprecated flags for a while.

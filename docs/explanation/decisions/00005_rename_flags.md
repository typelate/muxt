# 5 - Rename Flags

## Context

I had not designed the flag names. Grouping them by purpose was worth expressing in the names.

## Decision

Prefix flags that name what muxt reads with `--use-` (`--use-receiver-type`, `--use-templates-variable`). Prefix flags that name what it writes with `--output-` (`--output-file`, `--output-htmx`).

## Status

Decided

## Consequences

The old names stay as deprecated aliases ([CLI](../../reference/cli.md#deprecated-flags)).

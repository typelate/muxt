---
name: muxt-maintain-tools
description: "Muxt: Use when installing, updating, or upgrading the tools used in a Muxt codebase — muxt itself, gofumpt, counterfeiter, domtest, testify, sqlc, txtar, chromedp."
---

# Maintain muxt-related tools

Install and update the tools used in a Muxt development workflow. Quick-reference list — copy/paste the install commands you need.

## Required Tools

### muxt

The code generator and type checker:

```bash
go install github.com/typelate/muxt/cmd/muxt@latest
```

Verify:

```bash
muxt version
```

### gofumpt

Stricter Go formatter (superset of `gofmt`):

```bash
go install mvdan.cc/gofumpt@latest
```

Format after code changes:

```bash
gofumpt -w .
```

## Test Tools

### counterfeiter

Generate test doubles from interfaces:

```bash
go install github.com/maxbrunsfeld/counterfeiter/v6@latest
```

Usage in a Muxt project:

```go
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate . RoutesReceiver
```

```bash
go generate ./...
```

### domtest

HTML assertion library for testing generated handlers:

```bash
go get github.com/typelate/dom/domtest
```

### testify

Assertion helpers:

```bash
go get github.com/stretchr/testify/{assert,require}
```

## Optional Tools

### sqlc

Type-safe SQL code generation (for database-backed projects):

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

### txtar

Extract txtar archives (used by skill scaffolds):

```bash
go install golang.org/x/tools/cmd/txtar@latest
```

### chromedp

Headless Chrome testing (for HTMX island tests):

```bash
go get github.com/chromedp/chromedp
```

## Updating All Tools

```bash
go install github.com/typelate/muxt/cmd/muxt@latest
go install mvdan.cc/gofumpt@latest
go install github.com/maxbrunsfeld/counterfeiter/v6@latest
go install golang.org/x/tools/cmd/txtar@latest
```

## Reference

- [CLI Overview](../../reference/cli.md)
- [muxt generate](../../reference/commands/generate.md)
- [muxt check](../../reference/commands/check.md)

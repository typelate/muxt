# muxt version

Print the muxt version number. Use `-v` for verbose output including the Go version used to compile muxt.

**Aliases:** `v`

```bash
muxt version
muxt version -v  # Shows Go version used to compile muxt
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-v, --verbose` | bool | `false` | Show Go version used to compile muxt. |

## Example Output

```
v0.17.0
```

With `--verbose`:

```
v0.17.0
go version: go1.25.0
```

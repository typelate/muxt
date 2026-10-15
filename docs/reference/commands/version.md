# muxt version

Prints the binary's module version: `(devel)` under `go run`, the VCS pseudo-version after `go build` or `go install` from a checkout.

```bash
muxt version -v
```

```text
v0.21.0-dev.2
go version: go1.27.0
```

`-v` adds the Go version muxt was compiled with.

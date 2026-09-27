# muxt version

Prints the binary's module version: `(devel)` under `go run`, the VCS pseudo-version after `go build` or `go install` from a checkout.

```bash
muxt version -v
```

```text
v0.17.0
go version: go1.25.0
```

`-v` adds the Go version muxt was compiled with.

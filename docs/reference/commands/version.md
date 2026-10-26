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

`muxt generate` records this version in generated files only when it is a tagged version; an untagged build keeps the version already in the file. See [Version header](generate.md#version-header).

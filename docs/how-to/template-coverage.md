# See which template blocks your tests run

1. In the package directory, run `muxt test`.
2. Write a profile: `muxt test --output-cover-profile cover.out`.
3. Open it: `go tool cover -html=cover.out`.
4. In a directory with a [muxt.json](../reference/muxt-json.md), `muxt test` runs every entry in turn and writes each key on its own line before that package's output.
5. When a test fails only under `muxt test`, run `go test` and read the differences: the engine [escapes script and URL attributes differently](../reference/commands/test.md#differences-from-go-test).

# muxt generate-fake-server

Writes a `main.go` that serves a package's routes from a counterfeiter fake of its receiver, so the pages can be browsed before the receiver exists. The fake's interface is unstable; do not depend on it.

```bash
muxt generate-fake-server ./hypertext
```

Arguments are package directories; none means the working directory. Each must be a library package with a generated routes file ([err_generate_fake_server_main_package.txt](../../../cmd/muxt/testdata/err_generate_fake_server_main_package.txt)). `-o` sets the output directory, default `./cmd/explore-goland`.

| File | Contents |
|---|---|
| `main.go` | Builds the fake, registers the routes, starts an `httptest` server, prints `Explore at: http://127.0.0.1:PORT`, and waits for Ctrl-C. |
| `internal/fake/receiver.go` | The counterfeiter fake, with a `Returns` and `CallCount` method per receiver method. |

Every method returns zero values until `main.go` gets a `receiver.<Method>Returns(...)` call before the routes function:

```go
receiver.ListArticlesReturns([]hypertext.Article{{ID: 1, Title: "First Post"}}, nil)
```

```bash
go run ./cmd/explore-goland
```

Script: [reference_generate_fake_server.txt](../../../cmd/muxt/testdata/reference_generate_fake_server.txt). [`explore-module`](explore-module.md) lists the packages this command can target.

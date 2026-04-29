# Interactive exploration via fake server + Chrome DevTools MCP

Generate an httptest exploration server with a counterfeiter fake so you can browse the running app with realistic data. Useful when reading templates and trying to picture what they render is too indirect.

## Generate

```bash
muxt generate-fake-server path/to/package
```

Files in `./cmd/explore-goland/`:

- **`main.go`** — readable entry point: creates the fake receiver, wires routes, starts httptest server.
- **`internal/fake/receiver.go`** — counterfeiter-generated fake struct with `*Returns()` and `*CallCount()` methods.

Override the output dir:

```bash
muxt generate-fake-server path/to/package --output ./cmd/my-explorer
```

## Set up fake return values

Edit `main.go` before the server starts. The fake has `*Returns(...)` per interface method:

```go
receiver := new(fake.RoutesReceiver)

// Set up fake data so routes render with realistic content
receiver.ListArticlesReturns([]hypertext.Article{
    {ID: 1, Title: "First Post", Body: "Hello world"},
    {ID: 2, Title: "Second Post", Body: "Another article"},
}, nil)
receiver.GetArticleReturns(hypertext.Article{
    ID: 1, Title: "First Post", Body: "Hello world",
}, nil)
```

Modify return values and rerun `go run ./cmd/explore-goland/` to test different states (empty lists, errors, edge cases).

## Run the server

```bash
go run ./cmd/explore-goland/
```

Prints `Explore at: http://127.0.0.1:<port>` and waits for Ctrl-C.

## Browse with Chrome DevTools MCP

```
navigate_page({"url": "http://127.0.0.1:<port>/"})
take_snapshot({})
take_screenshot({})
```

Click links, inspect the DOM, screenshot the rendered page. For HTMX-enabled pages, interact and observe the network:

```
click({"uid": "<element-uid>"})
take_snapshot({})
list_network_requests({})
```

Chrome DevTools MCP runs a real browser, so HTMX swaps, JavaScript, and CSS all work. Workflow:

1. **Navigate** + take snapshot for the rendered DOM.
2. **Click** interactive elements (buttons, links, HTMX triggers); snapshot again to see what changed.
3. **Check network requests** for HTMX fragment fetches and form submissions.
4. **Take screenshots** for the visual result with CSS applied.

For HTMX pages this is the fastest way to verify `hx-get`, `hx-post`, and swap targets work correctly with the fake data.

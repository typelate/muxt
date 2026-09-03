# Hot Reload with Air

Rebuild and restart your HTTP server automatically when a template or Go file changes, using [Air](https://github.com/air-verse/air).

Muxt does not have its own hot-reload mode. Templates are embedded with `go:embed` and routes are generated from template names, so an edit to a `.gohtml` file requires three steps: regenerate `template_routes.go`, rebuild the binary, restart the server. Air already orchestrates exactly that loop — a `pre_cmd` for regeneration, then build and restart — so a small config file does the whole job.

## Prerequisites

- A working Muxt project (one where `muxt generate` already runs cleanly)
- A `go:generate` directive that runs Muxt, for example:

```go
//go:generate muxt generate --use-receiver-type=Server
```

## Step 1: Install Air

Follow Air's [installation instructions](https://github.com/air-verse/air#installation) and make sure `air` is on your `PATH`.

## Step 2: Create .air.toml

Run `air init` for a starting point, or write this directly:

```toml
root = "."
tmp_dir = "tmp"

[build]
  pre_cmd = ["go generate ./..."]
  cmd = "go build -o ./tmp/app ."
  bin = "./tmp/app"
  include_ext = ["go", "gohtml", "css", "js"]
  exclude_file = ["template_routes.go"]
  exclude_regex = ["_test\\.go$"]
  exclude_dir = ["tmp"]

[screen]
  clear_on_rebuild = true
```

Adjust `cmd` if your server's main package lives in a subdirectory (for example `go build -o ./tmp/app ./cmd/server`).

Two lines are particularly important:

- `pre_cmd = ["go generate ./..."]` runs `muxt generate` before every build, so route changes in template names take effect on save.
- `exclude_file = ["template_routes.go"]` keeps Air from watching the file `muxt generate` writes. Without it, every regeneration triggers another rebuild — an endless loop. If you set `--output-file` to a different name, exclude that name instead.

`go generate ./...` runs every directive in the module, which can make the reload loop slow — generators like [counterfeiter](https://github.com/maxbrunsfeld/counterfeiter) fakes add up fast. Narrow the `pre_cmd` to the package that holds your templates, and use `go generate`'s directive filters to run only what the loop needs:

```toml
  pre_cmd = ["go generate -run muxt ./internal/hypertext"]
```

`-run muxt` matches only the Muxt directives; `-skip counterfeiter` is the inverse when you would rather name what to leave out.

## Step 3: Run It

```bash
air
```

Edit a `.gohtml` file and save. Air regenerates, rebuilds, restarts, and the next browser refresh serves the new markup. Because the rebuild re-embeds the templates, this works without any dev-mode file reading in your server code.

## When Generation Fails

A bad template name stops the loop at `pre_cmd`, and the Muxt error appears in Air's output with the template file to open:

```text
Error: /path/to/index.gohtml: OPTIONS method not allowed
```

Fix the template and save again; Air picks up where it left off. Adding `muxt check` to your generate directives catches template type errors in the same pass.

## Excluding Multiple Generated Files

With `muxt generate --output-multiple-files`, regeneration writes `template_routes.go` plus one `*_template_routes_gen.go` file per template source file. Every one of them can retrigger the watcher, so exclude the whole family by pattern instead of listing names:

```toml
[build]
  exclude_regex = ["_test\\.go$", "_template_routes_gen\\.go$"]
```

Keep the `exclude_file = ["template_routes.go"]` line for the shared file. The same applies to any other generator your `pre_cmd` runs: whatever it writes must be excluded, or the loop never settles.

## Live Browser Reload

Air restarts the server, but the browser still needs a refresh. Air ships an optional [proxy](https://github.com/air-verse/air#include-proxy-for-browser-auto-reload) that injects a reload script:

```toml
[proxy]
  enabled = true
  proxy_port = 8090
  app_port = 8080
```

Browse to the proxy port and pages reload themselves after each rebuild.

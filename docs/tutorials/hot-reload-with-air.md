# Hot Reload with Air

After you save a `.gohtml` file, `muxt generate`, a build, and a restart must run before the page changes. [Air](https://github.com/air-verse/air) runs that loop from one config file.

Prerequisites: [Air](https://github.com/air-verse/air#installation) and the project from the [Quick Start](quick-start.md), through step 4. Its `go:generate` directive runs `muxt generate`.

## Step 1: Create .air.toml

```toml
root = "."
tmp_dir = "tmp"

[build]
  pre_cmd = ["go generate ./..."]
  cmd = "go build -o ./tmp/app ."
  entrypoint = ["./tmp/app"]
  include_ext = ["go", "gohtml", "css", "js"]
  exclude_regex = ["_test\\.go$", "template_routes(_gen)?\\.go$"]
  exclude_dir = ["tmp"]

[screen]
  clear_on_rebuild = true
```

`pre_cmd` regenerates before every build. `entrypoint` replaces the deprecated `bin` key. `exclude_regex` keeps the generated file from triggering a second rebuild; it matches `template_routes.go` in any package, the `<name>_template_routes_gen.go` files of `--output-multiple-files`, and any `--output-file` name ending in `template_routes.go`.

## Step 2: Run

```bash
air
```

Edit and save `index.gohtml`. Air regenerates, rebuilds, and restarts, and the next refresh serves the new markup. A bad template name fails in `pre_cmd`, and the muxt error names the file.

## Step 3: Reload the browser

Air's [proxy](https://github.com/air-verse/air#proxy-reload-the-browser-automatically) injects a reload script. Append to `.air.toml`:

```toml
[proxy]
  enabled = true
  proxy_port = 8090
  app_port = 8080
```

Stop Air with Ctrl-C and run `air` again. Browse to <http://localhost:8090>, edit `index.gohtml`, and the page reloads on save.

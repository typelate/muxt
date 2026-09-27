# Hot Reload with Air

A saved `.gohtml` needs `muxt generate`, a build, and a restart. [Air](https://github.com/air-verse/air) runs that loop from one config file.

Prerequisites: [Air](https://github.com/air-verse/air#installation) and a `go:generate` directive that runs `muxt generate`.

## Step 1: Create .air.toml

```toml
root = "."
tmp_dir = "tmp"

[build]
  pre_cmd = ["go generate ./..."]
  cmd = "go build -o ./tmp/app ."
  bin = "./tmp/app"
  include_ext = ["go", "gohtml", "css", "js"]
  exclude_regex = ["_test\\.go$", "template_routes(_gen)?\\.go$"]
  exclude_dir = ["tmp"]

[screen]
  clear_on_rebuild = true
```

`pre_cmd` regenerates before every build. `exclude_regex` stops the generated files from triggering a second rebuild. It matches `template_routes.go` in any package, the `<name>_template_routes_gen.go` files of `--output-multiple-files`, and any `--output-file` name ending in `template_routes.go`.

## Step 2: Run

```bash
air
```

Save a `.gohtml` file. Air regenerates, rebuilds, restarts, and the next refresh serves the new markup. A bad template name fails in `pre_cmd` and the muxt error names the file.

## Step 3 (optional): Browser reload

Air's [proxy](https://github.com/air-verse/air#proxy-reload-the-browser-automatically) injects a reload script:

```toml
[proxy]
  enabled = true
  proxy_port = 8090
  app_port = 8080
```

Browse to the proxy port.

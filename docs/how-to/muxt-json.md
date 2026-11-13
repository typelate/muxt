# How to move generate flags into muxt.json

This page is for a module with several muxt packages. Putting flags on a `//go:generate muxt generate --some-flags` line is the normal way to configure a package and needs no file; a module with one or two packages should keep doing that. `muxt.json` is optional. It collects the flags of many packages in one place.

We assume a module `example.com/m` with two packages, each with a `templates` variable and a `go:generate` line with flags:

- `example.com/m` (the root), receiver type `Server`, imports `example.com/m/workspace`.
- `example.com/m/workspace`, receiver type `T`.

The root's line reads:

```go
//go:generate muxt generate --use-receiver-type=Server --output-htmx
```

and the workspace's reads `//go:generate muxt generate --use-receiver-type=T --output-htmx`.

## 1. Create the file

In the directory that holds `go.mod`:

```bash
muxt generate config
```

```text
wrote muxt.json
```

The file starts as `{"packages": {}}`. The command refuses to overwrite one: `/home/me/app/muxt.json already configures this directory; add a package with: muxt generate package NAME`.

## 2. Write an entry for each package

Both flags lines become entries ([keys](../reference/muxt-json.md)):

```json
{
	"packages": {
		"example.com/m": {
			"use_receiver_type": "Server",
			"output_htmx": true
		},
		"example.com/m/workspace": {
			"use_receiver_type": "T",
			"output_htmx": true
		}
	}
}
```

## 3. Move shared settings to package_defaults

`output_htmx` appears twice. Put it once under `package_defaults`; an entry's own key wins over it.

```json
{
	"package_defaults": {
		"output_htmx": true
	},
	"packages": {
		"example.com/m": {
			"use_receiver_type": "Server"
		},
		"example.com/m/workspace": {
			"use_receiver_type": "T"
		}
	}
}
```

## 4. Keep one go:generate

Delete the `//go:generate` line from `workspace`. Every package whose line you delete needs an entry: [a package with no entry is skipped](../reference/muxt-json.md). In the root package, where `muxt.json` sits, the line becomes:

```go
//go:generate muxt generate
```

The package load is the expensive part of `muxt generate`; one run generates every entry and pays for it once.

If the directory holding `muxt.json` and `go.mod` has no Go package, put the one directive in any package, with `-C` naming the directory that holds `muxt.json`: `//go:generate muxt -C .. generate` in `workspace`.

## 5. Generate

```bash
go generate ./...
```

It prints nothing unless a route has a note or warning; `-v` lists what was written. `example.com/m` imports `example.com/m/workspace`, so the workspace is written first.

`muxt check` in the same directory checks every entry, the workspace first.

Run `muxt generate` inside `workspace` and it writes that entry only. A directive left in a sub-package generates that package a second time.

## 6. What a flag does now

In a directory with an entry, a flag is rejected and nothing is written:

```bash
cd workspace
muxt generate --output-htmx
```

```text
Error: configure generate with fields in /home/me/app/muxt.json instead of flags: --output-htmx (output_htmx)
```

Change the entry. `-v` and `-C` are still accepted.

## Next

- Keys, the resolution order and errors: [muxt.json reference](../reference/muxt-json.md).
- `muxt generate package` and `muxt generate def` scaffold a package and a route; packages and routes are normally written by hand.
- These steps as a test: [howto_muxt_json_from_flags.txt](../../cmd/muxt/testdata/howto_muxt_json_from_flags.txt). The scaffolds: [howto_muxt_json.txt](../../cmd/muxt/testdata/howto_muxt_json.txt).

# muxt.json

`muxt.json` holds the `muxt generate` flags of each package. Each package's settings are an **entry**, the object under its `packages` key. The file is found by walking up from the working directory; the walk stops at the directory holding `go.mod`, and the file needs a `go.mod` at or above it. The nearest file wins, so a hand-written `muxt.json` in a sub-directory replaces the one above it there; `muxt generate config` refuses to write one.

```json
{
	"package_defaults": {"output_htmx": true},
	"packages": {
		"example.com/m/workspace": {"use_receiver_type": "T"}
	}
}
```

- `packages` keys are import paths of packages at or below the file's directory.
- Entry keys are the `generate` flag names without `--`, dashes written `_`: [flags](cli.md#flags). `verbose` is not a key.
- A setting resolves from the entry, then `package_defaults`, then the flag default.

| Command | Reads |
|---|---|
| `muxt generate`, no flags | every entry in the file's directory, dependencies first; the directory's entry elsewhere |
| `muxt check`, `list-template-calls`, `list-template-callers` | `use_templates_variable` |
| `muxt` | all of it, shown by source under `Configuration:` |

Only a flagless `muxt generate` needs an entry. In a directory with none, the other commands use their own flags and defaults.

`muxt generate config` creates the file, `muxt generate package NAME` adds an entry, `muxt generate def NAME` adds a route template.

Errors name the file and the fix: `packages key "workspace" is not an import path; write "example.com/m/workspace"`, `key "use-receiver-type" is written with underscores; write "use_receiver_type"`, `unknown key "KEY"`, `no packages entry "KEY"; add one to generate this directory`.

Steps: [move generate flags](../how-to/muxt-json.md).

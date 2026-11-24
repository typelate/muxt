# muxt explore-module

Lists every package in the module with a generated routes file: its generation flags, the muxt commands to run against it, and the external URLs its templates reference.

```bash
muxt explore-module --format=json
```

The JSON is `{module, moduleDir, packages}`; each package has:

| Field | Contents |
|---|---|
| `path` | Import path of the package |
| `dir` | Directory of the package |
| `muxtVersion` | Version recorded in the generated file's header |
| `config` | `routesFunction`, `receiverInterface`, `receiverType`, `receiverPackage`, `templateRoutePathsType`, and the booleans `outputHTMX`, `outputDatastar`, `logger`, `pathPrefix`, `middleware` |
| `commands` | Ready-to-run `listRoutes`, `calls`, `callers`, `check`, `generate` command lines; `generate` is `muxt -C DIR generate` for a package a `muxt.json` entry configures, and otherwise carries the flags recorded in the generated file's header |
| `externalAssets` | `url`, `file`, `line`, `startCol`, `endCol` for each URL in a `.gohtml` file |

A package with several route sets has one row per set, each with the `path` and `dir` of the package and its own `config`, in the order of the generated files' names. The rows come from generated files, so a set not yet generated has none.

A `muxt.json` entry that breaks a rule fails the command, as it fails `generate`. Optional keys are absent when empty or false. Text output shows the same, minus the `logger`, `pathPrefix`, and `middleware` booleans, `endCol`, and `moduleDir`.

```bash
muxt explore-module --format=json | jq -r '.packages[].commands.calls' | sh
```

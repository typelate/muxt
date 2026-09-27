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
| `commands` | Ready-to-run `listRoutes`, `calls`, `callers`, `check`, `generate` command lines |
| `externalAssets` | `url`, `file`, `line`, `startCol`, `endCol` for each URL in a `.gohtml` file |

Optional keys are absent when empty or false. Text output shows the same, minus the `logger`, `pathPrefix`, and `middleware` booleans, `endCol`, and `moduleDir`.

```bash
muxt explore-module --format=json | jq -r '.packages[].commands.calls' | sh
```

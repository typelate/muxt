# Known Issues

## Templates must be valid HTML

Generating server side validations based on input field attributes
requires muxt parse the template HTML.
If you want to use this feature, your template HTML must be valid HTML.
This means no actions inside element tags. Actions in quoted attribute values are ok: `<div title="{{.HelpText}}">`.

If for example I want to do something like this

```html
<details {{if .Predicate}}open{{end}}>
    <p>Secret Message</p>
</details>
```

I instead do

```html
{{define "secret-message"}}<p>Secret Message</p>{{end}}

{{if .Predicate}}
<detail open>{{template "secret-message"}}</detail>
{{else}}
<detail>{{template "secret-message"}}</detail>
{{end}}
```

You will see a warning if your template source is not valid HTML.

## Type Checking

**Not all Go template features are supported.**
`muxt check` may give false negative type check errors.
If you find something you think is
wrong, [please open an issue (or better yet PR a line to the following list)](https://github.com/typelate/muxt/issues/new).

- methods or fields of type any may only be the final type in an action
- gotype comments used in the GoLand IDE from JetBrains products are not consulted
- ...

## Next Steps

If you are coming across a hard blocker when using Muxt, consider using the more sophisticated templating
language [templ](https://templ.guide).

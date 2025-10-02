# Action Type Checking

`muxt check` does best effort static analysis of template actions given the results from endpoint methods.
Template execution uses reflection during execution.
This makes static analysis fully compatible with Execute impossible.
Avoid using `any` (the empty interface) as a result or data field and `muxt` will be able to provide type checking for your templates.

Read the type-checking code in [github.com/typelate/check](https://pkg.go.dev/github.com/typelate/check).
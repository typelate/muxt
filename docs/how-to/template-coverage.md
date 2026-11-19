# See which template blocks your tests run

1. In the package directory, run `muxt test`.
2. Write a profile: `muxt test --output-cover-profile cover.out`.
3. Open it: `go tool cover -html=cover.out`.

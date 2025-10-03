module github.com/typelate/muxt

go 1.25.0

require (
	github.com/ettle/strcase v0.2.0
	github.com/spf13/pflag v1.0.10
	github.com/stretchr/testify v1.11.1
	github.com/typelate/check v0.0.3
	github.com/typelate/dom v0.6.1
	golang.org/x/net v0.44.0
	golang.org/x/tools v0.37.0
	rsc.io/script v0.0.2
)

require (
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/crhntr/txtarfmt v0.0.8 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/maxbrunsfeld/counterfeiter/v6 v6.12.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	golang.org/x/mod v0.28.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

tool (
	github.com/crhntr/txtarfmt/cmd/txtarfmt
	github.com/maxbrunsfeld/counterfeiter/v6
)

retract v0.15.0 // v0.15.0 used the wrong module path

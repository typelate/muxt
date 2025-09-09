package main

import (
	"bytes"
	_ "embed"
	"log"
	"os"
	"path/filepath"
	"text/template"
)

//go:generate go run ./

var (
	//go:embed README.md.template
	templateSource string
	templates      = template.Must(template.New("README.md.template").Delims("{{{", "}}}").Parse(templateSource))
)

func main() {
	var out bytes.Buffer
	if err := templates.Execute(&out, struct{}{}); err != nil {
		log.Fatal(err)
	}

	docsIndex, err := os.ReadFile(filepath.FromSlash("../../docs/README.md"))
	if err != nil {
		log.Fatal(err)
	}
	out.WriteString("#")
	docsIndex = bytes.ReplaceAll(docsIndex, []byte("](./"), []byte("](./docs/"))
	out.Write(docsIndex)

	if err := os.WriteFile(filepath.FromSlash("../../README.md"), out.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
}

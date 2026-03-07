package docs

import (
	"embed"
	"io/fs"
)

//go:embed *.md */*.md */*/*.md
var source embed.FS

func Markdown() fs.FS { return source }

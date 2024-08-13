package main

import (
	_ "embed"
	"log"
	"os"

	"github.com/crhntr/muxt/internal/generate"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	if err := generate.Command(wd, os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

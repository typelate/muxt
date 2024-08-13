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
	if err := generate.Command(os.Args[1:], wd, log.New(os.Stdout, "muxt: ", 0), os.LookupEnv); err != nil {
		log.Fatal(err)
	}
}

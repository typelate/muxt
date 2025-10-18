package main

import (
	"cmp"
	"log"
	"net/http"
	"os"

	hypertext2 "github.com/typelate/muxt/docs/examples/htmx"
)

func main() {
	mux := http.NewServeMux()
	srv := new(hypertext2.Server)
	hypertext2.TemplateRoutes(mux, srv)
	log.Fatal(http.ListenAndServe(":"+cmp.Or(os.Getenv("PORT"), "8000"), mux))
}

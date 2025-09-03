package main

import (
	"cmp"
	"log"
	"net/http"
	"os"

	hypertext "github.com/typelate/muxt/docs/htmx"
)

func main() {
	mux := http.NewServeMux()
	srv := new(hypertext.Server)
	hypertext.TemplateRoutes(mux, srv)
	log.Fatal(http.ListenAndServe(":"+cmp.Or(os.Getenv("PORT"), "8000"), mux))
}

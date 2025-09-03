// package muxt
//
// Generate HTTP Endpoints from HTML Templates
//
//	 `muxt check`
//
//		  Do some static analysis on the templates.
//
//	 `muxt documentation`
//
//		  This work in progress command will
//
//	 `muxt generate`
//
//		  Use this command to generate template_routes.go
//
//		  Consider using a Go generate comment where your templates variable is declared.
//
//		  //go:generate muxt generate --receiver-type=Server
//	   var templates = templates = template.Must(template.ParseFS(templatesSource, "*.gohtml"))
//
//	 `muxt version`
//
//		  Print the version of muxt to standard out.
package main

import (
	"fmt"
	"io"

	"github.com/typelate/muxt/internal/configuration"
)

func writeHelp(stdout io.Writer) {
	_, _ = fmt.Fprintf(stdout, `muxt - Generate HTTP Endpoints from HTML Templates

muxt check

	Do some static analysis on the templates. 

muxt documentation

	This work in progress command will 

muxt generate

	Use this command to generate template_routes.go
	
	Consider using a Go generate comment where your templates variable is declared.

	  //go:generate muxt generate --%s=Server
      var templates = templates = template.Must(template.ParseFS(templatesSource, "*.gohtml"))

muxt version

	Print the version of muxt to standard out.

`, configuration.ReceiverStaticType)
}

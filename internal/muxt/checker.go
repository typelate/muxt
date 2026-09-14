package muxt

import "go/types"

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o muxtfakes/fake_checker.go . Checker

// Checker answers what route resolution needs to know about the standard
// library: the types the reserved argument identifiers bind to, and which
// types marshal to and from text.
//
// Resolution asks these questions and nothing else of the packages outside
// the one it resolves routes in. load.StandardLibrary answers them from the
// official standard library a run loaded; a test answers them with a mock
// over types of its own, so it depends on muxt's rules rather than on the
// shape of any one standard library version.
type Checker interface {
	// ScopeType returns the type a reserved argument identifier binds to:
	// request (*http.Request), response (http.ResponseWriter), ctx
	// (context.Context), form (url.Values), multipart (*multipart.Form) and
	// body (io.Reader).
	ScopeType(identifier string) (types.Type, error)

	// FileHeader returns *multipart.FileHeader, the type a multipart struct
	// field binds an uploaded file to.
	FileHeader() (types.Type, error)

	// RawJSON returns json.RawMessage, the parameter type inferred for
	// unmarshalJSON(body) when the method is not yet defined.
	RawJSON() (types.Type, error)

	// TextUnmarshaler reports whether a pointer to tp implements
	// encoding.TextUnmarshaler, so tp parses from a request string.
	TextUnmarshaler(tp types.Type) bool

	// TextMarshaler reports whether tp implements encoding.TextMarshaler,
	// so a route path formats it as a path segment.
	TextMarshaler(tp types.Type) bool
}

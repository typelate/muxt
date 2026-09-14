package example

// Stand-ins for the standard library types a route argument binds to. The
// tests bind the reserved argument identifiers to these with a mock
// muxt.Checker, so what they state holds whatever the real ones look like.

type Request struct{ Method string }

type ResponseWriter interface{ WriteHeader(statusCode int) }

type Context interface{ Done() <-chan struct{} }

type Values map[string][]string

type FileHeader struct{ Filename string }

type Form struct {
	Value map[string][]string
	File  map[string][]*FileHeader
}

type File interface{ Close() error }

type Reader interface {
	Read(p []byte) (n int, err error)
}

type RawMessage []byte

type URL struct{ Path string }

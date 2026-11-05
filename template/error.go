package template

import "fmt"

// ExecError is the custom error type returned when Execute fails.
// It provides the name of the template in which the error occurred.
type ExecError struct {
	Name string
	Err  error
}

func (e ExecError) Error() string {
	return e.Err.Error()
}

func (e ExecError) Unwrap() error {
	return e.Err
}

func errorf(name string, format string, args ...any) ExecError {
	return ExecError{
		Name: name,
		Err:  fmt.Errorf(format, args...),
	}
}

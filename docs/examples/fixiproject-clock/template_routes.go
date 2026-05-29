package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
)

type RoutesReceiver interface {
	Time(context.Context, string, func(data string) error)
	Index() string
}

func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver) TemplateRoutePaths {
	pathsPrefix := ""
	bytesBufferPool := sync.Pool{New: func() any {
		return bytes.NewBuffer(nil)
	}}
	mux.HandleFunc("GET /time", func(response http.ResponseWriter, request *http.Request) {
		defer func() { _ = request.Body.Close() }()
		flusher, ok := response.(http.Flusher)
		if !ok {
			http.Error(response, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		lastEventID := request.Header.Get("Last-Event-Id")
		h := response.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Connection", "keep-alive")
		h.Set("Cache-Control", "no-cache")
		response.WriteHeader(http.StatusOK)
		flusher.Flush()
		var mut sync.Mutex
		receiver.Time(request.Context(), lastEventID, func(result string) error {
			if err := request.Context().Err(); err != nil {
				return err
			}
			buf := bytesBufferPool.Get().(*bytes.Buffer)
			buf.Reset()
			defer bytesBufferPool.Put(buf)
			td := SSETemplateData[RoutesReceiver, string]{receiver: receiver, request: request, pathsPrefix: pathsPrefix, result: result}
			if err := templates.ExecuteTemplate(buf, "GET /time Time(ctx, lastEventID, sse)", &td); err != nil {
				slog.ErrorContext(request.Context(), "failed to render page", slog.String("path", request.URL.Path), slog.String("pattern", request.Pattern), slog.String("error", err.Error()))
				return err
			}
			td.data = buf
			mut.Lock()
			defer mut.Unlock()
			if _, err := td.WriteTo(response); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		})
	})
	mux.HandleFunc("GET /{$}", func(response http.ResponseWriter, request *http.Request) {
		td := TemplateData[RoutesReceiver, string]{receiver: receiver, response: response, request: request, pathsPrefix: pathsPrefix}
		if len(td.errList) == 0 {
			td.result = receiver.Index()
			td.okay = true
		}
		buf := bytesBufferPool.Get().(*bytes.Buffer)
		buf.Reset()
		defer bytesBufferPool.Put(buf)
		if err := templates.ExecuteTemplate(buf, "GET /{$} Index()", &td); err != nil {
			slog.ErrorContext(request.Context(), "failed to render page", slog.String("path", request.URL.Path), slog.String("pattern", request.Pattern), slog.String("error", err.Error()))
			http.Error(response, "failed to render page", http.StatusInternalServerError)
			return
		}
		statusCode := cmp.Or(td.statusCode, td.errStatusCode, http.StatusOK)
		if contentType := response.Header().Get("Content-Type"); contentType == "" {
			response.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		response.Header().Set("content-length", strconv.Itoa(buf.Len()))
		response.WriteHeader(statusCode)
		_, _ = buf.WriteTo(response)
	})
	return TemplateRoutePaths{pathsPrefix: pathsPrefix}
}

type TemplateData[R any, T any] struct {
	receiver      R
	response      http.ResponseWriter
	request       *http.Request
	result        T
	statusCode    int
	errStatusCode int
	okay          bool
	errList       []error
	redirectURL   string
	pathsPrefix   string
}

func (data *TemplateData[R, T]) MuxtVersion() string {
	const muxtVersion = "(devel)"
	return muxtVersion
}

func (data *TemplateData[R, T]) Path() TemplateRoutePaths {
	return TemplateRoutePaths{pathsPrefix: data.pathsPrefix}
}

func (data *TemplateData[R, T]) Result() T {
	return data.result
}

func (data *TemplateData[R, T]) Request() *http.Request {
	return data.request
}

func (data *TemplateData[R, T]) StatusCode(statusCode int) *TemplateData[R, T] {
	data.statusCode = statusCode
	return data
}

func (data *TemplateData[R, T]) Header(key, value string) *TemplateData[R, T] {
	data.response.Header().Set(key, value)
	return data
}

func (data *TemplateData[R, T]) Ok() bool {
	return data.okay
}

func (data *TemplateData[R, T]) Err() error {
	return errors.Join(data.errList...)
}

func (data *TemplateData[R, T]) Receiver() R {
	return data.receiver
}

func (data *TemplateData[R, T]) Redirect(url string, code int) (*TemplateData[R, T], error) {
	if code < 300 || code >= 400 {
		return data, fmt.Errorf("invalid status code %d for redirect", code)
	}
	data.redirectURL = url
	return data.StatusCode(code), nil
}

func (data *TemplateData[R, T]) RedirectMultipleChoices(url string) (*TemplateData[R, T], error) {
	return data.Redirect(url, http.StatusMultipleChoices)
}

func (data *TemplateData[R, T]) RedirectMovedPermanently(url string) (*TemplateData[R, T], error) {
	return data.Redirect(url, http.StatusMovedPermanently)
}

func (data *TemplateData[R, T]) RedirectFound(url string) (*TemplateData[R, T], error) {
	return data.Redirect(url, http.StatusFound)
}

func (data *TemplateData[R, T]) RedirectSeeOther(url string) (*TemplateData[R, T], error) {
	return data.Redirect(url, http.StatusSeeOther)
}

func (data *TemplateData[R, T]) String() string {
	return ""
}

type SSETemplateData[R, T any] struct {
	receiver          R
	request           *http.Request
	result            T
	pathsPrefix       string
	event, id         *string
	retryMilliseconds *int
	errList           []error
	data              *bytes.Buffer
}

func (m *SSETemplateData[R, T]) String() string { return "" }

func (m *SSETemplateData[R, T]) Receiver() R {
	return m.receiver
}

func (m *SSETemplateData[R, T]) Request() *http.Request {
	return m.request
}

func (m *SSETemplateData[R, T]) Result() T {
	return m.result
}

func (m *SSETemplateData[R, T]) Err() error {
	return errors.Join(m.errList...)
}

func (m *SSETemplateData[R, T]) Event(event string) *SSETemplateData[R, T] {
	m.event = &event
	return m
}

func (m *SSETemplateData[R, T]) ID(id string) *SSETemplateData[R, T] {
	m.id = &id
	return m
}

func (m *SSETemplateData[R, T]) Retry(retryMilliseconds int) *SSETemplateData[R, T] {
	m.retryMilliseconds = &retryMilliseconds
	return m
}

func (m *SSETemplateData[R, T]) Path() TemplateRoutePaths {
	return TemplateRoutePaths{pathsPrefix: m.pathsPrefix}
}

func (m *SSETemplateData[R, T]) WriteTo(w io.Writer) (int64, error) {
	if m.id != nil && strings.ContainsAny(*m.id, "\r\n\x00") {
		return 0, errors.New("sse: id contains a forbidden character")
	}
	if m.event != nil && strings.ContainsAny(*m.event, "\r\n") {
		return 0, errors.New("sse: event contains a forbidden character")
	}
	var bytesWritten int
	if m.id != nil {
		if n, err := io.WriteString(w, "id: "); err != nil {
			return int64(bytesWritten + n), err
		} else {
			bytesWritten += n
		}
		if n, err := io.WriteString(w, *m.id); err != nil {
			return int64(bytesWritten + n), err
		} else {
			bytesWritten += n
		}
		if n, err := w.Write([]byte{'\n'}); err != nil {
			return int64(bytesWritten + n), err
		} else {
			bytesWritten += n
		}
	}
	if m.event != nil {
		if n, err := io.WriteString(w, "event: "); err != nil {
			return int64(bytesWritten + n), err
		} else {
			bytesWritten += n
		}
		if n, err := io.WriteString(w, *m.event); err != nil {
			return int64(bytesWritten + n), err
		} else {
			bytesWritten += n
		}
		if n, err := w.Write([]byte{'\n'}); err != nil {
			return int64(bytesWritten + n), err
		} else {
			bytesWritten += n
		}
	}
	if m.retryMilliseconds != nil {
		if n, err := io.WriteString(w, "retry: "); err != nil {
			return int64(bytesWritten + n), err
		} else {
			bytesWritten += n
		}
		var retryBuf [20]byte
		if n, err := w.Write(strconv.AppendInt(retryBuf[:0], int64(*m.retryMilliseconds), 10)); err != nil {
			return int64(bytesWritten + n), err
		} else {
			bytesWritten += n
		}
		if n, err := w.Write([]byte{'\n'}); err != nil {
			return int64(bytesWritten + n), err
		} else {
			bytesWritten += n
		}
	}
	data := m.data.Bytes()
	if bytes.IndexByte(data, '\r') >= 0 {
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
	}
	data = bytes.TrimSuffix(data, []byte{'\n'})
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		if n, err := io.WriteString(w, "data: "); err != nil {
			return int64(bytesWritten + n), err
		} else {
			bytesWritten += n
		}
		if len(line) > 0 {
			if n, err := w.Write(line); err != nil {
				return int64(bytesWritten + n), err
			} else {
				bytesWritten += n
			}
		}
		if n, err := w.Write([]byte{'\n'}); err != nil {
			return int64(bytesWritten + n), err
		} else {
			bytesWritten += n
		}
	}
	if n, err := w.Write([]byte{'\n'}); err != nil {
		return int64(bytesWritten + n), err
	} else {
		bytesWritten += n
	}
	return int64(bytesWritten), nil
}

type TemplateRoutePaths struct {
	pathsPrefix string
}

func (routePaths TemplateRoutePaths) Time() string {
	return path.Join(cmp.Or(routePaths.pathsPrefix, "/"), "time")
}

func (routePaths TemplateRoutePaths) Index() string {
	return "/"
}

package astgen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"net/http"
	"strconv"
	"strings"
)

var httpCodes = map[int]string{
	http.StatusContinue:           "StatusContinue",
	http.StatusSwitchingProtocols: "StatusSwitchingProtocols",
	http.StatusProcessing:         "StatusProcessing",
	http.StatusEarlyHints:         "StatusEarlyHints",

	http.StatusOK:                   "StatusOK",
	http.StatusCreated:              "StatusCreated",
	http.StatusAccepted:             "StatusAccepted",
	http.StatusNonAuthoritativeInfo: "StatusNonAuthoritativeInfo",
	http.StatusNoContent:            "StatusNoContent",
	http.StatusResetContent:         "StatusResetContent",
	http.StatusPartialContent:       "StatusPartialContent",
	http.StatusMultiStatus:          "StatusMultiStatus",
	http.StatusAlreadyReported:      "StatusAlreadyReported",
	http.StatusIMUsed:               "StatusIMUsed",

	http.StatusMultipleChoices:   "StatusMultipleChoices",
	http.StatusMovedPermanently:  "StatusMovedPermanently",
	http.StatusFound:             "StatusFound",
	http.StatusSeeOther:          "StatusSeeOther",
	http.StatusNotModified:       "StatusNotModified",
	http.StatusUseProxy:          "StatusUseProxy",
	http.StatusTemporaryRedirect: "StatusTemporaryRedirect",
	http.StatusPermanentRedirect: "StatusPermanentRedirect",

	http.StatusBadRequest:                   "StatusBadRequest",
	http.StatusUnauthorized:                 "StatusUnauthorized",
	http.StatusPaymentRequired:              "StatusPaymentRequired",
	http.StatusForbidden:                    "StatusForbidden",
	http.StatusNotFound:                     "StatusNotFound",
	http.StatusMethodNotAllowed:             "StatusMethodNotAllowed",
	http.StatusNotAcceptable:                "StatusNotAcceptable",
	http.StatusProxyAuthRequired:            "StatusProxyAuthRequired",
	http.StatusRequestTimeout:               "StatusRequestTimeout",
	http.StatusConflict:                     "StatusConflict",
	http.StatusGone:                         "StatusGone",
	http.StatusLengthRequired:               "StatusLengthRequired",
	http.StatusPreconditionFailed:           "StatusPreconditionFailed",
	http.StatusRequestEntityTooLarge:        "StatusRequestEntityTooLarge",
	http.StatusRequestURITooLong:            "StatusRequestURITooLong",
	http.StatusUnsupportedMediaType:         "StatusUnsupportedMediaType",
	http.StatusRequestedRangeNotSatisfiable: "StatusRequestedRangeNotSatisfiable",
	http.StatusExpectationFailed:            "StatusExpectationFailed",
	http.StatusTeapot:                       "StatusTeapot",
	http.StatusMisdirectedRequest:           "StatusMisdirectedRequest",
	http.StatusUnprocessableEntity:          "StatusUnprocessableEntity",
	http.StatusLocked:                       "StatusLocked",
	http.StatusFailedDependency:             "StatusFailedDependency",
	http.StatusTooEarly:                     "StatusTooEarly",
	http.StatusUpgradeRequired:              "StatusUpgradeRequired",
	http.StatusPreconditionRequired:         "StatusPreconditionRequired",
	http.StatusTooManyRequests:              "StatusTooManyRequests",
	http.StatusRequestHeaderFieldsTooLarge:  "StatusRequestHeaderFieldsTooLarge",
	http.StatusUnavailableForLegalReasons:   "StatusUnavailableForLegalReasons",

	http.StatusInternalServerError:           "StatusInternalServerError",
	http.StatusNotImplemented:                "StatusNotImplemented",
	http.StatusBadGateway:                    "StatusBadGateway",
	http.StatusServiceUnavailable:            "StatusServiceUnavailable",
	http.StatusGatewayTimeout:                "StatusGatewayTimeout",
	http.StatusHTTPVersionNotSupported:       "StatusHTTPVersionNotSupported",
	http.StatusVariantAlsoNegotiates:         "StatusVariantAlsoNegotiates",
	http.StatusInsufficientStorage:           "StatusInsufficientStorage",
	http.StatusLoopDetected:                  "StatusLoopDetected",
	http.StatusNotExtended:                   "StatusNotExtended",
	http.StatusNetworkAuthenticationRequired: "StatusNetworkAuthenticationRequired",
}

// HTTPStatusName converts an http.Status constant name to its integer value
func HTTPStatusName(name string) (int, error) {
	n := strings.TrimPrefix(name, "http.")
	candidates := make([]string, 0, len(httpCodes))
	for code, constName := range httpCodes {
		if constName == n {
			return code, nil
		}
		candidates = append(candidates, constName)
	}
	// The caller prefixes the message with the token ("invalid status
	// code X: …"), so neither branch repeats it.
	if suggestion, ok := NearestString(n, candidates); ok {
		return 0, fmt.Errorf("did you mean http.%s?", suggestion)
	}
	return 0, errors.New("not an http.Status constant")
}

// HTTPStatusCode creates an AST expression for an HTTP status code.
func HTTPStatusCode(im ImportManager, n int) ast.Expr {
	ident, ok := httpCodes[n]
	if !ok {
		return &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(n)}
	}
	return ExportedIdentifier(im, "", "net/http", ident)
}

var httpMethods = map[string]string{
	http.MethodGet:     "MethodGet",
	http.MethodHead:    "MethodHead",
	http.MethodPost:    "MethodPost",
	http.MethodPut:     "MethodPut",
	http.MethodPatch:   "MethodPatch",
	http.MethodDelete:  "MethodDelete",
	http.MethodConnect: "MethodConnect",
	http.MethodOptions: "MethodOptions",
	http.MethodTrace:   "MethodTrace",
}

// HTTPMethod creates an AST expression for an HTTP method: the net/http
// constant when there is one, otherwise a string literal.
func HTTPMethod(im ImportManager, method string) ast.Expr {
	ident, ok := httpMethods[method]
	if !ok {
		return String(method)
	}
	return ExportedIdentifier(im, "", "net/http", ident)
}

func HTTPErrorCall(im ImportManager, response, message ast.Expr, code int) *ast.CallExpr {
	return Call(im, "", "net/http", "Error", response, message, HTTPStatusCode(im, code))
}

func HTTPRequestPtr(im ImportManager) *ast.StarExpr {
	return &ast.StarExpr{
		X: ExportedIdentifier(im, "http", "net/http", "Request"),
	}
}

func HTTPResponseWriter(im ImportManager) *ast.SelectorExpr {
	return ExportedIdentifier(im, "http", "net/http", "ResponseWriter")
}

func AddNetHTTP(im ImportManager) string {
	return im.Import("", "net/http")
}

func HTTPResponseField(im ImportManager, ident string) *ast.Field {
	return &ast.Field{Names: []*ast.Ident{ast.NewIdent(ident)}, Type: HTTPResponseWriter(im)}
}

func HTTPRequestField(im ImportManager, ident string) *ast.Field {
	return &ast.Field{Names: []*ast.Ident{ast.NewIdent(ident)}, Type: HTTPRequestPtr(im)}
}

func HTTPHandlerFuncType(file ImportManager, res, req string) *ast.FuncType {
	return &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{HTTPResponseField(file, res), HTTPRequestField(file, req)}}}
}

func HTTPHandler(im ImportManager) *ast.SelectorExpr {
	return ExportedIdentifier(im, "http", "net/http", "Handler")
}

func HTTPMiddlewareFuncType(im ImportManager) *ast.FuncType {
	return &ast.FuncType{
		Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("next")}, Type: HTTPHandler(im)}}},
		Results: &ast.FieldList{List: []*ast.Field{{Type: HTTPHandler(im)}}},
	}
}

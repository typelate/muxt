package astgen

import (
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
	for code, constName := range httpCodes {
		if constName == n {
			return code, nil
		}
	}
	return 0, fmt.Errorf("unknown %s", name)
}

// HTTPStatusCode creates an AST expression for an HTTP status code.
// Returns http.StatusXXX constant if available, otherwise a literal int.
func HTTPStatusCode(im ImportManager, n int) ast.Expr {
	ident, ok := httpCodes[n]
	if !ok {
		return &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(n)}
	}
	return ExportedIdentifier(im, "", "net/http", ident)
}

// HTTPErrorCall creates an http.Error call expression
func HTTPErrorCall(im ImportManager, response, message ast.Expr, code int) *ast.CallExpr {
	return Call(im, "", "net/http", "Error", []ast.Expr{
		response,
		message,
		HTTPStatusCode(im, code),
	})
}

// HTTPRequestPtr creates a *http.Request type expression
func HTTPRequestPtr(im ImportManager) *ast.StarExpr {
	return &ast.StarExpr{
		X: ExportedIdentifier(im, "http", "net/http", "Request"),
	}
}

// HTTPResponseWriter creates an http.ResponseWriter type expression
func HTTPResponseWriter(im ImportManager) *ast.SelectorExpr {
	return ExportedIdentifier(im, "http", "net/http", "ResponseWriter")
}

// HTTPHeader creates an http.Header type expression
func HTTPHeader(im ImportManager) *ast.SelectorExpr {
	return ExportedIdentifier(im, "http", "net/http", "Header")
}

// AddNetHTTP registers the net/http import and returns its identifier
func AddNetHTTP(im ImportManager) string {
	return im.Import("", "net/http")
}

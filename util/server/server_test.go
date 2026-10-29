package server

import (
	"net/http"
	"testing"

	"google.golang.org/grpc/codes"
)

func TestGRPCCodeFromHTTPStatus(t *testing.T) {
	for status, want := range map[int]codes.Code{
		http.StatusOK:                  codes.OK,
		http.StatusRequestTimeout:      codes.Canceled,
		http.StatusInternalServerError: codes.Internal,
		http.StatusBadRequest:          codes.InvalidArgument,
		http.StatusGatewayTimeout:      codes.DeadlineExceeded,
		http.StatusNotFound:            codes.NotFound,
		http.StatusConflict:            codes.AlreadyExists,
		http.StatusForbidden:           codes.PermissionDenied,
		http.StatusUnauthorized:        codes.Unauthenticated,
		http.StatusTooManyRequests:     codes.ResourceExhausted,
		http.StatusNotImplemented:      codes.Unimplemented,
		http.StatusServiceUnavailable:  codes.Unavailable,
		http.StatusTeapot:              codes.Unknown,
		0:                              codes.Unknown,
	} {
		if got := GRPCCodeFromHTTPStatus(status); got != want {
			t.Errorf("HTTP %d: %v, want %v", status, got, want)
		}
	}
}

package server

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/logger"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const testSecret = "s3cret-Pa55word-XYZ"

func basicHeader(user, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))
}

func TestAuthErrorCodes(t *testing.T) {
	auth := NewAuthentication(
		[]*config.BasicCredential{{User: "alice", Password: testSecret}},
		nil, AccessAll, logger.NewLogger("test", logger.DefaultConfig()))
	info := &grpc.UnaryServerInfo{FullMethod: "/tes.TaskService/ListTasks"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) { return "ok", nil }

	for _, tc := range []struct {
		name string
		ctx  context.Context
		code codes.Code
		want string
	}{
		{"no metadata", context.Background(), codes.InvalidArgument, "FUNNEL-AUTH-001: "},
		{"no credentials", metadata.NewIncomingContext(context.Background(), metadata.MD{}), codes.Unauthenticated, "FUNNEL-AUTH-002: "},
		{"wrong password", metadata.NewIncomingContext(context.Background(),
			metadata.Pairs("authorization", basicHeader("alice", "wrong-"+testSecret))), codes.Unauthenticated, "FUNNEL-AUTH-003: "},
		{"bearer without OIDC", metadata.NewIncomingContext(context.Background(),
			metadata.Pairs("authorization", "Bearer "+testSecret)), codes.Unauthenticated, "FUNNEL-AUTH-002: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := auth.Interceptor(tc.ctx, nil, info, handler)
			st, _ := status.FromError(err)
			if st.Code() != tc.code {
				t.Errorf("code %v, want %v", st.Code(), tc.code)
			}
			if !strings.HasPrefix(st.Message(), tc.want) {
				t.Errorf("message %q should start with %q", st.Message(), tc.want)
			}
			if strings.Contains(st.Message(), testSecret) {
				t.Errorf("message %q leaks the credentials", st.Message())
			}
		})
	}

	t.Run("valid credentials", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(),
			metadata.Pairs("authorization", basicHeader("alice", testSecret)))
		resp, err := auth.Interceptor(ctx, nil, info, handler)
		if err != nil || resp != "ok" {
			t.Fatalf("got %v, %v", resp, err)
		}
	})
}

// The interceptor returns errInvalidBearerToken when the OIDC check of a
// Bearer token fails (Authentication.Interceptor).
func TestBearerTokenNotAcceptedCode(t *testing.T) {
	st, _ := status.FromError(errInvalidBearerToken)
	if st.Code() != codes.Unauthenticated || !strings.HasPrefix(st.Message(), "FUNNEL-AUTH-004: ") {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
}

func TestOidcFlowErrorCodes(t *testing.T) {
	c := &OidcConfig{
		local:  &config.OidcAuth{ServiceConfigURL: "https://idp.example/.well-known", ClientId: "client", ClientSecret: testSecret},
		oauth2: &oauth2.Config{Endpoint: oauth2.Endpoint{AuthURL: "https://idp.example/auth", TokenURL: "http://127.0.0.1:1/token"}},
	}
	state := c.computeState(httptest.NewRequest(http.MethodGet, "/login", nil))

	get := func(query url.Values) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c.HandleAuthCode(w, httptest.NewRequest(http.MethodGet, "/login?"+query.Encode(), nil))
		return w
	}

	t.Run("wrong state", func(t *testing.T) {
		w := get(url.Values{"state": {"bogus"}})
		if w.Code != http.StatusBadRequest || !strings.HasPrefix(w.Body.String(), "FUNNEL-AUTH-005: ") {
			t.Fatalf("got %d %q", w.Code, w.Body.String())
		}
	})

	t.Run("error from the identity provider", func(t *testing.T) {
		long := strings.Repeat("x", 10*1024)
		w := get(url.Values{"state": {state}, "error": {"access_denied"}, "error_description": {long}})
		body := w.Body.String()
		if w.Code != http.StatusInternalServerError || !strings.HasPrefix(body, "FUNNEL-AUTH-006: ") {
			t.Fatalf("got %d %q", w.Code, body[:min(len(body), 100)])
		}
		if !strings.Contains(body, "access_denied") {
			t.Errorf("the provider's error code should be shown: %q", body[:min(len(body), 300)])
		}
		if len(body) > 600 {
			t.Errorf("the provider's error description is echoed unbounded (%d bytes)", len(body))
		}
	})

	t.Run("token exchange fails", func(t *testing.T) {
		w := get(url.Values{"state": {state}, "code": {"auth-code"}})
		if w.Code != http.StatusInternalServerError || !strings.HasPrefix(w.Body.String(), "FUNNEL-AUTH-007: ") {
			t.Fatalf("got %d %q", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), testSecret) {
			t.Errorf("message leaks the client secret: %q", w.Body.String())
		}
	})
}

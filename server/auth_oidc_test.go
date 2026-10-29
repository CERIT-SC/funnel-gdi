package server

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/ohsu-comp-bio/funnel/config"
)

// fakeIdP is an OIDC identity provider for tests: discovery document, JWKS,
// token introspection and token endpoint.
type fakeIdP struct {
	*httptest.Server
	key         jwk.Key
	active      atomic.Bool
	introStatus atomic.Int32
	introType   atomic.Value
	introCalls  atomic.Int32
	noIntro     bool
}

func newFakeIdP(t *testing.T, noIntrospection bool) *fakeIdP {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	key, err := jwk.FromRaw(raw)
	if err != nil {
		t.Fatal(err)
	}
	key.Set(jwk.KeyIDKey, "test-key")
	key.Set(jwk.AlgorithmKey, jwa.RS256)

	idp := &fakeIdP{key: key, noIntro: noIntrospection}
	idp.active.Store(true)
	idp.introStatus.Store(http.StatusOK)
	idp.introType.Store("application/json")

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		conf := OidcRemoteConfig{
			Issuer:                idp.URL,
			JwksURI:               idp.URL + "/jwks",
			AuthorizationEndpoint: idp.URL + "/auth",
			TokenEndpoint:         idp.URL + "/token",
		}
		if !idp.noIntro {
			conf.IntrospectionEndpoint = idp.URL + "/introspect"
		}
		json.NewEncoder(w).Encode(conf)
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		pub, _ := idp.key.PublicKey()
		set := jwk.NewSet()
		set.AddKey(pub)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(set)
	})
	mux.HandleFunc("/introspect", func(w http.ResponseWriter, r *http.Request) {
		idp.introCalls.Add(1)
		if user, pass, ok := r.BasicAuth(); !ok || user != "client" || pass != "secret" {
			http.Error(w, "client authentication required", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", idp.introType.Load().(string))
		w.WriteHeader(int(idp.introStatus.Load()))
		json.NewEncoder(w).Encode(IntrospectionResponse{Active: idp.active.Load()})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "issued-token", "token_type": "Bearer", "expires_in": 3600})
	})
	idp.Server = httptest.NewServer(mux)
	t.Cleanup(idp.Close)
	return idp
}

// token returns a JWT signed by the provider, modified by opts.
func (idp *fakeIdP) token(t *testing.T, subject string, opts ...func(jwt.Token)) string {
	t.Helper()
	tok := jwt.New()
	tok.Set(jwt.IssuerKey, idp.URL)
	tok.Set(jwt.SubjectKey, subject)
	tok.Set(jwt.ExpirationKey, time.Now().Add(time.Hour))
	tok.Set(jwt.AudienceKey, []string{"funnel"})
	tok.Set("scope", "openid funnel")
	for _, o := range opts {
		o(tok)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, idp.key))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}

func newTestOidc(t *testing.T, idp *fakeIdP, modify func(*config.OidcAuth)) *OidcConfig {
	t.Helper()
	conf := &config.OidcAuth{
		ServiceConfigURL: idp.URL + "/.well-known/openid-configuration",
		ClientId:         "client",
		ClientSecret:     "secret",
		RedirectURL:      "http://funnel.example/login",
		Admins:           []string{"admin-user"},
	}
	if modify != nil {
		modify(conf)
	}
	return initOidcConfig(conf)
}

func TestOidcConfigFromProvider(t *testing.T) {
	idp := newFakeIdP(t, false)
	c := newTestOidc(t, idp, func(c *config.OidcAuth) { c.RequireScope = "funnel" })

	if c.oauth2.ClientID != "client" || c.oauth2.Endpoint.TokenURL != idp.URL+"/token" || c.oauth2.RedirectURL != "http://funnel.example/login" {
		t.Errorf("unexpected OAuth2 config: %+v", c.oauth2)
	}
	if strings.Join(c.oauth2.Scopes, " ") != "openid funnel" {
		t.Errorf("scopes %v", c.oauth2.Scopes)
	}
	if initOidcConfig(nil) != nil || initOidcConfig(&config.OidcAuth{}) != nil {
		t.Error("no OIDC config expected without ServiceConfigURL")
	}
}

func TestOidcAuthorize(t *testing.T) {
	idp := newFakeIdP(t, false)
	c := newTestOidc(t, idp, func(c *config.OidcAuth) {
		c.RequireAudience = "funnel"
		c.RequireScope = "funnel"
	})

	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	otherKey, _ := jwk.FromRaw(other)
	otherKey.Set(jwk.KeyIDKey, "test-key")

	for _, tc := range []struct {
		name  string
		token string
		user  string
	}{
		{"valid token", idp.token(t, "alice"), "alice"},
		{"admin", idp.token(t, "admin-user"), "admin-user"},
		{"expired", idp.token(t, "alice", func(tok jwt.Token) { tok.Set(jwt.ExpirationKey, time.Now().Add(-time.Hour)) }), ""},
		{"wrong issuer", idp.token(t, "alice", func(tok jwt.Token) { tok.Set(jwt.IssuerKey, "https://evil.example") }), ""},
		{"missing audience", idp.token(t, "alice", func(tok jwt.Token) { tok.Set(jwt.AudienceKey, []string{"other"}) }), ""},
		{"missing scope", idp.token(t, "alice", func(tok jwt.Token) { tok.Set("scope", "openid") }), ""},
		{"no scope claim", idp.token(t, "alice", func(tok jwt.Token) { tok.Remove("scope") }), ""},
		{"not a JWT", "not-a-jwt", ""},
		{"signed by another key", func() string {
			tok := jwt.New()
			tok.Set(jwt.IssuerKey, idp.URL)
			tok.Set(jwt.SubjectKey, "mallory")
			s, _ := jwt.Sign(tok, jwt.WithKey(jwa.RS256, otherKey))
			return string(s)
		}(), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := c.Authorize("Bearer " + tc.token)
			switch {
			case tc.user == "" && info != nil:
				t.Fatalf("token accepted as %q", info.Username)
			case tc.user != "" && info == nil:
				t.Fatal("token rejected")
			case tc.user != "":
				if info.Username != tc.user || info.Token != tc.token {
					t.Errorf("got %+v", info)
				}
				if info.IsAdmin != (tc.user == "admin-user") {
					t.Errorf("IsAdmin %v for %s", info.IsAdmin, tc.user)
				}
			}
		})
	}
}

func TestOidcIntrospection(t *testing.T) {
	idp := newFakeIdP(t, false)
	c := newTestOidc(t, idp, nil)
	token := idp.token(t, "alice")

	for _, tc := range []struct {
		name        string
		active      bool
		status      int
		contentType string
		accepted    bool
	}{
		{"active", true, http.StatusOK, "application/json", true},
		{"revoked", false, http.StatusOK, "application/json", false},
		{"error status", true, http.StatusInternalServerError, "application/json", false},
		{"not JSON", true, http.StatusOK, "text/plain", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idp.active.Store(tc.active)
			idp.introStatus.Store(int32(tc.status))
			idp.introType.Store(tc.contentType)
			if got := c.Authorize("Bearer "+token) != nil; got != tc.accepted {
				t.Errorf("accepted %v, want %v", got, tc.accepted)
			}
		})
	}

	t.Run("without client credentials", func(t *testing.T) {
		idp.active.Store(true)
		idp.introStatus.Store(http.StatusOK)
		idp.introType.Store("application/json")
		c.local.ClientSecret = ""
		defer func() { c.local.ClientSecret = "secret" }()
		if c.Authorize("Bearer "+token) != nil {
			t.Error("the provider requires client credentials for introspection")
		}
	})
}

func TestOidcWithoutIntrospectionEndpoint(t *testing.T) {
	idp := newFakeIdP(t, true)
	c := newTestOidc(t, idp, nil)
	if c.Authorize("Bearer "+idp.token(t, "alice")) == nil {
		t.Fatal("a valid token should be accepted when the provider has no introspection endpoint")
	}
	if idp.introCalls.Load() != 0 {
		t.Error("introspection endpoint called although not configured")
	}
}

func TestOidcLoginFlow(t *testing.T) {
	idp := newFakeIdP(t, false)
	c := newTestOidc(t, idp, nil)
	auth := &Authentication{oidc: c, basic: map[string]string{}}

	// Start: redirect to the provider with the client ID and a state.
	w := httptest.NewRecorder()
	auth.LoginHandler(w, httptest.NewRequest(http.MethodGet, "/login", nil))
	loc, _ := url.Parse(w.Header().Get("Location"))
	if w.Code != http.StatusSeeOther || !strings.HasPrefix(loc.String(), idp.URL+"/auth") {
		t.Fatalf("got %d %s", w.Code, loc)
	}
	state := loc.Query().Get("state")
	if loc.Query().Get("client_id") != "client" || state == "" {
		t.Fatalf("redirect %s", loc)
	}

	// Return from the provider with a code: the token is stored in a cookie.
	w = httptest.NewRecorder()
	auth.LoginHandler(w, httptest.NewRequest(http.MethodGet, "/login?state="+state+"&code=abc", nil))
	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	var jwtCookie *http.Cookie
	for _, ck := range w.Result().Cookies() {
		if ck.Name == "jwt" {
			jwtCookie = ck
		}
	}
	if jwtCookie == nil || jwtCookie.Value != "issued-token" || !jwtCookie.HttpOnly {
		t.Fatalf("cookie %+v", jwtCookie)
	}

	// The token can be read back from the cookie.
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login/token", nil)
	req.AddCookie(jwtCookie)
	auth.EchoTokenHandler(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "issued-token") {
		t.Errorf("got %d %q", w.Code, w.Body.String())
	}

	// Without the cookie there is nothing to echo.
	w = httptest.NewRecorder()
	auth.EchoTokenHandler(w, httptest.NewRequest(http.MethodGet, "/login/token", nil))
	if strings.Contains(w.Body.String(), "issued-token") {
		t.Error("token echoed without cookie")
	}

	// Only GET is supported.
	w = httptest.NewRecorder()
	c.HandleAuthCode(w, httptest.NewRequest(http.MethodPost, "/login", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: got %d", w.Code)
	}
}

func TestLoginWithoutAuthentication(t *testing.T) {
	none := NewAuthentication(nil, nil, AccessAll, nil)
	w := httptest.NewRecorder()
	none.LoginHandler(w, httptest.NewRequest(http.MethodGet, "/login", nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Errorf("got %d %s", w.Code, w.Header().Get("Location"))
	}

	w = httptest.NewRecorder()
	none.EchoTokenHandler(w, httptest.NewRequest(http.MethodGet, "/login/token", nil))
	if w.Code != http.StatusOK || w.Body.Len() != 0 {
		t.Errorf("token endpoint: got %d %q", w.Code, w.Body.String())
	}
}

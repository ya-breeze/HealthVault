package server

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/cfaccess"
)

const cfAccessTestAUD = "test-aud"

// newCFAccessTestServer serves a JWKS containing key's public half, and
// returns the issuer string a token must carry plus a teardown func.
func newCFAccessTestServer(t *testing.T, key *rsa.PrivateKey, kid string) (certsURL, issuer string, teardown func()) {
	t.Helper()
	type jwk struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		N   string `json:"n"`
		E   string `json:"e"`
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/cdn-cgi/access/certs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"keys": []jwk{{
				Kid: kid,
				Kty: "RSA",
				N:   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
			}},
		})
	})
	srv := httptest.NewServer(mux)
	return srv.URL + "/cdn-cgi/access/certs", srv.URL, srv.Close
}

func signCFAccessTestToken(t *testing.T, key *rsa.PrivateKey, kid, issuer, aud, email string) string {
	t.Helper()
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   issuer,
		"aud":   aud,
		"email": email,
		"sub":   "google-oauth2|" + uuid.NewString(),
		"exp":   now.Add(time.Hour).Unix(),
		"nbf":   now.Add(-time.Minute).Unix(),
		"iat":   now.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func doCFAccess(h *authHandlers, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/cf-access", nil)
	if token != "" {
		req.Header.Set("Cf-Access-Jwt-Assertion", token)
	}
	rec := httptest.NewRecorder()
	h.CFAccess(rec, req)
	return rec
}

func TestCFAccess_DisabledConfigAnswers404(t *testing.T) {
	// newLoginTestHandlers leaves cfVerifier nil and cfEmailMap empty, the
	// same shape a deployment with no HCW_CF_ACCESS_* settings has.
	h, _ := newLoginTestHandlers(t)

	rec := doCFAccess(h, "irrelevant-token")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("CFAccess with unconfigured verifier: expected 404, got %d", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("CFAccess with unconfigured verifier set cookies: %v", rec.Result().Cookies())
	}
}

func TestCFAccess_SpoofedUnsignedHeaderAnswers401(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	certsURL, issuer, teardown := newCFAccessTestServer(t, key, "kid-1")
	defer teardown()

	h, _ := newLoginTestHandlers(t)
	h.cfVerifier = cfaccess.NewWithEndpoint(certsURL, issuer, cfAccessTestAUD)
	h.cfEmailMap = map[string]string{"alice@example.com": "alice"}

	// alg:none — a request off the LAN port can set this header to anything,
	// including a token that was never signed by Cloudflare's key.
	unsigned := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"iss": issuer, "aud": cfAccessTestAUD, "email": "alice@example.com",
	})
	spoofed, err := unsigned.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign spoofed token: %v", err)
	}

	rec := doCFAccess(h, spoofed)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("CFAccess with a spoofed unsigned header: expected 401, got %d", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("CFAccess with a spoofed unsigned header set cookies: %v", rec.Result().Cookies())
	}
}

func TestCFAccess_VerifiedButUnmappedEmailAnswers403(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	certsURL, issuer, teardown := newCFAccessTestServer(t, key, "kid-1")
	defer teardown()

	h, _ := newLoginTestHandlers(t)
	h.cfVerifier = cfaccess.NewWithEndpoint(certsURL, issuer, cfAccessTestAUD)
	h.cfEmailMap = map[string]string{"alice@example.com": "alice"}

	token := signCFAccessTestToken(t, key, "kid-1", issuer, cfAccessTestAUD, "stranger@example.com")
	rec := doCFAccess(h, token)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("CFAccess with an unmapped email: expected 403, got %d", rec.Code)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error != "unknown_identity" {
		t.Fatalf("error body: expected unknown_identity, got %q", body.Error)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("CFAccess with an unmapped email set cookies: %v", rec.Result().Cookies())
	}
}

func TestCFAccess_MappedEmailAnswers200WithBothCookies(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	certsURL, issuer, teardown := newCFAccessTestServer(t, key, "kid-1")
	defer teardown()

	h, storage := newLoginTestHandlers(t)
	username := "cf-access-" + uuid.NewString()
	createLoginTestUser(t, storage, username, "unused-password")
	h.cfVerifier = cfaccess.NewWithEndpoint(certsURL, issuer, cfAccessTestAUD)
	h.cfEmailMap = map[string]string{"alice@example.com": username}

	token := signCFAccessTestToken(t, key, "kid-1", issuer, cfAccessTestAUD, "alice@example.com")
	rec := doCFAccess(h, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("CFAccess with a mapped email: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if accessCookie(t, rec) == "" {
		t.Fatal("CFAccess with a mapped email set no access cookie")
	}
	if responseCookie(t, rec, "kin_refresh") == "" {
		t.Fatal("CFAccess with a mapped email set no refresh cookie")
	}
}

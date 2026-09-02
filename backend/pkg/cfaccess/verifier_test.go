package cfaccess

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testKid = "test-kid-1"

// newTestServer serves a JWKS containing key's public half under testKid, and
// returns the Verifier pointed at it plus a teardown func. teamDomain is
// derived from the httptest server's host:port, matching how New builds the
// issuer/certsURL from a real team domain.
func newTestServer(t *testing.T, key *rsa.PrivateKey, aud string) (*Verifier, func()) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/cdn-cgi/access/certs", func(w http.ResponseWriter, r *http.Request) {
		set := jwks{Keys: []jwk{{
			Kid: testKid,
			Kty: "RSA",
			N:   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(set) //nolint:errcheck
	})
	srv := httptest.NewServer(mux)
	teamDomain := strings.TrimPrefix(srv.URL, "http://")
	v := New(teamDomain, aud)
	v.certsURL = srv.URL + "/cdn-cgi/access/certs"
	v.issuer = "https://" + teamDomain
	return v, srv.Close
}

func signToken(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func baseClaims(issuer, aud string) jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"iss":   issuer,
		"aud":   aud,
		"email": "alice@example.com",
		"sub":   "google-oauth2|12345",
		"exp":   now.Add(time.Hour).Unix(),
		"nbf":   now.Add(-time.Minute).Unix(),
		"iat":   now.Unix(),
	}
}

func TestVerifyValidToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	v, closeSrv := newTestServer(t, key, "test-aud")
	defer closeSrv()

	token := signToken(t, key, testKid, baseClaims(v.issuer, "test-aud"))
	identity, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if identity.Email != "alice@example.com" {
		t.Errorf("Email = %q, want alice@example.com", identity.Email)
	}
	if identity.Subject != "google-oauth2|12345" {
		t.Errorf("Subject = %q, want google-oauth2|12345", identity.Subject)
	}
}

func TestVerifyWrongAudience(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	v, closeSrv := newTestServer(t, key, "test-aud")
	defer closeSrv()

	token := signToken(t, key, testKid, baseClaims(v.issuer, "some-other-aud"))
	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("Verify: want error for wrong audience, got nil")
	}
}

func TestVerifyWrongIssuer(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	v, closeSrv := newTestServer(t, key, "test-aud")
	defer closeSrv()

	token := signToken(t, key, testKid, baseClaims("https://not-the-team-domain.example.com", "test-aud"))
	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("Verify: want error for wrong issuer, got nil")
	}
}

func TestVerifyExpiredToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	v, closeSrv := newTestServer(t, key, "test-aud")
	defer closeSrv()

	claims := baseClaims(v.issuer, "test-aud")
	// Well past the 60s leeway, so this is a genuine expiry rather than an
	// edge case the leeway is meant to tolerate.
	claims["exp"] = time.Now().Add(-5 * time.Minute).Unix()
	token := signToken(t, key, testKid, claims)
	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("Verify: want error for expired token, got nil")
	}
}

func TestVerifyAlgNoneRejected(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	v, closeSrv := newTestServer(t, key, "test-aud")
	defer closeSrv()

	token := jwt.NewWithClaims(jwt.SigningMethodNone, baseClaims(v.issuer, "test-aud"))
	token.Header["kid"] = testKid
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none token: %v", err)
	}
	if _, err := v.Verify(context.Background(), signed); err == nil {
		t.Fatal("Verify: want error for alg:none token, got nil")
	}
}

func TestVerifyUnknownKid(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	v, closeSrv := newTestServer(t, key, "test-aud")
	defer closeSrv()

	token := signToken(t, key, "some-other-kid", baseClaims(v.issuer, "test-aud"))
	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("Verify: want error for unknown kid, got nil")
	}
}

func TestVerifyUnreachableJWKS(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	v := New("cf-access-test-unreachable.invalid", "test-aud")
	v.httpClient.Timeout = time.Second

	token := signToken(t, key, testKid, baseClaims(v.issuer, "test-aud"))
	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("Verify: want error for unreachable JWKS endpoint, got nil")
	}
}

// Package cfaccess verifies a Cloudflare Access JWT assertion — the
// Cf-Access-Jwt-Assertion header Cloudflare's tunnel attaches to every
// request once its Access policy has approved a Google sign-in — against
// Cloudflare's own published key set.
//
// It deliberately never reads Cf-Access-Authenticated-User-Email. The
// backend is also reachable directly on the LAN, bypassing the tunnel
// entirely, so anyone who can reach that port can set any header they like.
// An unsigned header is worthless as proof of identity in that setting; the
// Cf-Access-Jwt-Assertion signature, checked here against Cloudflare's
// published keys and pinned to the configured AUD tag, is the only check a
// forged header cannot pass.
package cfaccess

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// keySetTTL is how long a fetched key set is trusted before a routine
	// refetch, independent of whether any kid has gone unrecognized.
	keySetTTL = 10 * time.Minute
	// unknownKidRefetchInterval bounds how often an unrecognized kid can
	// trigger its own refetch, so a caller sending bogus kids cannot drive
	// unbounded outbound requests to Cloudflare.
	unknownKidRefetchInterval = 1 * time.Minute
	// fetchTimeout bounds a single JWKS fetch.
	fetchTimeout = 5 * time.Second
)

// Identity is the caller identity recovered from a verified Access Assertion.
type Identity struct {
	Email   string
	Subject string
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

// Verifier verifies Access Assertions issued by one Cloudflare Access team
// domain for one AUD tag. It is safe for concurrent use.
type Verifier struct {
	issuer   string
	aud      string
	certsURL string

	httpClient *http.Client

	mu             sync.Mutex
	keys           map[string]*rsa.PublicKey
	fetchedAt      time.Time
	lastUnknownKid time.Time
}

// New returns a Verifier for the given Cloudflare Access team domain (e.g.
// "myteam.cloudflareaccess.com") and AUD tag.
func New(teamDomain, aud string) *Verifier {
	return &Verifier{
		issuer:     "https://" + teamDomain,
		aud:        aud,
		certsURL:   "https://" + teamDomain + "/cdn-cgi/access/certs",
		httpClient: &http.Client{Timeout: fetchTimeout},
	}
}

// NewWithEndpoint builds a Verifier against an explicit certsURL and issuer
// instead of deriving both from a team domain over HTTPS. New always dials
// HTTPS, which an httptest.Server cannot serve without a self-signed cert a
// caller then has to make the client trust — this exists so tests outside
// this package (e.g. the exchange endpoint in backend/pkg/server) can point
// a real Verifier at a plain-HTTP httptest.Server instead.
func NewWithEndpoint(certsURL, issuer, aud string) *Verifier {
	return &Verifier{
		issuer:     issuer,
		aud:        aud,
		certsURL:   certsURL,
		httpClient: &http.Client{Timeout: fetchTimeout},
	}
}

// Verify validates token as a Cloudflare Access Assertion: RS256 only,
// issuer equal to this Verifier's team domain, audience containing its AUD
// tag, and exp/nbf checked with 60 seconds of leeway. On success it returns
// the identity carried by the token's email and sub claims.
func (v *Verifier) Verify(ctx context.Context, token string) (Identity, error) {
	claims := jwt.MapClaims{}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.aud),
		jwt.WithLeeway(60*time.Second),
	)
	_, err := parser.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("token has no kid")
		}
		return v.key(ctx, kid)
	})
	if err != nil {
		return Identity{}, fmt.Errorf("verify access assertion: %w", err)
	}

	email, _ := claims["email"].(string)
	if email == "" {
		return Identity{}, fmt.Errorf("access assertion has no email claim")
	}
	subject, _ := claims["sub"].(string)
	return Identity{Email: email, Subject: subject}, nil
}

// key returns the RSA public key for kid, fetching (or refetching) the key
// set as needed. A stale cache always triggers a refetch; a cache that is
// still fresh but simply lacks kid refetches at most once a minute, per
// unknownKidRefetchInterval.
//
// That once-a-minute cap is enforced by check-and-claim: the goroutine that
// passes the check stamps lastUnknownKid before it releases the mutex, so a
// concurrent goroutine holding the same unrecognized kid loses the check and
// is turned away without fetching. Claiming afterwards — once the fetch had
// already returned — left the window between the check and the stamp wide
// open, and every goroutine that arrived inside it fetched, which is exactly
// the unbounded outbound traffic the cap exists to prevent.
//
// Check-and-claim rather than single-flight because the cap is a rate limit,
// not a deduplication: the second caller must be refused, not made to wait
// for a fetch whose result is already known not to contain its kid. The
// mutex is still never held across fetchKeys, so verification of an
// already-cached kid never queues behind a network call.
//
// The claim is taken whether or not the refetch turns up kid, because it
// counts outbound requests rather than failures. A genuine key rotation is
// unaffected: Cloudflare publishes the old and new keys together, so the one
// refetch that a rotation costs caches both. The stamp is renewed after a
// refetch that still did not find kid, which is what bounds the stale-cache
// path — it takes no claim, since a stale cache had to be refetched anyway.
func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	fresh := time.Since(v.fetchedAt) < keySetTTL
	if key, ok := v.keys[kid]; ok && fresh {
		v.mu.Unlock()
		return key, nil
	}
	if fresh {
		if time.Since(v.lastUnknownKid) < unknownKidRefetchInterval {
			v.mu.Unlock()
			return nil, fmt.Errorf("unknown key id %q", kid)
		}
		v.lastUnknownKid = time.Now()
	}
	v.mu.Unlock()

	keys, err := v.fetchKeys(ctx)
	if err != nil {
		return nil, err
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	v.keys = keys
	v.fetchedAt = time.Now()
	key, ok := keys[kid]
	if !ok {
		v.lastUnknownKid = time.Now()
		return nil, fmt.Errorf("unknown key id %q", kid)
	}
	return key, nil
}

func (v *Verifier) fetchKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.certsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build jwks request: %w", err)
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch jwks: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read jwks: %w", err)
	}

	var set jwks
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, fmt.Errorf("parse jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" || k.N == "" || k.E == "" {
			continue
		}
		pub, err := rsaPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}
	return keys, nil
}

// rsaPublicKey decodes a JWK's base64url-encoded modulus (n) and exponent
// (e) into an rsa.PublicKey, per RFC 7518 §6.3.1.
func rsaPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}, nil
}

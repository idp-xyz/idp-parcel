package oidc_test

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// 发行方替身：进程内签发令牌、经 httptest 发布 JWKS（票 operator-channel/02 做什么第 3 条）。
// 它只替身发行方的两样对外可见物——签出的令牌与公钥集——不替身校验器的任何一部分。

const (
	testIssuer   = "https://id.syn.example/dex"
	testAudience = "idp-parcel-admin-web"
	testSubject  = "SYN-OPERATOR-01"
)

var testNow = time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC)

type signingKey struct {
	kid string
	alg string
	rsa *rsa.PrivateKey
	ec  *ecdsa.PrivateKey
}

var (
	rsaKeyA = sync.OnceValue(func() signingKey { return generateRSAKey("rsa-a") })
	rsaKeyB = sync.OnceValue(func() signingKey { return generateRSAKey("rsa-b") })
	ecKeyA  = sync.OnceValue(func() signingKey { return generateECKey("ec-a") })
)

func generateRSAKey(kid string) signingKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return signingKey{kid: kid, alg: "RS256", rsa: key}
}

func generateECKey(kid string) signingKey {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	return signingKey{kid: kid, alg: "ES256", ec: key}
}

func (key signingKey) jwk() map[string]any {
	if key.rsa != nil {
		return map[string]any{
			"kty": "RSA", "kid": key.kid, "use": "sig", "alg": key.alg,
			"n": b64(key.rsa.N.Bytes()),
			"e": b64(big.NewInt(int64(key.rsa.E)).Bytes()),
		}
	}
	point, err := key.ec.PublicKey.Bytes()
	if err != nil {
		panic(err)
	}
	return map[string]any{
		"kty": "EC", "kid": key.kid, "use": "sig", "alg": key.alg, "crv": "P-256",
		"x": b64(point[1:33]),
		"y": b64(point[33:]),
	}
}

func (key signingKey) header() map[string]any {
	return map[string]any{"alg": key.alg, "kid": key.kid, "typ": "JWT"}
}

func b64(raw []byte) string { return base64.RawURLEncoding.EncodeToString(raw) }

// sign 按 JWS 紧凑序列化签出令牌；header 由调用方给，才能造出「kid 指着甲钥、实为乙钥所签」这类坏令牌。
func sign(t *testing.T, key signingKey, header, claims map[string]any) string {
	t.Helper()
	encodedClaims, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return signPayload(t, key, header, encodedClaims)
}

// signRawClaims 签一段不是 JSON 对象的载荷：签名合格而内容不成形的令牌。
func signRawClaims(t *testing.T, key signingKey, payload []byte) string {
	t.Helper()
	return signPayload(t, key, key.header(), payload)
}

func signPayload(t *testing.T, key signingKey, header map[string]any, payload []byte) string {
	t.Helper()
	encodedHeader, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	input := b64(encodedHeader) + "." + b64(payload)
	digest := sha256.Sum256([]byte(input))
	var signature []byte
	if key.rsa != nil {
		signature, err = rsa.SignPKCS1v15(rand.Reader, key.rsa, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatal(err)
		}
	} else {
		r, s, err := ecdsa.Sign(rand.Reader, key.ec, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		signature = make([]byte, 64)
		r.FillBytes(signature[:32])
		s.FillBytes(signature[32:])
	}
	return input + "." + b64(signature)
}

// signHS256WithPublishedRSAKey 造算法混淆攻击的令牌：把公开的 RSA 公钥当 HMAC 密钥签 HS256。
// 校验方若按令牌头部挑算法、再拿同一把公钥去验，这一枚就会过。
func signHS256WithPublishedRSAKey(t *testing.T, key signingKey, claims map[string]any) string {
	t.Helper()
	encodedHeader, err := json.Marshal(map[string]any{"alg": "HS256", "kid": key.kid, "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	encodedClaims, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	input := b64(encodedHeader) + "." + b64(encodedClaims)
	mac := hmac.New(sha256.New, key.rsa.N.Bytes())
	mac.Write([]byte(input))
	return input + "." + b64(mac.Sum(nil))
}

func qualifiedClaims() map[string]any {
	return map[string]any{
		"iss": testIssuer,
		"aud": testAudience,
		"sub": testSubject,
		"iat": testNow.Add(-time.Minute).Unix(),
		"exp": testNow.Add(10 * time.Minute).Unix(),
	}
}

type keySetDouble struct {
	mu      sync.Mutex
	keys    []signingKey
	failing bool
	junk    bool
	fetches int
	server  *httptest.Server
}

func newKeySetDouble(t *testing.T, keys ...signingKey) *keySetDouble {
	t.Helper()
	double := &keySetDouble{keys: keys}
	double.server = httptest.NewServer(http.HandlerFunc(double.serve))
	t.Cleanup(double.server.Close)
	return double
}

func (double *keySetDouble) serve(writer http.ResponseWriter, _ *http.Request) {
	double.mu.Lock()
	defer double.mu.Unlock()
	double.fetches++
	if double.failing {
		http.Error(writer, "issuer unavailable", http.StatusServiceUnavailable)
		return
	}
	if double.junk {
		_, _ = writer.Write([]byte("<html>maintenance</html>"))
		return
	}
	published := make([]map[string]any, 0, len(double.keys))
	for _, key := range double.keys {
		published = append(published, key.jwk())
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"keys": published})
}

func (double *keySetDouble) publish(keys ...signingKey) {
	double.mu.Lock()
	defer double.mu.Unlock()
	double.keys = keys
}

func (double *keySetDouble) fail() {
	double.mu.Lock()
	defer double.mu.Unlock()
	double.failing = true
}

func (double *keySetDouble) serveJunk() {
	double.mu.Lock()
	defer double.mu.Unlock()
	double.junk = true
}

func (double *keySetDouble) fetchCount() int {
	double.mu.Lock()
	defer double.mu.Unlock()
	return double.fetches
}

func (double *keySetDouble) url() string { return double.server.URL + "/keys" }

package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// maxKeySetBytes 给公钥集的读取设上限：一个发行方公布的钥以个位数计，一兆字节已宽出几个量级，
// 设限只为不让一个坏掉的端点把整段响应读进内存。
const maxKeySetBytes = 1 << 20

// minRSAModulusBits 是肯信的 RSA 钥下限，取 NIST SP 800-57 对当下签名钥的最低要求。
const minRSAModulusBits = 2048

func unavailable(cause error) error {
	return fmt.Errorf("%w: %w", accessidentity.ErrCredentialVerifierUnavailable, cause)
}

// publicKey 是发行方公钥集里一把可用于验签的钥，连同它允许的算法。
type publicKey struct {
	alg string
	rsa *rsa.PublicKey
	ec  *ecdsa.PublicKey
}

func (key publicKey) verify(signingInput string, signature []byte) bool {
	digest := sha256.Sum256([]byte(signingInput))
	switch key.alg {
	case "RS256":
		return rsa.VerifyPKCS1v15(key.rsa, crypto.SHA256, digest[:], signature) == nil
	case "ES256":
		// JWS 的 ES256 签名是定长的 r‖s 各 32 字节（RFC 7518 第 3.4 节），不是 ASN.1。
		if len(signature) != 64 {
			return false
		}
		r := new(big.Int).SetBytes(signature[:32])
		s := new(big.Int).SetBytes(signature[32:])
		return ecdsa.Verify(key.ec, digest[:], r, s)
	}
	return false
}

// keySet 是取回的一份公钥集。
type keySet struct {
	byID map[string]publicKey
	all  []publicKey
}

// find 按 kid 取钥。令牌不带 kid 时只在公钥集恰有一把钥时认它：OIDC Core 第 10.1 节只在单钥时
// 允许省略 kid，多钥时省略就说不清是哪一把，拿每一把都试一遍等于替出示方补了它没给的信息。
func (set keySet) find(kid string) (publicKey, bool) {
	if kid != "" {
		key, found := set.byID[kid]
		return key, found
	}
	if len(set.all) == 1 {
		return set.all[0], true
	}
	return publicKey{}, false
}

type jsonWebKey struct {
	KeyType string `json:"kty"`
	KeyID   string `json:"kid"`
	Use     string `json:"use"`
	Alg     string `json:"alg"`
	N       string `json:"n"`
	E       string `json:"e"`
	Curve   string `json:"crv"`
	X       string `json:"x"`
	Y       string `json:"y"`
}

// fetchKeys 取发行方的公钥集。取不回、状态不对与内容不成形都答依赖故障：那是发行方那头的事，
// 与出示的令牌无关。
func (verifier *Verifier) fetchKeys(ctx context.Context) (keySet, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, verifier.config.JWKSURL, nil)
	if err != nil {
		return keySet{}, unavailable(err)
	}
	response, err := verifier.client.Do(request)
	if err != nil {
		return keySet{}, unavailable(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return keySet{}, unavailable(fmt.Errorf("JWKS endpoint answered status %d", response.StatusCode))
	}
	var document struct {
		Keys []jsonWebKey `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxKeySetBytes)).Decode(&document); err != nil {
		return keySet{}, unavailable(fmt.Errorf("JWKS is not a well-formed key set: %w", err))
	}
	set := keySet{byID: make(map[string]publicKey, len(document.Keys))}
	for _, published := range document.Keys {
		key, usable := published.signatureKey()
		if !usable {
			continue
		}
		set.all = append(set.all, key)
		if published.KeyID != "" {
			set.byID[published.KeyID] = key
		}
	}
	return set, nil
}

// signatureKey 把一把公布的钥转成验签钥。认不得或不够强的钥跳过而不是让整份公钥集作废：
// 发行方同时公布加密钥或别的算法的钥是正常态，不该因此让所有令牌都验不了。
func (published jsonWebKey) signatureKey() (publicKey, bool) {
	if published.Use != "" && published.Use != "sig" {
		return publicKey{}, false
	}
	switch published.KeyType {
	case "RSA":
		if published.Alg != "" && published.Alg != "RS256" {
			return publicKey{}, false
		}
		modulus, err := base64.RawURLEncoding.DecodeString(published.N)
		if err != nil {
			return publicKey{}, false
		}
		exponent, err := base64.RawURLEncoding.DecodeString(published.E)
		if err != nil || len(exponent) == 0 || len(exponent) > 4 {
			return publicKey{}, false
		}
		n := new(big.Int).SetBytes(modulus)
		e := new(big.Int).SetBytes(exponent).Int64()
		if n.BitLen() < minRSAModulusBits || e < 3 || e%2 == 0 {
			return publicKey{}, false
		}
		return publicKey{alg: "RS256", rsa: &rsa.PublicKey{N: n, E: int(e)}}, true
	case "EC":
		if published.Curve != "P-256" || (published.Alg != "" && published.Alg != "ES256") {
			return publicKey{}, false
		}
		x, errX := base64.RawURLEncoding.DecodeString(published.X)
		y, errY := base64.RawURLEncoding.DecodeString(published.Y)
		if errX != nil || errY != nil || len(x) != 32 || len(y) != 32 {
			return publicKey{}, false
		}
		// 未压缩点编码 0x04‖x‖y；解析时顺带核对点在曲线上，不在的钥不认。
		point := append(append([]byte{0x04}, x...), y...)
		key, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), point)
		if err != nil {
			return publicKey{}, false
		}
		return publicKey{alg: "ES256", ec: key}, true
	}
	return publicKey{}, false
}

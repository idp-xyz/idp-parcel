// Package oidc 是操作者渠道凭据校验的生产实现（ADR-0100 决定二）：parcel-api 自己按发行方的
// JWKS 校验令牌的签名、iss、aud 与有效期，交回经核验的操作者主体。
package oidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// ErrInvalidConfig 表示部署参数缺件或不成形，校验器建不成。
var ErrInvalidConfig = errors.New("oidc: operator channel issuer parameters are incomplete or malformed")

// Config 是操作者渠道信任锚的三件部署形态参数（ADR-0100 决定二第一条）。三件全部必填、
// 不给默认值：发行方与受众属部署，缺省任何一个都等于替部署方选了一个信任锚。
type Config struct {
	// Issuer 是发行方标识，与令牌的 iss 逐字比较。
	Issuer string
	// JWKSURL 是发行方公钥集的地址。它单列而不由 Issuer 推出：不做 OIDC 发现，于是校验路径上
	// 只有这一处出网，发行方的发现文档写错也拖不垮它。
	JWKSURL string
	// Audience 是本部署在发行方那里的受众（管理台 SPA 的客户端标识），令牌的 aud 须含它。
	Audience string
}

func (config Config) validate() error {
	if err := absoluteHTTPURL(config.Issuer); err != nil {
		return fmt.Errorf("%w: issuer %w", ErrInvalidConfig, err)
	}
	if parsed, _ := url.Parse(config.Issuer); parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%w: issuer must not carry a query or fragment (OIDC Core 1.0 section 2)", ErrInvalidConfig)
	}
	if err := absoluteHTTPURL(config.JWKSURL); err != nil {
		return fmt.Errorf("%w: JWKS URL %w", ErrInvalidConfig, err)
	}
	if strings.TrimSpace(config.Audience) == "" {
		return fmt.Errorf("%w: audience is absent", ErrInvalidConfig)
	}
	return nil
}

// absoluteHTTPURL 只要求 http 或 https 的绝对地址，不强求 https：演示环境里发行方在同一个
// compose 网络内按服务名互访，走不了公网证书；生产用 https 属部署方的事，不在这里替它判。
func absoluteHTTPURL(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("is absent")
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("%q is not an absolute http(s) URL", value)
	}
	return nil
}

// minRefetchInterval 是两次向发行方取公钥集之间的最短间隔。缓存里没有令牌所指的那把钥时才重取，
// 间隔挡住的是拿随机 kid 逼每个请求都去打发行方的那一种；取失败同样计入间隔，发行方宕着的时候
// 每个请求立刻答依赖故障，而不是各自排队等一次超时。代价是发行方恢复或换钥后，最迟晚这一个
// 间隔才认得新钥。
const minRefetchInterval = time.Minute

// Verifier 核验操作者令牌。
type Verifier struct {
	config Config
	client *http.Client
	now    func() time.Time

	// fetching 让同一时刻只有一个请求在向发行方取钥，其余的等它取回后直接查缓存。
	fetching sync.Mutex

	mu            sync.RWMutex
	keys          keySet
	lastAttemptAt time.Time
	lastAttemptOK bool
	lastErr       error
}

var _ accessidentity.OperatorCredentialVerifier = (*Verifier)(nil)

// NewVerifier 建校验器。client 的超时由装配方定；now 是校验有效期所用的钟。
func NewVerifier(config Config, client *http.Client, now func() time.Time) (*Verifier, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	if client == nil || now == nil {
		return nil, errors.New("oidc: verifier needs an HTTP client and a clock")
	}
	return &Verifier{config: config, client: client, now: now}, nil
}

type header struct {
	Algorithm string          `json:"alg"`
	KeyID     string          `json:"kid"`
	Critical  json.RawMessage `json:"crit"`
}

// acceptedAlgorithms 是本校验器认的签名算法：RS256 是 OIDC Core 要求每个发行方都支持的那一种，
// ES256 是常见的第二种。对称算法（HS*）与 none 一律不认——令牌头部由出示方写，算法若按头部
// 挑，拿公开的 RSA 公钥当 HMAC 密钥签的令牌就能过。
var acceptedAlgorithms = []string{"RS256", "ES256"}

// notBeforeLeeway 只放宽 nbf 一侧的时钟偏差，与 go-oidc、Azure 身份库同取五分钟；exp 一侧不放宽，
// 过了就是过了——放宽 exp 等于替发行方延长它签出的有效期。
const notBeforeLeeway = 5 * time.Minute

type claims struct {
	Issuer    string       `json:"iss"`
	Subject   string       `json:"sub"`
	Audience  audience     `json:"aud"`
	Expiry    *numericDate `json:"exp"`
	NotBefore *numericDate `json:"nbf"`
}

// numericDate 是 RFC 7519 第 2 节的 NumericDate：自纪元起的秒数，可带小数；写成串的不认。
type numericDate struct{ at time.Time }

func (value *numericDate) UnmarshalJSON(raw []byte) error {
	var seconds float64
	if err := json.Unmarshal(raw, &seconds); err != nil {
		return err
	}
	whole, fraction := math.Modf(seconds)
	value.at = time.Unix(int64(whole), int64(fraction*float64(time.Second))).UTC()
	return nil
}

// audience 按 RFC 7519 第 4.1.3 节可以是单个串，也可以是串的数组；两种写法同义。
type audience []string

func (value *audience) UnmarshalJSON(raw []byte) error {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		*value = audience{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return err
	}
	*value = many
	return nil
}

// VerifyOperatorCredential 先验签名、后读声明：签名过之前，载荷里的 iss、aud、exp 一个字都不信，
// 否则一枚伪造令牌能借「过期」「受众不符」这类拒因探出本部署认什么。
func (verifier *Verifier) VerifyOperatorCredential(
	ctx context.Context,
	credential accessidentity.OperatorCredential,
) (accessidentity.OperatorSubject, error) {
	raw := credential.Token()
	if raw == "" {
		return accessidentity.OperatorSubject{}, rejected("token is absent")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return accessidentity.OperatorSubject{}, rejected("token is not a JWS compact serialization")
	}
	var decodedHeader header
	if err := decodeSegment(parts[0], &decodedHeader); err != nil {
		return accessidentity.OperatorSubject{}, rejected("header is not a well-formed JSON object")
	}
	if !slices.Contains(acceptedAlgorithms, decodedHeader.Algorithm) {
		return accessidentity.OperatorSubject{}, rejected("alg is not an accepted asymmetric algorithm")
	}
	// RFC 7515 第 4.1.11 节：crit 列出的扩展收方不认得就必须拒；本校验器一个扩展也不认。
	if len(decodedHeader.Critical) > 0 {
		return accessidentity.OperatorSubject{}, rejected("header lists critical extensions")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return accessidentity.OperatorSubject{}, rejected("signature is not base64url")
	}
	key, found, err := verifier.keyFor(ctx, decodedHeader.KeyID)
	if err != nil {
		return accessidentity.OperatorSubject{}, err
	}
	// 算法由钥定，不由头部定：头部的 alg 只用来核对它与钥自己的算法一致。
	if !found || key.alg != decodedHeader.Algorithm || !key.verify(parts[0]+"."+parts[1], signature) {
		return accessidentity.OperatorSubject{}, rejected("signature does not verify against the issuer's key set")
	}
	var decoded claims
	if err := decodeSegment(parts[1], &decoded); err != nil {
		return accessidentity.OperatorSubject{}, rejected("claims are not a well-formed JSON object")
	}
	// iss 逐字比较，不折叠末尾斜杠或大小写：OIDC Core 要求发行方标识精确匹配，而操作者主体的
	// 身份就是（发行方、sub）这一对，放宽比较等于让两个发行方的同一个 sub 撞成一个人。
	if decoded.Issuer != verifier.config.Issuer {
		return accessidentity.OperatorSubject{}, rejected("iss is not this deployment's issuer")
	}
	if !slices.Contains(decoded.Audience, verifier.config.Audience) {
		return accessidentity.OperatorSubject{}, rejected("aud does not name this deployment")
	}
	now := verifier.now()
	if decoded.Expiry == nil || !now.Before(decoded.Expiry.at) {
		return accessidentity.OperatorSubject{}, rejected("token has expired or carries no exp")
	}
	if decoded.NotBefore != nil && now.Add(notBeforeLeeway).Before(decoded.NotBefore.at) {
		return accessidentity.OperatorSubject{}, rejected("token is not valid yet")
	}
	if strings.TrimSpace(decoded.Subject) == "" {
		return accessidentity.OperatorSubject{}, rejected("sub is absent")
	}
	return accessidentity.NewOperatorSubject(verifier.config.Issuer, decoded.Subject)
}

// keyFor 取 kid 所指的验签钥：缓存里有就用；没有且上次取已过 minRefetchInterval（或从未取过），
// 才向发行方重取一次。发行方宕着时缓存里已有的钥照常能验。
func (verifier *Verifier) keyFor(ctx context.Context, kid string) (publicKey, bool, error) {
	if key, found, settled, err := verifier.cached(kid); found || settled {
		return key, found, err
	}
	verifier.fetching.Lock()
	defer verifier.fetching.Unlock()
	// 等取锁期间别人可能刚取回，再看一眼缓存。
	if key, found, settled, err := verifier.cached(kid); found || settled {
		return key, found, err
	}

	set, err := verifier.fetchKeys(ctx)
	verifier.mu.Lock()
	defer verifier.mu.Unlock()
	verifier.lastAttemptAt = verifier.now()
	verifier.lastAttemptOK = err == nil
	verifier.lastErr = err
	if err != nil {
		// 取失败不清空旧钥：发行方一时答不上来，不等于它此前公布的钥作废了。
		return publicKey{}, false, err
	}
	verifier.keys = set
	key, found := set.find(kid)
	return key, found, nil
}

// cached 查缓存。settled 为真表示不必为这个 kid 再去发行方：离上次取还不到间隔，上次取成功就
// 答「公钥集里没有这把钥」，上次取失败就原样答那次的依赖故障。
func (verifier *Verifier) cached(kid string) (key publicKey, found, settled bool, err error) {
	verifier.mu.RLock()
	defer verifier.mu.RUnlock()
	if key, found := verifier.keys.find(kid); found {
		return key, true, true, nil
	}
	if verifier.lastAttemptAt.IsZero() || verifier.now().Sub(verifier.lastAttemptAt) >= minRefetchInterval {
		return publicKey{}, false, false, nil
	}
	if verifier.lastAttemptOK {
		return publicKey{}, false, true, nil
	}
	return publicKey{}, false, true, verifier.lastErr
}

// rejected 把拒因包进 ErrCredentialRejected。拒因只给服务端诊断用：答复代数对外只有「令牌不过」
// 一格，不说哪一项不对（ADR-0100 决定四照 ADR-0055 决定四的探针同答约束）。
func rejected(reason string) error {
	return fmt.Errorf("%w: %s", accessidentity.ErrCredentialRejected, reason)
}

func decodeSegment(segment string, into any) error {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, into)
}

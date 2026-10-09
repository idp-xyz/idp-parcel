package oidc

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// ClientVerifier 核验集成客户端经 OAuth 2.0 客户端凭据许可取得的访问令牌（ADR-0149 决定三；票 operator-channel/11）。
// 签名、iss、aud 与有效期与操作者族是同一段核验；之后核 RFC 8705 的证书绑定，交回（发行方、sub）为客户端主体。
type ClientVerifier struct {
	tokens *issuerTokens
}

var _ accessidentity.IntegrationClientCredentialVerifier = (*ClientVerifier)(nil)

// NewClientVerifier 建集成客户端族的校验器。它与操作者族的校验器各用一组部署参数，至少受众不同（Config.Audience 头注）。
func NewClientVerifier(config Config, client *http.Client, now func() time.Time) (*ClientVerifier, error) {
	tokens, err := newIssuerTokens(config, client, now)
	if err != nil {
		return nil, err
	}
	return &ClientVerifier{tokens: tokens}, nil
}

// VerifyIntegrationClientCredential 校验令牌，再核它的证书绑定。
func (verifier *ClientVerifier) VerifyIntegrationClientCredential(
	ctx context.Context,
	credential accessidentity.IntegrationClientCredential,
) (accessidentity.VerifiedClientToken, error) {
	verified, err := verifier.tokens.verify(ctx, credential.Token())
	if err != nil {
		return accessidentity.VerifiedClientToken{}, err
	}
	bound, err := certificateBinding(verified.Confirmation, credential.Certificate())
	if err != nil {
		return accessidentity.VerifiedClientToken{}, err
	}
	subject, err := accessidentity.NewIntegrationClientSubject(verifier.tokens.config.Issuer, verified.Subject)
	if err != nil {
		return accessidentity.VerifiedClientToken{}, err
	}
	return accessidentity.NewVerifiedClientToken(subject, bound), nil
}

// certificateThumbprintMethod 是 RFC 8705 第 3.1 节的确认方法名：证书 DER 的 SHA-256，base64url 不带填充。
const certificateThumbprintMethod = "x5t#S256"

// certificateBinding 核令牌的 cnf。缺席是持有者令牌，答未绑定。只带 x5t#S256 时，这次连接出示的证书摘要须与之相等，
// 答已绑定。其余一律拒：带了本校验器验不了的确认方法（如 DPoP 的 jkt）的令牌要求出示方证明持有另一把钥，当持有者令牌
// 收就丢了那道持有证明；cnf 写成 null 或空对象也拒，它在声称绑定却说不出绑在什么上。
func certificateBinding(confirmation json.RawMessage, certificate []byte) (bool, error) {
	if len(confirmation) == 0 {
		return false, nil
	}
	var methods map[string]json.RawMessage
	if err := json.Unmarshal(confirmation, &methods); err != nil {
		return false, rejected("cnf is not a JSON object")
	}
	if len(methods) == 0 {
		return false, rejected("cnf names no confirmation method")
	}
	for method := range methods {
		if method != certificateThumbprintMethod {
			return false, rejected("cnf carries a confirmation method this verifier cannot check")
		}
	}
	var encoded string
	if err := json.Unmarshal(methods[certificateThumbprintMethod], &encoded); err != nil {
		return false, rejected("x5t#S256 is not a string")
	}
	expected, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(expected) != sha256.Size {
		return false, rejected("x5t#S256 is not a base64url SHA-256 digest")
	}
	if len(certificate) == 0 {
		return false, rejected("token is certificate-bound but no client certificate was presented")
	}
	presented := sha256.Sum256(certificate)
	if subtle.ConstantTimeCompare(presented[:], expected) != 1 {
		return false, rejected("presented certificate is not the one the token is bound to")
	}
	return true, nil
}

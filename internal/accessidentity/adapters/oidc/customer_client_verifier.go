package oidc

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// CustomerClientVerifier 核验首方客户族经 OAuth 2.0 客户端凭据许可取得的访问令牌（ADR-0139 决定二）。
//
// 核验整段就是 ClientVerifier：签名、iss、aud、有效期，再核 RFC 8705 的证书绑定——首方族复用集成客户端族的凭据机制，
// 不另写一遍。分出这个类型只为两件事：交回的主体是 CustomerClientSubject，以及装配点把集成客户端族那一只（受众不同）
// 接到首方铸造器上时编译不过。
type CustomerClientVerifier struct {
	client *ClientVerifier
}

var _ accessidentity.CustomerClientCredentialVerifier = (*CustomerClientVerifier)(nil)

// NewCustomerClientVerifier 建首方族的校验器。它用首方族自己那组部署参数，受众与操作者族、集成客户端族不同由装配点核。
func NewCustomerClientVerifier(config Config, client *http.Client, now func() time.Time) (*CustomerClientVerifier, error) {
	verifier, err := NewClientVerifier(config, client, now)
	if err != nil {
		return nil, err
	}
	return &CustomerClientVerifier{client: verifier}, nil
}

// VerifyCustomerClientCredential 校验令牌、核证书绑定，交回（发行方、sub）为首方客户端主体。
func (verifier *CustomerClientVerifier) VerifyCustomerClientCredential(
	ctx context.Context,
	credential accessidentity.IntegrationClientCredential,
) (accessidentity.VerifiedCustomerClientToken, error) {
	verified, err := verifier.client.VerifyIntegrationClientCredential(ctx, credential)
	if err != nil {
		return accessidentity.VerifiedCustomerClientToken{}, err
	}
	subject, err := accessidentity.NewCustomerClientSubject(verified.Subject().Issuer(), verified.Subject().Subject())
	if err != nil {
		return accessidentity.VerifiedCustomerClientToken{}, err
	}
	return accessidentity.NewVerifiedCustomerClientToken(subject, verified.CertificateBound()), nil
}

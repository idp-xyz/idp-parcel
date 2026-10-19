package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/accessidentity/adapters/oidc"
	psaccess "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 首方客户族信任锚的三件部署形态参数（ADR-0139 决定二，形状同集成客户端族那三件）。全部必填、不给默认值；受众必须与操作者
// 族、集成客户端族的都不同。
const (
	customerAPIIssuerEnv   = "IDP_PARCEL_CUSTOMER_API_OIDC_ISSUER"
	customerAPIJWKSURLEnv  = "IDP_PARCEL_CUSTOMER_API_OIDC_JWKS_URL"
	customerAPIAudienceEnv = "IDP_PARCEL_CUSTOMER_API_OIDC_AUDIENCE"
)

const customerAPIKeySetFetchTimeout = 5 * time.Second

// buildCustomerClientCredentialVerifier 解析首方客户族的发行方参数。三态同另两族：三件都未设 → 未配置核验方，整族答
// ACCESS_CHANNEL_NOT_CONFIGURED；设了一部分或值不成形 → 进程启动即拒；三件齐且成形 → OIDC 校验器。
//
// 另加一条两族没有的：受众与操作者族、集成客户端族那一件相同即启动拒。三族可以落在同一个发行方实例上，那时一枚令牌能过
// 哪一族的校验只看受众；受众撞了，一个集成客户端的令牌就过得了首方族的校验期，「不得互相顶替」就只剩册这一道（ADR-0139
// 决定三）。只比已设的那几件：另两族没配时无从撞。
func buildCustomerClientCredentialVerifier(getenv func(string) string) (accessidentity.CustomerClientCredentialVerifier, bool, error) {
	config := oidc.Config{
		Issuer:   strings.TrimSpace(getenv(customerAPIIssuerEnv)),
		JWKSURL:  strings.TrimSpace(getenv(customerAPIJWKSURLEnv)),
		Audience: strings.TrimSpace(getenv(customerAPIAudienceEnv)),
	}
	if config.Issuer == "" && config.JWKSURL == "" && config.Audience == "" {
		return accessidentity.UnconfiguredCustomerClientCredentialVerifier{}, false, nil
	}
	for _, other := range []string{operatorAudienceEnv, integrationClientAudienceEnv} {
		if config.Audience != "" && strings.TrimSpace(getenv(other)) == config.Audience {
			return nil, false, fmt.Errorf(
				"%s=%q equals %s: the first-party customer family needs an audience of its own, or another family's tokens pass its verification (ADR-0139); refusing to start",
				customerAPIAudienceEnv, config.Audience, other,
			)
		}
	}
	verifier, err := oidc.NewCustomerClientVerifier(config, &http.Client{Timeout: customerAPIKeySetFetchTimeout}, time.Now)
	if err != nil {
		return nil, false, fmt.Errorf(
			"first-party customer channel parameters %s, %s and %s must be set together and well-formed (ADR-0139); refusing to start: %w",
			customerAPIIssuerEnv, customerAPIJWKSURLEnv, customerAPIAudienceEnv, err,
		)
	}
	return verifier, true, nil
}

// buildCustomerClientMinter 建本进程唯一一只首方铸造器。准入范围与另两族同一只（buildAdmissionScope，ADR-0180）：今天换上
// 的提交口不在铸造侧判它，余下各口换上来时判。发行方参数没设时核验方就是未配置那一只。
func buildCustomerClientMinter(
	verifier accessidentity.CustomerClientCredentialVerifier,
	registry accessidentity.CustomerClientRegistry,
	admission accessidentity.AdmissionScope,
) (*accessidentity.CustomerClientMinter, error) {
	if verifier == nil || registry == nil || admission == nil {
		return nil, fmt.Errorf("customer client minter needs a verifier, a customer client registry and an admission scope")
	}
	return accessidentity.NewCustomerClientMinter(verifier, registry, admission, time.Now)
}

// buildCustomerSubmissionIntake 装配提交口的首方 Intake（ADR-0139 决定四；票 first-party-api/00）。ownership 要与提交编排
// 那一只读同一本登记册、同一组坐标（buildProductionOwnership 头注）：Intake 预取的期望修订由编排在提交时点重判。
func buildCustomerSubmissionIntake(
	minter *accessidentity.CustomerClientMinter,
	ownership ports.ProductionOwnershipAuthority,
) (shipmenthttp.SubmissionIntake, error) {
	if minter == nil {
		return nil, fmt.Errorf("customer submission intake needs a minter")
	}
	authenticator, err := psaccess.NewCustomerClientAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	intake, err := shipmenthttp.NewCustomerSubmissionIntake(authenticator, ownership, systemClock{})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: customer submission intake: %w", err)
	}
	return intake, nil
}

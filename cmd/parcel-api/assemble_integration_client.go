package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/accessidentity/adapters/oidc"
	ccaccess "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	saaccess "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/accessidentity"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
)

// 集成客户端族信任锚的三件部署形态参数（ADR-0149 决定三，方式同 ADR-0100 决定二）。与操作者族分开：至少受众不同，
// 一族的令牌过不了另一族的校验。全部必填、不给默认值。
const (
	integrationClientIssuerEnv   = "IDP_PARCEL_INTEGRATION_CLIENT_OIDC_ISSUER"
	integrationClientJWKSURLEnv  = "IDP_PARCEL_INTEGRATION_CLIENT_OIDC_JWKS_URL"
	integrationClientAudienceEnv = "IDP_PARCEL_INTEGRATION_CLIENT_OIDC_AUDIENCE"
)

const integrationClientKeySetFetchTimeout = 5 * time.Second

// integrationClientIntakes 是集成客户端族在外部结果、监管凭证与外部资金事实（含更正）上的 Intake。
type integrationClientIntakes struct {
	customs    *customshttp.IntegrationClientIntake
	settlement *settlementhttp.IntegrationClientIntake
}

// buildIntegrationClientCredentialVerifier 解析集成客户端族的发行方参数。三态同操作者族：三件都未设 → 未配置核验方；
// 设了一部分或值不成形 → 进程启动即拒；三件齐且成形 → OIDC 校验器。
func buildIntegrationClientCredentialVerifier(getenv func(string) string) (accessidentity.IntegrationClientCredentialVerifier, bool, error) {
	config := oidc.Config{
		Issuer:   strings.TrimSpace(getenv(integrationClientIssuerEnv)),
		JWKSURL:  strings.TrimSpace(getenv(integrationClientJWKSURLEnv)),
		Audience: strings.TrimSpace(getenv(integrationClientAudienceEnv)),
	}
	if config.Issuer == "" && config.JWKSURL == "" && config.Audience == "" {
		return accessidentity.UnconfiguredIntegrationClientCredentialVerifier{}, false, nil
	}
	verifier, err := oidc.NewClientVerifier(config, &http.Client{Timeout: integrationClientKeySetFetchTimeout}, time.Now)
	if err != nil {
		return nil, false, fmt.Errorf(
			"integration client channel parameters %s, %s and %s must be set together and well-formed (ADR-0149); refusing to start: %w",
			integrationClientIssuerEnv, integrationClientJWKSURLEnv, integrationClientAudienceEnv, err,
		)
	}
	return verifier, true, nil
}

// buildIntegrationClientMinter 建本进程唯一一只集成客户端铸造器。准入范围取 UnconfiguredAdmissionScope：租户到治理坐标的
// 对照还没有任何租户登过，一切生产写答「不在准入范围」（ADR-0149 越权风险点 3）。发行方参数没设时核验方就是未配置那一只。
func buildIntegrationClientMinter(
	verifier accessidentity.IntegrationClientCredentialVerifier,
	registry accessidentity.IntegrationClientRegistry,
) (*accessidentity.IntegrationClientMinter, error) {
	if verifier == nil || registry == nil {
		return nil, fmt.Errorf("integration client minter needs a verifier and a client registry")
	}
	return accessidentity.NewIntegrationClientMinter(verifier, registry, accessidentity.UnconfiguredAdmissionScope{}, time.Now)
}

func buildIntegrationClientIntakes(minter *accessidentity.IntegrationClientMinter) (integrationClientIntakes, error) {
	if minter == nil {
		return integrationClientIntakes{}, fmt.Errorf("integration client intakes need a minter")
	}
	customsAuthenticator, err := ccaccess.NewIntegrationClientAuthenticator(minter)
	if err != nil {
		return integrationClientIntakes{}, err
	}
	customs, err := customshttp.NewIntegrationClientIntake(customsAuthenticator, systemClock{})
	if err != nil {
		return integrationClientIntakes{}, err
	}
	settlementAuthenticator, err := saaccess.NewIntegrationClientAuthenticator(minter)
	if err != nil {
		return integrationClientIntakes{}, err
	}
	settlement, err := settlementhttp.NewIntegrationClientIntake(settlementAuthenticator)
	if err != nil {
		return integrationClientIntakes{}, err
	}
	return integrationClientIntakes{customs: customs, settlement: settlement}, nil
}

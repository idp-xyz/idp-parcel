package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/accessidentity/adapters/oidc"
)

// 操作者渠道信任锚的三件部署形态参数（ADR-0100 决定二第一条）：发行方、JWKS 地址、受众。
// 全部必填、不给默认值——缺省任何一件都等于替部署方选了信任锚（同 ADR-0049 对节奏参数的纪律）。
const (
	operatorIssuerEnv   = "IDP_PARCEL_OPERATOR_OIDC_ISSUER"
	operatorJWKSURLEnv  = "IDP_PARCEL_OPERATOR_OIDC_JWKS_URL"
	operatorAudienceEnv = "IDP_PARCEL_OPERATOR_OIDC_AUDIENCE"
)

// operatorKeySetFetchTimeout 是向发行方取一次公钥集的上限。它压在请求路径上——缓存里没有令牌所指
// 的钥时，那个请求要等这一取——所以取得比 http.Server 的读写时限短得多；取超时答依赖故障。
const operatorKeySetFetchTimeout = 5 * time.Second

// buildOperatorCredentialVerifier 解析操作者渠道的发行方参数，交回操作者族的核验方。
//
// 三态：三件都未设 → 未配置核验方，整族答 ACCESS_CHANNEL_NOT_CONFIGURED，configured 为假；设了
// 一部分或值不成形 → 报错，进程启动即拒——静默回落会让「配错了」与「刻意没配」两态可观察签名
// 相同（判据同隔离读开关）；三件齐且成形 → OIDC 校验器。
func buildOperatorCredentialVerifier(getenv func(string) string) (accessidentity.OperatorCredentialVerifier, bool, error) {
	config := oidc.Config{
		Issuer:   strings.TrimSpace(getenv(operatorIssuerEnv)),
		JWKSURL:  strings.TrimSpace(getenv(operatorJWKSURLEnv)),
		Audience: strings.TrimSpace(getenv(operatorAudienceEnv)),
	}
	if config.Issuer == "" && config.JWKSURL == "" && config.Audience == "" {
		return accessidentity.UnconfiguredOperatorCredentialVerifier{}, false, nil
	}
	verifier, err := oidc.NewVerifier(config, &http.Client{Timeout: operatorKeySetFetchTimeout}, time.Now)
	if err != nil {
		return nil, false, fmt.Errorf(
			"operator channel parameters %s, %s and %s must be set together and well-formed (ADR-0100); refusing to start: %w",
			operatorIssuerEnv, operatorJWKSURLEnv, operatorAudienceEnv, err,
		)
	}
	return verifier, true, nil
}

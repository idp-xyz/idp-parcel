package main

import (
	"fmt"
	"strings"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipilot "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/pilotgovernance"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	pgpostgres "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	pgapplication "go.idp.xyz/idp-parcel/internal/pilotgovernance/application"
)

// governanceSelfAuthorityEnv 是本产品在治理登记册里的权威串（ADR-0180 决定三）：一件部署形态参数，与发行方、受众同属
// 随部署走的那一类。值是部署取值，产品不给默认；未设即未配置。准入范围桥与 parcel-shipment 生产归属同读这一处，进程里
// 只解析一次——两处各读各的，「本产品是谁」就有了两个答案（票 operator-channel/14 判断项 2）。
const governanceSelfAuthorityEnv = "IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY"

// governanceSelfAuthority 只去首尾空白，不校验形状：串的样子由部署方的登记册约定，产品无从判对错。
func governanceSelfAuthority(getenv func(string) string) string {
	return strings.TrimSpace(getenv(governanceSelfAuthorityEnv))
}

// buildAdmissionScope 建操作者与集成客户端两只铸造器共用的准入范围（ADR-0180 决定一至三；票 operator-channel/19）。
//
// 权威串未设时交回 UnconfiguredAdmissionScope，对照与区间一行都不读：各口答复与桥接上之前逐字节相同，库读不动也不会
// 把「不在准入范围」换成依赖故障。设了才接桥——读本能力的对照换坐标，再问 pilot-governance 的 AuthorityCoverage；
// 对照没登的租户照旧答不在准入范围（ADR-0149 越权风险点 3）。
func buildAdmissionScope(db *bentopg.DB, selfAuthority string) (accessidentity.AdmissionScope, error) {
	if strings.TrimSpace(selfAuthority) == "" {
		return accessidentity.UnconfiguredAdmissionScope{}, nil
	}
	intervals, err := pgpostgres.NewAuthorityIntervals(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: authority intervals for the admission scope: %w", err)
	}
	coverage, err := pgapplication.NewAuthorityCoverage(intervals, selfAuthority)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: authority coverage: %w", err)
	}
	mappings, err := aipg.NewAdmissionScopeMappings(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: admission scope mappings: %w", err)
	}
	scope, err := aipilot.NewAdmissionScope(mappings, coverage)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: admission scope bridge: %w", err)
	}
	return scope, nil
}

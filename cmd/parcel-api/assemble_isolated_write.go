package main

import (
	"fmt"
	"strings"

	pspilot "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/pilotgovernance"
)

// isolatedWriteTenantEnv 是隔离写路径准入（ADR-0091）的显式输入。它与隔离读面的开关
// 分成两个而不是合一：合一会让今天所有设了读开关的环境在升级那一刻静默获得写准入，
// 而「已存在的配置被后来的代码改宽」正是缺省朝拦要防的那件事（ADR-0091 决定四）。
const isolatedWriteTenantEnv = "IDP_PARCEL_ISOLATED_WRITE_TENANT"

// 隔离形态的合成治理坐标。它们不从租户派生：治理登记册无租户维是设计（ADR-0083），
// 开关的值在这一格只作启用凭据——与 buildIsolatedReadIntakes 对治理读面的处置同款。
//
// 四个值要与种子包里那条委托受理维的权威区间逐字对上，对不上的后果不是报错而是查不到
// 那一行，答出来的`权威未确定`与「压根没登记」一模一样。
const (
	isolatedGovernanceObjectScope = "SYN-PILOT-SCOPE/shipment-intake@v1"
	isolatedGovernanceCapability  = "SYN-CAP/shipment-intake"
	isolatedGovernanceFactKind    = "SYN-FACT/shipment-request"
	isolatedGovernancePilotScope  = "SYN-PILOT-SCOPE/shipment-intake@v1"
)

// isolatedSelfAuthority 是隔离形态下代表本产品的那个权威串。它进归属决定的修订串，
// 因此也带 `SYN-` 前缀：修订串会随决定一路传到调用方与门禁，合成来源要在那里也看得见。
const isolatedSelfAuthority = "SYN-AUTH/idp-parcel-pilot"

// isolatedWriteAdmission 携带 ADR-0091 放行的写路径那一格：委托受理的生产归属范围目录（墙二）与隔离形态的自身权威串。
// nil 表示未启用——各装配函数对 nil 的处理与本记录之前逐字节同形。
//
// 墙一（提交口的命令面 Intake）不在这里：提交口挂首方客户渠道的 Intake（ADR-0150 决定三，票 first-party-api/00），写开关
// 不放行任何命令面，也不进 assembleBusinessEndpoints。墙二的真形态是 PS 的生产归属目录，未立；在它立起之前，演示部署靠这
// 一格让提交编排读得到合成坐标，生产形态下目录照旧留空、归属答`权威未确定`。写开关随读开关在最后一口换完的那一笔一起删
// （ADR-0150 决定三），删之前要有生产目录接手这一格，否则演示提交停在 OWNERSHIP_UNRESOLVED。
type isolatedWriteAdmission struct {
	governanceDirectory pspilot.GovernanceScopeDirectory
	selfAuthority       string
}

// buildIsolatedWriteAdmission 解析隔离写路径准入的显式输入（ADR-0091 决定四）。
//
// 三态与隔离读面同款：未设 → (nil, nil)；设了但不带合成前缀 → 报错，进程启动即拒；
// 设了且合规 → 交回两格，启动日志由调用方写。另加一条读面没有的校验：两开关都设时
// 取值必须相同。开关的值只作启用凭据（ADR-0091 决定五），不进任何一格。
func buildIsolatedWriteAdmission(getenv func(string) string) (*isolatedWriteAdmission, error) {
	tenant := getenv(isolatedWriteTenantEnv)
	if tenant == "" {
		return nil, nil
	}
	syntheticMarker := syntheticIdentifierPrefix + "-"
	if !strings.HasPrefix(tenant, syntheticMarker) {
		return nil, fmt.Errorf(
			"%s=%q: isolated write admission only accepts synthetic tenants with the %q prefix (ADR-0091); refusing to start",
			isolatedWriteTenantEnv, tenant, syntheticMarker,
		)
	}
	if readTenant := getenv(isolatedReadTenantEnv); readTenant != "" && readTenant != tenant {
		return nil, fmt.Errorf(
			"%s=%q and %s=%q disagree: a request written under one tenant is invisible to a read face filtering by the other (ADR-0091); refusing to start",
			isolatedWriteTenantEnv, tenant, isolatedReadTenantEnv, readTenant,
		)
	}
	// 一个进程里不许有两个「本产品」（ADR-0180 决定三）：隔离分支的归属认 isolatedSelfAuthority，准入范围桥认部署参数，
	// 两值不等，同一个进程就会认领两种权威串的区间。隔离写开关删去的那一笔，这条检查随之删去。
	if selfAuthority := governanceSelfAuthority(getenv); selfAuthority != "" && selfAuthority != isolatedSelfAuthority {
		return nil, fmt.Errorf(
			"%s=%q and the isolated write admission's self authority %q disagree: one process cannot hold two self authorities on the governance register (ADR-0180); refusing to start",
			governanceSelfAuthorityEnv, selfAuthority, isolatedSelfAuthority,
		)
	}

	directory, err := pspilot.NewIsolatedGovernanceScopeDirectory(
		isolatedGovernanceObjectScope,
		isolatedGovernanceCapability,
		isolatedGovernanceFactKind,
		isolatedGovernancePilotScope,
	)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: isolated governance scope: %w", err)
	}
	return &isolatedWriteAdmission{
		governanceDirectory: directory,
		selfAuthority:       isolatedSelfAuthority,
	}, nil
}

package main

import (
	"fmt"
	"strings"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	pspilot "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/pilotgovernance"
	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
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

// 提交口注入的合成来源与准入范围（ADR-0091 决定二）。
//
// 客户账户**刻意复用隔离读面那一个**：写下的委托挂在哪个账户上，决定了它在委托查阅页
// 上可不可见。两处各写各的常量会长出一种「提交成功但页面查无此单」的配置，而那个症状
// 看起来像缺陷不像配置错——与两开关取值必须相同是同一条理由。
const (
	isolatedSubmissionSource       = "SYN-SOURCE/admin-web"
	isolatedSubmissionScopeRef     = "SYN-ADM-SCOPE/shipment-intake"
	isolatedSubmissionScopeDigest  = "sha256:syn-adm-scope-shipment-intake"
	isolatedSubmissionCustomerAcct = isolatedReadCustomerAccount
)

// isolatedReceptionNode 是节点收寄口注入的合成节点（票 operator-channel/08）。ReceptionIntake 的契约把节点身份与租户并列为
// 「只能来自认证结果」，隔离形态没有设备登记册可查，认证结果就是这一个合成常量——载荷里带节点即拒。取种子网络的始发枢纽，
// 让动线里的收寄落在网络定义认得的节点上；要在别的合成节点上收寄，改的是这一格，不是去采信请求。
const isolatedReceptionNode = "SYN-NODE-SHA-HUB"

// isolatedMovementSource 是移动事实口注入的合成来源（票 operator-channel/08）。MovementFactIntake 的契约把「自营还是外部」
// 交给认证结果说，隔离形态的认证结果就是这一个常量：它说自营执行方，外部承运轨迹照旧只走 TrackingSource 入站口。
// 带 `SYN-` 前缀：来源维是事后分辨隔离行的两维之一（ADR-0091 决定三）。
const isolatedMovementSource = "SYN-SOURCE/self-operated-executor"

// isolatedWriteAdmission 携带 ADR-0091 放行的写路径几格。nil 表示未启用——各装配函数
// 对 nil 的处理与本记录之前逐字节同形。
//
// 提交口的命令面 Intake（墙一）不在本结构里：它要先经库装归属权威，由 buildIsolatedSubmissionIntake
// 在开池之后构造。身份族的 Intake 在本结构里，因为它只收租户、不碰库，与目录、自身权威串同在这道门里
// 就位。命令面按端点逐口放行（Consequences）：设了写开关而某一口仍答 403，是那个分批的中间态，不是配置
// 没生效——哪几口已放行由 admittedCommandLines 说，启动日志照它出声。
type isolatedWriteAdmission struct {
	governanceDirectory pspilot.GovernanceScopeDirectory
	selfAuthority       string
	// tenant 是开关的值本身。归属那一格用不到它（治理登记册无租户维），提交口用得到：
	// 来源信封的租户维就是它。
	tenant string
	// partyIdentity 是 `/commercial-*` 身份族登记口的隔离 Intake（票 admin-web-group-legal-entities/06）。
	// 租户格填开关值，行内容只从载荷取；它实现了哪几口的 Intake 接口，装配点就换得了哪几行——编译期锁住。
	partyIdentity *commercialhttp.IsolatedPartyIdentityIntake
	// 以下各格是主链命令面各上下文的隔离命令 Intake（票 operator-channel/08），锁法同 partyIdentity。
	nodeOperations       *nodeopshttp.IsolatedCommandIntake
	transportFulfillment *tfhttp.IsolatedCommandIntake
	customs              *customshttp.IsolatedCommandIntake
	settlement           *settlementhttp.IsolatedCommandIntake
}

// isolatedWriteAdmittedCommandLines 是写开关到此刻为止换上隔离 Intake 的命令面，供启动日志出声
// （ADR-0078 决定三、ADR-0091 决定四「放行必须出声」）。每放一口在这里加一行，与 assembleBusinessEndpoints
// 里换的那几行同步——两处对不上由装配测试拦（isolated_write_test.go 的二分表）。
var isolatedWriteAdmittedCommandLines = []string{
	"/shipment-requests",
	"/commercial-legal-entity-registrations",
	"/commercial-business-party-registrations",
	"/commercial-customer-account-registrations",
	"/commercial-party-relationship-registrations",
	"/commercial-party-identity-deactivations",
	"/commercial-legal-entity-profile-registrations",
	"/node-operations/receptions",
	"/transport-fulfillment/offsite-pickups",
	"/transport-fulfillment/offsite-pickup-attempts",
	"/transport-fulfillment-carrier-first-effective-pickup-judgments",
	"/transport-fulfillment/handovers",
	"/transport-fulfillment/movement-facts",
	"/transport-fulfillment-dispatch-task-registrations",
	"/transport-fulfillment-delivery-dispatch-triggers",
	"/transport-fulfillment/deliveries",
	"/transport-fulfillment-segment-closures",
	"/transport-fulfillment-effective-time-judgments",
	"/customs/external-results",
	"/customs-regulatory-credential-registrations",
	"/settlement-external-funds-fact-registrations",
}

// admittedCommandLines 交回放行名单的副本：日志与测试都不该改得动那份表。
func (admission *isolatedWriteAdmission) admittedCommandLines() []string {
	return append([]string(nil), isolatedWriteAdmittedCommandLines...)
}

// partyIdentityIntake 对 nil 接收者交回 nil：装配点那几行随之挂字面量未配置即拒，与启用前逐字节同形。
func (admission *isolatedWriteAdmission) partyIdentityIntake() *commercialhttp.IsolatedPartyIdentityIntake {
	if admission == nil {
		return nil
	}
	return admission.partyIdentity
}

// nodeOperationsIntake 对 nil 接收者交回 nil，理由同 partyIdentityIntake。
func (admission *isolatedWriteAdmission) nodeOperationsIntake() *nodeopshttp.IsolatedCommandIntake {
	if admission == nil {
		return nil
	}
	return admission.nodeOperations
}

// transportFulfillmentIntake 对 nil 接收者交回 nil，理由同 partyIdentityIntake。
func (admission *isolatedWriteAdmission) transportFulfillmentIntake() *tfhttp.IsolatedCommandIntake {
	if admission == nil {
		return nil
	}
	return admission.transportFulfillment
}

// customsIntake 对 nil 接收者交回 nil，理由同 partyIdentityIntake。
func (admission *isolatedWriteAdmission) customsIntake() *customshttp.IsolatedCommandIntake {
	if admission == nil {
		return nil
	}
	return admission.customs
}

// settlementIntake 对 nil 接收者交回 nil，理由同 partyIdentityIntake。
func (admission *isolatedWriteAdmission) settlementIntake() *settlementhttp.IsolatedCommandIntake {
	if admission == nil {
		return nil
	}
	return admission.settlement
}

// buildIsolatedWriteAdmission 解析隔离写路径准入的显式输入（ADR-0091 决定四）。
//
// 三态与隔离读面同款：未设 → (nil, nil)；设了但不带合成前缀 → 报错，进程启动即拒；
// 设了且合规 → 交回两格，启动日志由调用方写。另加一条读面没有的校验：两开关都设时
// 取值必须相同。
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

	directory, err := pspilot.NewIsolatedGovernanceScopeDirectory(
		isolatedGovernanceObjectScope,
		isolatedGovernanceCapability,
		isolatedGovernanceFactKind,
		isolatedGovernancePilotScope,
	)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: isolated governance scope: %w", err)
	}
	partyIdentity, err := commercialhttp.NewIsolatedPartyIdentityIntake(tenant)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: isolated party identity intake: %w", err)
	}
	nodeOperations, err := nodeopshttp.NewIsolatedCommandIntake(tenant, isolatedReceptionNode)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: isolated node operations command intake: %w", err)
	}
	transportFulfillment, err := tfhttp.NewIsolatedCommandIntake(tfhttp.IsolatedCommandIntakeDeps{
		Tenant:         tenant,
		MovementSource: isolatedMovementSource,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: isolated transport fulfillment command intake: %w", err)
	}
	customs, err := customshttp.NewIsolatedCommandIntake(customshttp.IsolatedCommandIntakeDeps{Tenant: tenant, Clock: systemClock{}})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: isolated customs command intake: %w", err)
	}
	settlement, err := settlementhttp.NewIsolatedCommandIntake(tenant)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: isolated settlement command intake: %w", err)
	}
	return &isolatedWriteAdmission{
		governanceDirectory:  directory,
		selfAuthority:        isolatedSelfAuthority,
		tenant:               tenant,
		partyIdentity:        partyIdentity,
		nodeOperations:       nodeOperations,
		transportFulfillment: transportFulfillment,
		customs:              customs,
		settlement:           settlement,
	}, nil
}

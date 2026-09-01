package main

import (
	"fmt"
	"strings"

	collectionhttp "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/http"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	governancehttp "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/http"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

// isolatedReadTenantEnv 是隔离读面准入（ADR-0078）的唯一显式输入：未设即全拦，设了
// 只换运营查阅行的 Intake。按环境切换装配只此一处、只此一维（ADR-0078 Decision 四），
// 不得据此再添 demo/mode 类开关。
const isolatedReadTenantEnv = "IDP_PARCEL_ISOLATED_READ_TENANT"

// syntheticIdentifierPrefix 是证据层级 S 在代码里的锚（ADR-0078 Decision 三），与
// scripts/demo-seeds 的种子标识同一纪律。真实租户标识没有这个前缀，结构上进不了
// 这个开关。横线归拼接处（标识前缀门禁），判定时拼成 `SYN-`。
const syntheticIdentifierPrefix = "SYN"

// 注入给运营查阅作用域的合成常量。作用域引用是授权结果的审计引用，不参与行过滤，
// 值只需自证合成；页大小对演示种子的量级绰绰有余，改它不需要动 ADR。
const (
	isolatedReadScopeReference = "SYN-SCOPE-ISOLATED-READ"
	isolatedReadLimit          = 200
)

// isolatedReadCustomerAccount 是委托查阅（AuthorizedQueryScope）的可见客户账户维。
// 演示动线票的事实链种子必须用同一账户提交委托，否则灌进去的行在页面上不可见——
// 扩账户在这里加，不另设输入。
const isolatedReadCustomerAccount = "SYN-ACCOUNT-01"

// isolatedReadIntakes 携带 ADR-0078 放行面的注入式 Intake，字段逐一对应各上下文；
// parcel-pricing 与 party-commercial 各自的两条目录行共用本上下文的一个 Intake
// （「分设只会让装配点看起来能只配一半」，与各包 Intake 注释同句）。nil 指针表示
// 未启用——assembleBusinessEndpoints 对 nil 的处理与 ADR-0078 之前逐字节同形。
type isolatedReadIntakes struct {
	shipmentRequestViews    shipmenthttp.ShipmentRequestViewsIntake
	labelTransactions       shipmenthttp.LabelTransactionQueryIntake
	nodeOperationsCatalogue nodeopshttp.CatalogueQueryIntake
	transportCatalogue      tfhttp.CatalogueQueryIntake
	trackingProjections     visibilityhttp.OperationsTrackingIntake
	pricingCatalogue        pricinghttp.PricingCatalogueIntake
	networkCatalog          networkhttp.CatalogueQueryIntake
	complianceRules         customshttp.CatalogueQueryIntake
	commercialCatalogue     commercialhttp.CommercialCatalogueIntake
	collectionCatalogue     collectionhttp.CatalogueQueryIntake
	settlementCatalogue     settlementhttp.CatalogueQueryIntake
	governanceRegisters     governancehttp.RegistryQueryIntake
}

// buildIsolatedReadIntakes 解析隔离读面准入的显式输入（ADR-0078 Decision 三）。
//
// 三态：未设 → (nil, nil)，全部端点未配置即拒；设了但不带合成前缀 → 报错，进程
// 启动即拒——静默回落会让配置错误与「刻意拦着」两态可观察签名相同，那正是「默认值
// 不出声」病；设了且合规 → 交回各上下文的注入式 Intake，启动日志由调用方写。
func buildIsolatedReadIntakes(getenv func(string) string) (*isolatedReadIntakes, error) {
	tenant := getenv(isolatedReadTenantEnv)
	if tenant == "" {
		return nil, nil
	}
	syntheticMarker := syntheticIdentifierPrefix + "-"
	if !strings.HasPrefix(tenant, syntheticMarker) {
		return nil, fmt.Errorf(
			"%s=%q: isolated read admission only accepts synthetic tenants with the %q prefix (ADR-0078); refusing to start",
			isolatedReadTenantEnv, tenant, syntheticMarker,
		)
	}

	shipmentViews, err := shipmenthttp.NewIsolatedOperationsReadIntake(
		isolatedReadScopeReference, tenant, []string{isolatedReadCustomerAccount}, isolatedReadLimit)
	if err != nil {
		return nil, err
	}
	nodeOperationsCatalogue, err := nodeopshttp.NewIsolatedOperationsReadIntake(
		isolatedReadScopeReference, tenant, isolatedReadLimit)
	if err != nil {
		return nil, err
	}
	transportCatalogue, err := tfhttp.NewIsolatedOperationsReadIntake(
		isolatedReadScopeReference, tenant, isolatedReadLimit)
	if err != nil {
		return nil, err
	}
	trackingProjections, err := visibilityhttp.NewIsolatedOperationsReadIntake(
		isolatedReadScopeReference, tenant, isolatedReadLimit)
	if err != nil {
		return nil, err
	}
	pricingCatalogue, err := pricinghttp.NewIsolatedOperationsReadIntake(
		isolatedReadScopeReference, tenant, isolatedReadLimit)
	if err != nil {
		return nil, err
	}
	networkCatalog, err := networkhttp.NewIsolatedOperationsReadIntake(
		isolatedReadScopeReference, tenant, isolatedReadLimit)
	if err != nil {
		return nil, err
	}
	complianceRules, err := customshttp.NewIsolatedOperationsReadIntake(
		isolatedReadScopeReference, tenant, isolatedReadLimit)
	if err != nil {
		return nil, err
	}
	commercialCatalogue, err := commercialhttp.NewIsolatedOperationsReadIntake(
		isolatedReadScopeReference, tenant, isolatedReadLimit)
	if err != nil {
		return nil, err
	}
	collectionCatalogue, err := collectionhttp.NewIsolatedOperationsReadIntake(
		isolatedReadScopeReference, tenant, isolatedReadLimit)
	if err != nil {
		return nil, err
	}
	// 结算与核算四页共用本上下文的一个 Intake：作用域形状同为「租户」一维，责任法人、
	// 结算账户与币种是账上的归属维不是查阅方身份（判据在 settlementhttp.CatalogueQuery）。
	settlementCatalogue, err := settlementhttp.NewIsolatedOperationsReadIntake(
		isolatedReadScopeReference, tenant, isolatedReadLimit)
	if err != nil {
		return nil, err
	}
	// 治理格不收合成租户：治理登记册无租户维是设计（ADR-0083 Decision 三）——同一个
	// 开关决定启用与 SYN- 门禁，但开关值只作启用凭据，不进治理作用域。把 tenant 传进去
	// 就是给一张没有租户列的表造一个过滤维。
	governanceRegisters, err := governancehttp.NewIsolatedOperationsReadIntake(
		isolatedReadScopeReference, isolatedReadLimit)
	if err != nil {
		return nil, err
	}

	return &isolatedReadIntakes{
		shipmentRequestViews: shipmentViews,
		// 面单交易查阅由同一个注入值服务，但走的是它的另一半接口
		// （LabelTransactionQueryIntake，只交出租户维）：同一开关、同一装配点是
		// ADR-0084 决定七要的，而两种查阅面收到的作用域形状不同是决定七同一句话的另一半。
		labelTransactions:       shipmentViews,
		nodeOperationsCatalogue: nodeOperationsCatalogue,
		transportCatalogue:      transportCatalogue,
		trackingProjections:     trackingProjections,
		pricingCatalogue:        pricingCatalogue,
		networkCatalog:          networkCatalog,
		complianceRules:         complianceRules,
		commercialCatalogue:     commercialCatalogue,
		collectionCatalogue:     collectionCatalogue,
		settlementCatalogue:     settlementCatalogue,
		governanceRegisters:     governanceRegisters,
	}, nil
}

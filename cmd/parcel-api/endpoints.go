package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	collectionhttp "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/http"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	governancehttp "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

// 本文件是业务端点的登记处（ADR-0222）：各 endpoints_<特性>.go 在 init 里把自己那几行登进 endpointRegistry，
// 生产装配（assembleBusinessEndpoints）与装配测试（endpoints_test.go）展开的是同一张表、同一个 mount。加一个端点
// 只动那个特性的文件，不动这里。

// endpointLine 是一行端点在登记处的声明。Intake 经 endpointChannels 来的行，接入面那一族由装配测试逐族核对
// （TestEachLineOpensOnlyOnItsDeclaredAccessFace），声明与 mount 那一行写的 Intake 漂不开；决定种类、作业范围种类与外部
// 事实种类各由读它的那条换口用例核。核不到的有两处：ownIntake 的行在装配测试里恒为未配置，族写错了也照绿；首方客户的
// 能力细分格没有用例读它。这两处靠各特性自己的测试另证（ADR-0222 越权风险点 6）。
type endpointLine struct {
	method string
	face   accessFace
	// ownIntake 为真：这一行的 Intake 由本特性从生产共享件自建，不经 endpointChannels，装配测试里它恒为未配置即拒。
	ownIntake bool
	// probe 只给装配测试用：带分派参数或路径参数的行要一条走得到 Intake 的请求目标；空则取路径本身。
	probe string
}

// faceFamily 是能打开一行的那一族接入渠道。
type faceFamily string

const (
	// faceUnconfigured：字面量未配置即拒，哪个开关、哪个渠道都换不了（ADR-0055）。
	faceUnconfigured faceFamily = "未配置即拒"
	// faceIsolatedRead：委托查阅及其子集视图，只有隔离读准入换得了（ADR-0078）。
	faceIsolatedRead faceFamily = "隔离读"
	// faceCustomerClient：首方客户族（ADR-0139）。
	faceCustomerClient faceFamily = "首方客户"
	// 以下四种是操作者渠道的能力面（ADR-0100、ADR-0149、ADR-0151）。
	faceOperationsRead    faceFamily = "主数据与运营查阅读"
	faceRegistryWrite     faceFamily = "登记册配置写"
	faceOperationDecision faceFamily = "运营决定"
	faceWorkFact          faceFamily = "作业事实登记"
	// faceIntegrationClient：集成客户端族（ADR-0149）。
	faceIntegrationClient faceFamily = "集成客户端"
)

// accessFace 是一行的接入面。细分格各只对一族有意义：运营决定按决定种类认证，作业事实按作业范围种类收，集成客户端
// 按外部事实种类授予，首方客户按客户面能力授予。
type accessFace struct {
	family     faceFamily
	decision   accessidentity.DecisionKind
	workScope  accessidentity.WorkScopeKind
	fact       accessidentity.ExternalFactType
	capability accessidentity.CustomerCapability
}

func unconfiguredFace() accessFace   { return accessFace{family: faceUnconfigured} }
func isolatedReadFace() accessFace   { return accessFace{family: faceIsolatedRead} }
func operationsReadFace() accessFace { return accessFace{family: faceOperationsRead} }
func registryWriteFace() accessFace  { return accessFace{family: faceRegistryWrite} }

func customerClientFace(capability accessidentity.CustomerCapability) accessFace {
	return accessFace{family: faceCustomerClient, capability: capability}
}

func decisionFace(kind accessidentity.DecisionKind) accessFace {
	return accessFace{family: faceOperationDecision, decision: kind}
}

func workFactFace(scope accessidentity.WorkScopeKind) accessFace {
	return accessFace{family: faceWorkFact, workScope: scope}
}

func integrationClientFace(fact accessidentity.ExternalFactType) accessFace {
	return accessFace{family: faceIntegrationClient, fact: fact}
}

// String 是路由清单里那一格的写法。
func (face accessFace) String() string {
	switch {
	case face.decision != "":
		return string(face.family) + "/" + face.decision.String()
	case face.workScope != "":
		return string(face.family) + "/" + string(face.workScope)
	case face.fact != "":
		return string(face.family) + "/" + string(face.fact)
	case face.capability != "":
		return string(face.family) + "/" + string(face.capability)
	}
	return string(face.family)
}

// endpointSources 是生产装配里各特性共用的件：库、两道开关与权威串、各族渠道的铸造件。它们在 main.go 开池之后构造一次；
// 特性的 build 从这里取，不另读环境。
type endpointSources struct {
	db            *bentopg.DB
	isolatedWrite *isolatedWriteAdmission
	selfAuthority string
	// 送站交接单的作业事实各口自建铸造器（buildDeliveryHandoverNoteFaces）用的那一组：核验方、操作者册、设备册与准入范围，
	// 与收寄口的作业事实渠道（buildWorkFactIntakes）同一组。设备册另喂作业设备册（buildWorkDeviceCatalogue）。
	operatorVerifier accessidentity.OperatorCredentialVerifier
	operatorRegistry accessidentity.OperatorRegistry
	deviceRegistry   *aipg.DeviceRegistry
	admission        accessidentity.AdmissionScope
	// operatorMinter 与 customerClientMinter 是两族在本进程唯一的那一只铸造器。
	operatorMinter       *accessidentity.OperatorMinter
	customerClientMinter *accessidentity.CustomerClientMinter
}

// endpointChannels 是各特性共用的接入渠道。生产装配与装配测试各给一份；装配测试换其中哪几格，那几格的答复格就由哪条用例钉。
type endpointChannels struct {
	// isolatedRead 只换委托查阅那一只变量（shipmentViews）。
	isolatedRead *isolatedReadIntakes
	// customerSubmission 是提交口的首方 Intake（ADR-0139 决定四；票 first-party-api/00）。
	customerSubmission shipmenthttp.SubmissionIntake
	operatorDecisions  operatorDecisionIntakes
	operatorRegistries operatorRegistryIntakes
	// 外部结果、监管凭证与外部资金事实（含更正）走集成客户端族（ADR-0149、ADR-0151 决定四）。发行方参数未设时
	// 核验方答未配置，与换口之前的 403 同字节；写开关换不了这几行（ADR-0150 决定三）。
	integrationClients integrationClientIntakes
	// 主链命令面每口一个变量、每条真渠道一个 if（各特性在自己的 mount 里立），不并进下面查阅方法里任何一个 if——理由同
	// endpoints_submission.go 里 submissionIntake 那段注释。入参是具体类型：它此刻实现了哪几口，各特性就只换得了哪几行。
	//
	// 作业事实各口挂操作者渠道的「作业事实登记」能力面：首登（ADR-0149 决定二）与更正（随原事实归族，ADR-0151 决定四）；
	// 写开关换不了它们（ADR-0150 决定三）。关段、建派送任务、有效时间判断、承运商首次有效收寄判断归运营决定（ADR-0151
	// 决定六），端点行直接挂 operatorDecisions.transport，不经这两格。
	workFactReception nodeopshttp.ReceptionIntake
	workFactTransport *tfhttp.WorkFactCommandIntake
}

// endpointFeature 是一个特性登进登记处的那一份：lines 声明各行，build 装生产依赖，mount 出端点行。D 是本特性的依赖结构：
// 命名字段，一个字段喂一行（或几行共用一只）。装配测试用的未接线占位与 D 同形，由本特性的测试文件登记（registerUnwired）：
// 占位是测试才用的类型，不进生产文件。
type endpointFeature[D any] struct {
	lines map[string]endpointLine
	build func(endpointSources) (D, error)
	mount func(endpointChannels, D) []httpapi.BusinessEndpoint
}

// registeredEndpoints 是登记处的一项：把 D 收进闭包，登记表因此不必知道各特性的依赖形状。
type registeredEndpoints struct {
	lines map[string]endpointLine
	// wiringKey 是 (*D)(nil)：同一个 D 相等、不同的 D 不等，装配测试按它找本特性登记的占位，不靠反射。
	wiringKey  any
	production func(endpointSources, endpointChannels) ([]httpapi.BusinessEndpoint, error)
	// mountWith 拿一份 D 出端点行；交来的不是 D 是编程错误，当场停。
	mountWith func(endpointChannels, any) []httpapi.BusinessEndpoint
}

// endpointRegistry 只由各特性文件的 init 追加；init 里不构造任何东西，构造发生在开池之后的 assembleBusinessEndpoints。
var endpointRegistry []registeredEndpoints

// registerEndpoints 登记一个特性。缺件、两个特性共用一种依赖结构都是编程错误，在 init 里就停：每一次测试运行都会先撞上它。
func registerEndpoints[D any](feature endpointFeature[D]) {
	if len(feature.lines) == 0 || feature.build == nil || feature.mount == nil {
		panic("parcel-api: an endpoint feature needs lines, build and mount")
	}
	key := any((*D)(nil))
	for _, registered := range endpointRegistry {
		if registered.wiringKey == key {
			panic(fmt.Sprintf("parcel-api: two endpoint features share the wiring type %T", key))
		}
	}
	endpointRegistry = append(endpointRegistry, registeredEndpoints{
		lines:     feature.lines,
		wiringKey: key,
		production: func(sources endpointSources, channels endpointChannels) ([]httpapi.BusinessEndpoint, error) {
			wiring, err := feature.build(sources)
			if err != nil {
				return nil, err
			}
			return feature.mount(channels, wiring), nil
		},
		mountWith: func(channels endpointChannels, wiring any) []httpapi.BusinessEndpoint {
			return feature.mount(channels, wiring.(D))
		},
	})
}

// assembleBusinessEndpoints 是业务端点的装配点（组合根）。各上下文 adapters/http
// 里的接入面处理器全部由各 endpoints_<特性>.go 登进本包的端点登记处，经这里进入
// 进程路由：命令面、业务查阅面与主数据目录查阅面都从这一处进入，领域边界仍由各自的
// 处理器和读口保持。
//
// 按 ADR-0055，登记处不以空清单等真渠道：每个端点各以「未配置即拒」的 Intake
// 起步——不读业务内容、不采信自报身份、不构造命令，对每个请求如实答「接入渠道未配置」
// （403 + ACCESS_CHANNEL_NOT_CONFIGURED）。空清单折叠了两件事：进程外看「产品没有这个
// 能力」与「租户还没配置接入渠道」同答 404，而前者无事可做、后者要去提供渠道参数。
//
// 红线不因此松动：这里不得出现任何「开发用」的采信头部实现。未配置即拒不是那种默认
// 实现——分界同 ADR-0052：「读一个空登记册并如实答未配置不是默认实现，恰恰是它想保护
// 的东西」，而这里的空登记册就是端点登记处自己：真渠道就位前它没有任何一行真 Intake。
//
// 真渠道 Intake 就位时在登记那一行的特性文件里逐端点替换，路由层与处理器不动。载荷规范化摘要与准入
// 范围装配（ADR-0055 第五条）按族解：登记写面不适用（ADR-0100），业务命令面按口定形状、
// 铸信封前读生产权威区间（ADR-0149 决定四、ADR-0151 决定三）；未配置即拒绕开它们只因它
// 走不到那一步。
//
// 路径取各包传输层测试已在用的那一个，不另立一套坐标；TF 的 POD 更正此前没有自己的
// 路径，按它与首登「命令形状与恢复动作不同、故分两个端点」的理由取独立子资源。这些
// 路径今天还不是任何租户的对外契约——真渠道就位那笔工作若要改，改的是登记那一行。
//
// 各端点的第二参（应用编排或读口）与 Intake 是两笔独立的接线：命令面编排均经真库，
// 委托、追踪与主数据目录查阅直接消费所属上下文的真库存储读面，全部由各特性的 build
// 从生产共享件构造。unwired* 类型只余装配测试在用，分辨见 unwired_orchestration.go 的文件注释。
//
// 主数据目录查阅按 ADR-0077 各自消费所属上下文存储；网络目录与服务区域共用
// /network-catalog 的 family 分派，其余页面各有独立入口。登记处按入口逐项登记，少登一个
// 就是把它折回 404，与「渠道未配置」不可分辨，那正是 ADR-0055 要治的病。
//
// 各特性依赖结构里的 *Views、*Catalog 与 *Rules 字段都是查阅端点的读口：读面不是编排（查阅不触发
// 判断、派生或披露），生产装配交入各自的真库读适配器；未配置 Intake 仍拒在它们之前，
// 接入渠道就位前一次也不会被调到。委托查阅及其子集视图另有隔离读面准入（ADR-0078，见 isolatedReadIntakes）可显式启用。
//
// 任一特性构造失败原样交回：调用方带着原因退出进程，与开池那一段同一条纪律。同一路径登了两次也交回错误——路由层会让
// 后一个静默盖掉前一个。
func assembleBusinessEndpoints(sources endpointSources, channels endpointChannels) ([]httpapi.BusinessEndpoint, error) {
	var endpoints []httpapi.BusinessEndpoint
	for _, feature := range endpointRegistry {
		mounted, err := feature.production(sources, channels)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, mounted...)
	}
	if err := refuseRepeatedPatterns(endpoints); err != nil {
		return nil, err
	}
	return endpoints, nil
}

func refuseRepeatedPatterns(endpoints []httpapi.BusinessEndpoint) error {
	seen := make(map[string]bool, len(endpoints))
	for _, endpoint := range endpoints {
		if seen[endpoint.Pattern] {
			return fmt.Errorf("parcel-api: endpoint %s is registered twice", endpoint.Pattern)
		}
		seen[endpoint.Pattern] = true
	}
	return nil
}

// 缺省朝拦：真渠道与隔离入参都缺席时，下面这组查阅变量全取未配置即拒。
//
// **读写入参各换各的行，互不顶替**（ADR-0091 决定四把开关也分成了两个）：isolatedRead 只换委托查阅那一只变量，
// customerSubmission（首方客户渠道）只换 `/shipment-requests` 一行。运营查阅各族走 operatorRegistries 的查阅读，作业事实各口走
// workFactReception / workFactTransport，运营决定各口走 operatorDecisions，都不经隔离开关。其余命令面挂字面量
// `UnconfiguredIntake{}`，不经由任何变量——读那一行就能看出它们哪个开关都换不了。
//
// 运营查阅各族挂操作者查阅读（ADR-0100 决定四；ADR-0150 决定三：合成租户按真实租户对待，不另开放行）。发行方
// 未配置时这些 Intake 仍在，答的是未配置，与未接真渠道时同码。

func (channels endpointChannels) shipmentViews() shipmenthttp.ShipmentRequestViewsIntake {
	// 委托查阅及其子集视图仍走隔离读：它们的读口按可见客户账户切，操作者信封只给得出租户（票 operator-channel/53）。
	if channels.isolatedRead != nil {
		return channels.isolatedRead.shipmentRequestViews
	}
	return shipmenthttp.UnconfiguredIntake{}
}

func (channels endpointChannels) labelTransactions() shipmenthttp.LabelTransactionQueryIntake {
	if channels.operatorRegistries.shipmentOperationsRead != nil {
		return channels.operatorRegistries.shipmentOperationsRead
	}
	return shipmenthttp.UnconfiguredIntake{}
}

func (channels endpointChannels) channelSelectionDecisions() shipmenthttp.ChannelSelectionDecisionQueryIntake {
	if channels.operatorRegistries.shipmentOperationsRead != nil {
		return channels.operatorRegistries.shipmentOperationsRead
	}
	return shipmenthttp.UnconfiguredIntake{}
}

func (channels endpointChannels) nodeOperationsCatalogue() nodeopshttp.CatalogueQueryIntake {
	if channels.operatorRegistries.nodeOperationsRead != nil {
		return channels.operatorRegistries.nodeOperationsRead
	}
	return nodeopshttp.UnconfiguredIntake{}
}

func (channels endpointChannels) transportCatalogue() tfhttp.CatalogueQueryIntake {
	if channels.operatorRegistries.transportOperationsRead != nil {
		return channels.operatorRegistries.transportOperationsRead
	}
	return tfhttp.UnconfiguredIntake{}
}

func (channels endpointChannels) trackingProjections() visibilityhttp.OperationsTrackingIntake {
	if channels.operatorRegistries.visibilityOperationsRead != nil {
		return channels.operatorRegistries.visibilityOperationsRead
	}
	return visibilityhttp.UnconfiguredIntake{}
}

func (channels endpointChannels) pricingCatalogue() pricinghttp.PricingCatalogueIntake {
	if channels.operatorRegistries.pricingOperationsRead != nil {
		return channels.operatorRegistries.pricingOperationsRead
	}
	return pricinghttp.UnconfiguredIntake{}
}

func (channels endpointChannels) networkCatalog() networkhttp.CatalogueQueryIntake {
	if channels.operatorRegistries.networkOperationsRead != nil {
		return channels.operatorRegistries.networkOperationsRead
	}
	return networkhttp.UnconfiguredIntake{}
}

func (channels endpointChannels) complianceRules() customshttp.CatalogueQueryIntake {
	if channels.operatorRegistries.customsOperationsRead != nil {
		return channels.operatorRegistries.customsOperationsRead
	}
	return customshttp.UnconfiguredIntake{}
}

func (channels endpointChannels) commercialCatalogue() commercialhttp.CommercialCatalogueIntake {
	if channels.operatorRegistries.commercialOperationsRead != nil {
		return channels.operatorRegistries.commercialOperationsRead
	}
	return commercialhttp.UnconfiguredIntake{}
}

func (channels endpointChannels) collectionCatalogue() collectionhttp.CatalogueQueryIntake {
	if channels.operatorRegistries.collectionOperationsRead != nil {
		return channels.operatorRegistries.collectionOperationsRead
	}
	return collectionhttp.UnconfiguredIntake{}
}

func (channels endpointChannels) settlementCatalogue() settlementhttp.CatalogueQueryIntake {
	if channels.operatorRegistries.settlementOperationsRead != nil {
		return channels.operatorRegistries.settlementOperationsRead
	}
	return settlementhttp.UnconfiguredIntake{}
}

func (channels endpointChannels) governanceRegistry() governancehttp.RegistryQueryIntake {
	if channels.operatorRegistries.governanceOperationsRead != nil {
		return channels.operatorRegistries.governanceOperationsRead
	}
	return governancehttp.UnconfiguredIntake{}
}

package main

import (
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

// assembleBusinessEndpoints 是业务端点的装配点（组合根）。各上下文 adapters/http
// 里的接入面处理器全部挂在这里：命令面、业务查阅面与主数据目录查阅面都从这一处进入
// 进程路由，领域边界仍由各自的处理器和读口保持。
//
// 按 ADR-0055，本函数不再以空清单等 `PAR-INT-01`：每个端点各以「未配置即拒」的 Intake
// 起步——不读业务内容、不采信自报身份、不构造命令，对每个请求如实答「接入渠道未配置」
// （403 + ACCESS_CHANNEL_NOT_CONFIGURED）。空清单折叠了两件事：进程外看「产品没有这个
// 能力」与「租户还没配置接入渠道」同答 404，而前者无事可做、后者要去提供渠道参数。
//
// 红线不因此松动：这里不得出现任何「开发用」的采信头部实现。未配置即拒不是那种默认
// 实现——分界同 ADR-0052：「读一个空登记册并如实答未配置不是默认实现，恰恰是它想保护
// 的东西」，而这里的空登记册就是本函数自己：真渠道就位前它没有任何一行真 Intake。
//
// 真渠道 Intake 就位时在本函数逐端点替换，路由层与处理器不动；载荷规范化摘要与准入
// 范围装配（`PAR-GOV-03..07`）仍拦着真渠道 Intake，未配置即拒绕开它们只因它走不到那
// 一步（ADR-0055 第五条）。
//
// 路径取各包传输层测试已在用的那一个，不另立一套坐标；TF 的 POD 更正此前没有自己的
// 路径，按它与首登「命令形状与恢复动作不同、故分两个端点」的理由取独立子资源。这些
// 路径今天还不是任何租户的对外契约——真渠道就位那笔工作若要改，改的是本函数一处。
//
// 各端点的第二参（应用编排或读口）与 Intake 是两笔独立的接线：命令面编排均经真库，
// 委托、追踪与主数据目录查阅直接消费所属上下文的真库存储读面，全部由 main 构造后入参
// 交入。unwired* 类型只余装配测试在用，分辨见 unwired_orchestration.go 的文件注释。
//
// 主数据目录查阅按 ADR-0077 各自消费所属上下文存储；网络目录与服务区域共用
// /network-catalog 的 family 分派，其余页面各有独立入口。此处按入口逐项枚举，少装一个
// 就是把它折回 404，与「渠道未配置」不可分辨，那正是 ADR-0055 要治的病。
//
// 所有 *Views、*Catalog 与 *Rules 参数都是查阅端点的读口：读面不是编排（查阅不触发
// 判断、派生或披露），生产装配交入各自的真库读适配器；未配置 Intake 仍拒在它们之前，
// 接入渠道就位前一次也不会被调到——除非隔离读面准入按 ADR-0078 被显式启用（见
// isolatedReadIntakes），且被换的只有运营查阅行。
func assembleBusinessEndpoints(
	submission shipmenthttp.SubmissionHandler,
	withdrawal shipmenthttp.WithdrawalHandler,
	requestViews shipmenthttp.ShipmentRequestViewsReader,
	manualReview shipmenthttp.ManualReviewCompletionHandler,
	rejection shipmenthttp.ActiveRejectionHandler,
	reviewQueue shipmenthttp.AcceptanceReviewQueueReader,
	reviewJudgments shipmenthttp.RecordedJudgmentsReader,
	labelTransactions shipmenthttp.LabelTransactionsReader,
	cancellation shipmenthttp.CancellationHandler,
	reception nodeopshttp.ReceptionHandler,
	nodeOperationsRecords nodeopshttp.ReviewCatalogueReader,
	delivery tfhttp.DeliveryHandler,
	transportFulfillmentRecords tfhttp.ReviewCatalogueReader,
	handoverScopeSummary tfhttp.HandoverScopeSummarizer,
	trackingViews visibilityhttp.TrackingViewReader,
	projectionViews visibilityhttp.OperationsProjectionReader,
	claims visibilityhttp.ClaimReceiver,
	results customshttp.ResultHandler,
	priceCards pricinghttp.PriceCardCatalogueReader,
	referenceSeries pricinghttp.ReferenceSeriesCatalogueReader,
	pricingEvaluations pricinghttp.EvaluationCatalogueReader,
	priceCardRegistration pricinghttp.PriceCardRegistrar,
	referenceSeriesRegistration pricinghttp.ReferenceSeriesRegistrar,
	referenceSeriesReview pricinghttp.ReferenceSeriesReviewer,
	networkCatalog networkhttp.OperationsCatalogReader,
	routePlans networkhttp.RoutePlanCatalogueReader,
	networkCatalogRegistration networkhttp.CatalogRegistrar,
	complianceRules customshttp.RuleCatalogueReader,
	caseRegisters customshttp.CaseRegisterCatalogueReader,
	gateConditions customshttp.GateConditionCatalogueReader,
	portsPaths customshttp.PortsPathsCatalogueReader,
	interpretationRuleRegistration customshttp.InterpretationRuleRegistrar,
	gateCatalogRegistration customshttp.GateCatalogRegistrar,
	candidatePortRegistration customshttp.CandidatePortRegistrar,
	declarationPathRegistration customshttp.DeclarationPathRegistrar,
	caseRequirementRegistration customshttp.CaseRequirementRegistrar,
	serviceProducts commercialhttp.ServiceProductCatalogueReader,
	commercialPolicies commercialhttp.CommercialPolicyCatalogueReader,
	commercialRelations commercialhttp.CommercialRelationCatalogueReader,
	partyIdentities commercialhttp.PartyIdentityCatalogueReader,
	customerAccounts commercialhttp.CustomerAccountCatalogueReader,
	productChannelMappings commercialhttp.ProductChannelCatalogueReader,
	commercialPublication commercialhttp.CommercialAuthorityPublisher,
	partyIdentityRegistration commercialhttp.PartyIdentityRegistrar,
	productChannelRegistration commercialhttp.ProductChannelRegistrar,
	channelAccountUseRegistration commercialhttp.ChannelAccountUseRegistrar,
	visibilityCatalogues visibilityhttp.VisibilityCatalogueReader,
	milestoneMappingRegistration visibilityhttp.MilestoneMappingRegistrar,
	triageRulesRegistration visibilityhttp.TriageRulesRegistrar,
	notificationPolicyRegistration visibilityhttp.NotificationPolicyRegistrar,
	claimEligibilityRegistration visibilityhttp.ClaimEligibilityRegistrar,
	claimAuthorizationRegistration visibilityhttp.ClaimAuthorizationRegistrar,
	disclosurePolicyRegistration visibilityhttp.DisclosurePolicyRegistrar,
	exceptionTriageRecords visibilityhttp.TriageReviewReader,
	exceptionCaseRecords visibilityhttp.CaseReviewReader,
	claimsRecoveryRecords visibilityhttp.ClaimsRecoveryReviewReader,
	codSubledgers collectionhttp.CodSubledgerCatalogueReader,
	settlementCharges settlementhttp.ChargeCatalogueReader,
	settlementStatements settlementhttp.StatementCatalogueReader,
	settlementFundsApplications settlementhttp.FundsApplicationCatalogueReader,
	settlementOperatingResults settlementhttp.OperatingCatalogueReader,
	governanceRegisters governancehttp.GovernanceRegistryReader,
	isolatedRead *isolatedReadIntakes,
	isolatedSubmission shipmenthttp.SubmissionIntake,
) []httpapi.BusinessEndpoint {
	// 缺省朝拦：两个隔离入参都为 nil 时，下面这组变量全取未配置即拒，整份装配与
	// ADR-0078/0091 之前逐字节同形。
	//
	// **两个入参各换各的行，互不顶替**（ADR-0091 决定四把开关也分成了两个）：isolatedRead
	// 换查阅行，isolatedSubmission 只换 `/shipment-requests` 一行。其余命令面仍挂字面量
	// `UnconfiguredIntake{}`，不经由任何变量——读这段代码就能看出它们两个开关都换不了。
	// 这句从前说的是「命令面全都换不了」，ADR-0091 让提交那一行成了例外，因此改成现在
	// 这句；剩下那几行的字面量纪律一字未松。
	shipmentViewsIntake := shipmenthttp.ShipmentRequestViewsIntake(shipmenthttp.UnconfiguredIntake{})
	labelTransactionIntake := shipmenthttp.LabelTransactionQueryIntake(shipmenthttp.UnconfiguredIntake{})
	nodeOperationsCatalogueIntake := nodeopshttp.CatalogueQueryIntake(nodeopshttp.UnconfiguredIntake{})
	transportCatalogueIntake := tfhttp.CatalogueQueryIntake(tfhttp.UnconfiguredIntake{})
	trackingProjectionsIntake := visibilityhttp.OperationsTrackingIntake(visibilityhttp.UnconfiguredIntake{})
	pricingCatalogueIntake := pricinghttp.PricingCatalogueIntake(pricinghttp.UnconfiguredIntake{})
	networkCatalogIntake := networkhttp.CatalogueQueryIntake(networkhttp.UnconfiguredIntake{})
	complianceRulesIntake := customshttp.CatalogueQueryIntake(customshttp.UnconfiguredIntake{})
	commercialCatalogueIntake := commercialhttp.CommercialCatalogueIntake(commercialhttp.UnconfiguredIntake{})
	collectionCatalogueIntake := collectionhttp.CatalogueQueryIntake(collectionhttp.UnconfiguredIntake{})
	settlementCatalogueIntake := settlementhttp.CatalogueQueryIntake(settlementhttp.UnconfiguredIntake{})
	governanceRegistryIntake := governancehttp.RegistryQueryIntake(governancehttp.UnconfiguredIntake{})
	if isolatedRead != nil {
		shipmentViewsIntake = isolatedRead.shipmentRequestViews
		labelTransactionIntake = isolatedRead.labelTransactions
		nodeOperationsCatalogueIntake = isolatedRead.nodeOperationsCatalogue
		transportCatalogueIntake = isolatedRead.transportCatalogue
		trackingProjectionsIntake = isolatedRead.trackingProjections
		pricingCatalogueIntake = isolatedRead.pricingCatalogue
		networkCatalogIntake = isolatedRead.networkCatalog
		complianceRulesIntake = isolatedRead.complianceRules
		commercialCatalogueIntake = isolatedRead.commercialCatalogue
		collectionCatalogueIntake = isolatedRead.collectionCatalogue
		settlementCatalogueIntake = isolatedRead.settlementCatalogue
		governanceRegistryIntake = isolatedRead.governanceRegisters
	}

	// 提交口是 ADR-0091 放行的第一格，也是唯一一格：它单独一个变量，与上面那组查阅
	// 变量分开，免得日后有人顺手把它并进 isolatedRead 那个 if 里——并进去就等于让读
	// 开关也能开写行，而两个开关分设的全部理由就是不许这样。
	submissionIntake := shipmenthttp.SubmissionIntake(shipmenthttp.UnconfiguredIntake{})
	if isolatedSubmission != nil {
		submissionIntake = isolatedSubmission
	}

	return []httpapi.BusinessEndpoint{
		{Pattern: "/shipment-requests", Handler: shipmenthttp.NewSubmitShipmentRequestEndpoint(submissionIntake, submission)},
		{Pattern: "/shipment-requests/withdrawals", Handler: shipmenthttp.NewWithdrawShipmentRequestEndpoint(shipmenthttp.UnconfiguredIntake{}, withdrawal)},
		{Pattern: "/shipment-requests/parcel-cancellations", Handler: shipmenthttp.NewCancelParcelEndpoint(shipmenthttp.UnconfiguredIntake{}, cancellation)},
		// 复核完成与主动拒绝两个命令口（票 09；ADR-0081 的命令面保留条款、ADR-0086）：
		// 与其余命令面同挂字面量 UnconfiguredIntake{}，隔离读准入换不了写行。
		{Pattern: "/shipment-requests/manual-review-completions", Handler: shipmenthttp.NewCompleteManualReviewEndpoint(shipmenthttp.UnconfiguredIntake{}, manualReview)},
		{Pattern: "/shipment-requests/rejections", Handler: shipmenthttp.NewRejectShipmentRequestEndpoint(shipmenthttp.UnconfiguredIntake{}, rejection)},
		{Pattern: "/shipment-request-views", Handler: shipmenthttp.NewQueryShipmentRequestViewsEndpoint(shipmentViewsIntake, requestViews)},
		// 复核队列查阅（票 09）：委托查阅面的子集视图，Intake 沿用同一变量——隔离读
		// 准入（ADR-0078）启用时随委托查阅一起换值，不另立第二种准入形。
		{Pattern: "/acceptance-review-queue", Handler: shipmenthttp.NewQueryAcceptanceReviewQueueEndpoint(shipmentViewsIntake, reviewQueue, reviewJudgments)},
		// 面单交易查阅（票 admin-skeleton-closure-batch/08，ADR-0084 决定七）：**另立一种
		// 准入形**而不是复用委托查阅那个变量。两者由同一个隔离读开关、同一个装配点换值
		// （决定七要的「沿用同一开关」），但接口不同：面单交易没有账户维可分，收一个必带
		// 账户维的作用域等于在类型上声称会按账户过滤而它不会。渠道墙未降前这一口读到的是
		// 空册，那是设计：写入方是渠道适配器，首发不进生产。
		{Pattern: "/label-transactions", Handler: shipmenthttp.NewQueryLabelTransactionsEndpoint(labelTransactionIntake, labelTransactions)},
		{Pattern: "/node-operations/receptions", Handler: nodeopshttp.NewReceiveDeliveredUnitEndpoint(nodeopshttp.UnconfiguredIntake{}, reception)},
		// 节点作业与运输履约查阅页（票 admin-skeleton-closure-batch/05）各一口按
		// registry 分派（NO 三册、TF 四册）：分派对应「一页里的页签」。命令端点在上，
		// 用的是字面量 UnconfiguredIntake{}；查阅行走本上下文自己的 Intake 变量，
		// 隔离读准入（ADR-0078）启用时只换查阅行，命令行换不了。
		{Pattern: "/node-operations-records", Handler: nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(nodeOperationsCatalogueIntake, nodeOperationsRecords)},
		{Pattern: "/transport-fulfillment/deliveries", Handler: tfhttp.NewRegisterEffectiveDeliveryEndpoint(tfhttp.UnconfiguredIntake{}, delivery)},
		{Pattern: "/transport-fulfillment/delivery-proof-corrections", Handler: tfhttp.NewCorrectDeliveryProofEndpoint(tfhttp.UnconfiguredIntake{}, delivery)},
		{Pattern: "/transport-fulfillment-records", Handler: tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(transportCatalogueIntake, transportFulfillmentRecords)},
		// 交接范围汇总（票 admin-web-audit-followups/06，读面来自 tf-unwired-seven/03）。
		// 它是本装配表上第一行第二参不是读口而是**应用读用例**的查阅端点：汇总是派生量，
		// 只能由 domain.SummarizeHandovers 派生（理由在 tfhttp.HandoverScopeSummarizer）。
		// 读用例零登记零编辑零披露，查阅的性质没变，因此 Intake 随运输履约查阅同一个变量
		// ——隔离读准入（ADR-0078）启用时两行一起换值，那三条判据它逐条满足。
		{Pattern: "/transport-fulfillment-handover-scope-summary", Handler: tfhttp.NewQueryHandoverScopeSummaryEndpoint(transportCatalogueIntake, handoverScopeSummary)},
		{Pattern: "/customer-tracking-view", Handler: visibilityhttp.NewQueryCustomerTrackingViewEndpoint(visibilityhttp.UnconfiguredIntake{}, trackingViews)},
		{Pattern: "/tracking-projections", Handler: visibilityhttp.NewQueryTrackingProjectionsEndpoint(trackingProjectionsIntake, projectionViews)},
		{Pattern: "/claims", Handler: visibilityhttp.NewReceiveClaimEndpoint(visibilityhttp.UnconfiguredIntake{}, claims)},
		{Pattern: "/customs/external-results", Handler: customshttp.NewReceiveExternalResultEndpoint(customshttp.UnconfiguredIntake{}, results)},
		{Pattern: "/pricing-price-cards", Handler: pricinghttp.NewQueryPriceCardsEndpoint(pricingCatalogueIntake, priceCards)},
		{Pattern: "/pricing-reference-series", Handler: pricinghttp.NewQueryReferenceSeriesEndpoint(pricingCatalogueIntake, referenceSeries)},
		// 评价册与路由判断两册（票 admin-skeleton-closure-batch/03）：业务事实册的
		// 查阅与目录查阅同属租户内运营读面，各随本上下文既有的 Intake 变量换值，
		// 不为事实册另立第二种准入形（裁决在各端点构造函数注释）。
		{Pattern: "/pricing-evaluations", Handler: pricinghttp.NewQueryEvaluationsEndpoint(pricingCatalogueIntake, pricingEvaluations)},
		// 价卡与序列登记写面（ADR-0085，票 admin-write-faces/01）：登记是命令行，与
		// 其余命令面同挂字面量 UnconfiguredIntake{}——写准入不另立形，隔离读准入
		// （ADR-0078）只经查阅行的 Intake 变量换值，写行换不了。两类登记各立端点，
		// 与其 CLI 命令一一对应（裁决在端点构造函数注释）；登记 CLI 保留为受控批量口，
		// 两口消费同一登记用例，答案代数一致。
		{Pattern: "/pricing-price-card-registrations", Handler: pricinghttp.NewRegisterPriceCardEndpoint(pricinghttp.UnconfiguredIntake{}, priceCardRegistration)},
		{Pattern: "/pricing-reference-series-registrations", Handler: pricinghttp.NewRegisterReferenceSeriesEndpoint(pricinghttp.UnconfiguredIntake{}, referenceSeriesRegistration)},
		// 序列版本复核（ADR-0099 决定二，票 pricing-reference-series-operations/04）：治理
		// 动作也是命令行，同挂字面量 UnconfiguredIntake{}。**这一口比两个登记口更不能松**
		// ——复核责任方是四眼门的一半，任何采信自报身份的 Intake 都等于把那道门拆了。
		{Pattern: "/pricing-reference-series-reviews", Handler: pricinghttp.NewReviewReferenceSeriesEndpoint(pricinghttp.UnconfiguredIntake{}, referenceSeriesReview)},
		{Pattern: "/network-catalog", Handler: networkhttp.NewQueryNetworkCatalogEndpoint(networkCatalogIntake, networkCatalog)},
		{Pattern: "/route-plans", Handler: networkhttp.NewQueryRoutePlansEndpoint(networkCatalogIntake, routePlans)},
		// 网络目录七族登记写面（ADR-0085，票 admin-write-faces/02 切片 02a）：登记是命令
		// 行，与其余命令面同挂字面量 UnconfiguredIntake{}——写准入不另立形，隔离读准入
		// （ADR-0078）只经查阅行的 Intake 变量换值，写行换不了。
		//
		// 路径取「读口册名 + 该族原词 + -registrations」：族词与查阅的 `?family=`、登记
		// CLI 的 `-kind` 逐字同一个，同一本册在三处不换词。一族一个端点而不用 `?family=`
		// 把七族塑进一个口：七族的命令类型互不相同，合成一口就得在 Intake 里先认族再定
		// 形状，装配点从此可以把一族的译装接到另一族的端点上而编译仍绿。
		{Pattern: "/network-catalog-node-registrations", Handler: networkhttp.NewRegisterNodeVersionEndpoint(networkhttp.UnconfiguredIntake{}, networkCatalogRegistration)},
		{Pattern: "/network-catalog-connection-registrations", Handler: networkhttp.NewRegisterConnectionVersionEndpoint(networkhttp.UnconfiguredIntake{}, networkCatalogRegistration)},
		{Pattern: "/network-catalog-line-registrations", Handler: networkhttp.NewRegisterLineVersionEndpoint(networkhttp.UnconfiguredIntake{}, networkCatalogRegistration)},
		{Pattern: "/network-catalog-service-area-registrations", Handler: networkhttp.NewRegisterServiceAreaVersionEndpoint(networkhttp.UnconfiguredIntake{}, networkCatalogRegistration)},
		{Pattern: "/network-catalog-service-calendar-registrations", Handler: networkhttp.NewRegisterServiceCalendarVersionEndpoint(networkhttp.UnconfiguredIntake{}, networkCatalogRegistration)},
		{Pattern: "/network-catalog-availability-adjustment-registrations", Handler: networkhttp.NewRegisterAvailabilityAdjustmentEndpoint(networkhttp.UnconfiguredIntake{}, networkCatalogRegistration)},
		{Pattern: "/network-catalog-route-strategy-registrations", Handler: networkhttp.NewRegisterRouteStrategyVersionEndpoint(networkhttp.UnconfiguredIntake{}, networkCatalogRegistration)},
		{Pattern: "/customs-compliance-rules", Handler: customshttp.NewQueryComplianceRulesEndpoint(complianceRulesIntake, complianceRules)},
		// 案件配置册、门禁条件册与规则库查阅同属关务租户内运营读面，共用同一个
		// CatalogueQueryIntake 变量：隔离读准入（ADR-0078）启用时它们随该变量一起换值，
		// 判据同为那三条（消费所属上下文存储读面、零持久化、作用域为运营侧授权结果）。
		//
		// 门禁条件不并进 /customs-case-registers 的 registry 分派：那三册归 customs-cases
		// 一张页面，门禁两表归 customs-restrictions——分派对应「一页里的页签」，各立入口
		// 对应「各自独立的页」（票 admin-web-page-wiring-frontier/04 的归属裁决、06 实施）。
		{Pattern: "/customs-case-registers", Handler: customshttp.NewQueryCaseRegistersEndpoint(complianceRulesIntake, caseRegisters)},
		{Pattern: "/customs-gate-conditions", Handler: customshttp.NewQueryGateConditionsEndpoint(complianceRulesIntake, gateConditions)},
		// 口岸与申报路径两册（票 admin-remainder-mechanism-batch/03）是一张页面的两签
		// 查阅面，共用一个端点按 registry 分派，Intake 与关务运营读面同族同变量。
		{Pattern: "/customs-ports-paths", Handler: customshttp.NewQueryPortsPathsEndpoint(complianceRulesIntake, portsPaths)},
		// 关务四类配置登记写面（ADR-0085，票 admin-write-faces/02 切片 02b）：写准入不
		// 另立形，判据同上。同一受控 CLI 里改「案上此刻的事实」的那些命令不在本端点族内，
		// 范围判据在票上，此处不复述。
		//
		// 路径不照网络那样带上读口册名：关务的查阅入口本就按事物平铺（/customs-case-registers、
		// /customs-gate-conditions、/customs-ports-paths），四个事物词在整个关务面上唯一，
		// 前缀套前缀只会把路径拉长而分不出更多东西。事物词与读口的 `?registry=` 同源。
		{Pattern: "/customs-interpretation-rule-registrations", Handler: customshttp.NewRegisterInterpretationRuleEndpoint(customshttp.UnconfiguredIntake{}, interpretationRuleRegistration)},
		{Pattern: "/customs-gate-catalog-registrations", Handler: customshttp.NewRegisterGateCatalogEndpoint(customshttp.UnconfiguredIntake{}, gateCatalogRegistration)},
		{Pattern: "/customs-candidate-port-registrations", Handler: customshttp.NewRegisterCandidatePortEndpoint(customshttp.UnconfiguredIntake{}, candidatePortRegistration)},
		{Pattern: "/customs-declaration-path-registrations", Handler: customshttp.NewRegisterDeclarationPathEndpoint(customshttp.UnconfiguredIntake{}, declarationPathRegistration)},
		// 建案要求规则是关务的第五类配置，比另四类晚一步接进来：票 admin-write-faces/02
		// 的关务片把十二个用例分成「配置四类」与「案件事实七类」，四加七只有十一个，漏掉
		// 的第十二个正是它。它有登记用例、有 CLI 命令（case-requirement）、读面早在合规
		// 规则页的册 chip 里，唯独端点表没有它的行——管理台的规则页因此只能把签名写死成
		// 「登记解释规则」，那一半的缺席是页面在替它认账。
		{Pattern: "/customs-case-requirement-registrations", Handler: customshttp.NewRegisterCaseRequirementEndpoint(customshttp.UnconfiguredIntake{}, caseRequirementRegistration)},
		{Pattern: "/commercial-service-products", Handler: commercialhttp.NewQueryServiceProductsEndpoint(commercialCatalogueIntake, serviceProducts)},
		{Pattern: "/commercial-policies", Handler: commercialhttp.NewQueryCommercialPoliciesEndpoint(commercialCatalogueIntake, commercialPolicies)},
		{Pattern: "/commercial-customer-contracts", Handler: commercialhttp.NewQueryCustomerContractsEndpoint(commercialCatalogueIntake, commercialRelations)},
		{Pattern: "/commercial-supplier-agreements", Handler: commercialhttp.NewQuerySupplierAgreementsEndpoint(commercialCatalogueIntake, commercialRelations)},
		// 参与方身份两册（票 admin-remainder-mechanism-batch/01）各立入口，不并进上面
		// 载体目录的读口：载体是版本化商业对象，身份与关系是登记→生效→停用的生命周期，
		// 行形状与状态代数不同（裁决在 ports.PartyIdentityCatalogueRead 注释）。
		// 身份本体那一册与法人、关系两册各立入口（票 admin-remainder-mechanism-batch/01
		// 的补格裁定）：一个参与方既可以不是法人、也可以不在任何关系里，被停用的那种恰恰
		// 如此；不给它自己的入口，身份生命周期的`已登记`与`已停用`两格在管理台就没有实例
		// 可显，而那正是本票标题那个生命周期。三口共用同一个读口参数与同一个 Intake 变量。
		{Pattern: "/commercial-business-parties", Handler: commercialhttp.NewQueryBusinessPartiesEndpoint(commercialCatalogueIntake, partyIdentities)},
		{Pattern: "/commercial-group-legal-entities", Handler: commercialhttp.NewQueryGroupLegalEntitiesEndpoint(commercialCatalogueIntake, partyIdentities)},
		{Pattern: "/commercial-party-relationships", Handler: commercialhttp.NewQueryPartyRelationshipsEndpoint(commercialCatalogueIntake, partyIdentities)},
		// 货主客户账户册（票 admin-write-faces/04）——身份三级里的第三级，独立入口且独立读口
		// 参数：它按 CONTEXT 落在客户与合同页而不是上面两册所在的页（一页一入口），读口跟着
		// 页走；不并进 /commercial-customer-contracts，合同是商业版本、账户是身份登记修订，
		// 状态代数不同（裁决在 ports.CustomerAccountCatalogueRead 注释）。生产装配交入的仍是
		// 同一只商业目录适配器。
		{Pattern: "/commercial-customer-accounts", Handler: commercialhttp.NewQueryCustomerAccountsEndpoint(commercialCatalogueIntake, customerAccounts)},
		// 产品—渠道映射册（票 admin-remainder-mechanism-batch/02）独立入口，不并进
		// /commercial-service-products：那边上列版本壳，这边上列登记册信封（产品×渠道
		// ×区间的修订），行形状与修订轴不同（裁决在 ports.ProductChannelMappingCatalogueRead）。
		{Pattern: "/commercial-product-channel-mappings", Handler: commercialhttp.NewQueryProductChannelMappingsEndpoint(commercialCatalogueIntake, productChannelMappings)},
		// 商业八类配置写面（ADR-0085，票 admin-write-faces/02 切片 02c）：写准入不另立形，
		// 判据同上——命令面一律挂字面量 UnconfiguredIntake{}，隔离读准入换不了写行。
		//
		// 发布口不带 `-registrations` 后缀：本上下文这一格的动词是发布，答案代数说的也是
		// 发布（`已发布已生效`/`已计划生效`），叫成登记会与身份、映射两族的登记答案混为
		// 一谈（裁决在端点文件头）。它也只有一个端点——服务产品、规则包、合同、协议与各类
		// 策略是同一个发布用例的输入，类别在版本规格里，不是另一种命令。
		//
		// 停用口叫 `-deactivations` 而不是 `-registrations`：它是往修订链上插一笔新修订的
		// 状态推进，不是登记一个新身份。名字照实说，是因为「登记」与「停用」在这本册上的
		// 续办动作不同，路径是登记方看见的第一样东西。
		{Pattern: "/commercial-publications", Handler: commercialhttp.NewPublishCommercialAuthorityEndpoint(commercialhttp.UnconfiguredIntake{}, commercialPublication)},
		{Pattern: "/commercial-business-party-registrations", Handler: commercialhttp.NewRegisterBusinessPartyEndpoint(commercialhttp.UnconfiguredIntake{}, partyIdentityRegistration)},
		{Pattern: "/commercial-legal-entity-registrations", Handler: commercialhttp.NewRegisterLegalEntityEndpoint(commercialhttp.UnconfiguredIntake{}, partyIdentityRegistration)},
		{Pattern: "/commercial-customer-account-registrations", Handler: commercialhttp.NewRegisterCustomerAccountEndpoint(commercialhttp.UnconfiguredIntake{}, partyIdentityRegistration)},
		{Pattern: "/commercial-party-relationship-registrations", Handler: commercialhttp.NewRegisterPartyRelationshipEndpoint(commercialhttp.UnconfiguredIntake{}, partyIdentityRegistration)},
		{Pattern: "/commercial-party-identity-deactivations", Handler: commercialhttp.NewDeactivatePartyIdentityEndpoint(commercialhttp.UnconfiguredIntake{}, partyIdentityRegistration)},
		{Pattern: "/commercial-service-product-form-registrations", Handler: commercialhttp.NewRegisterServiceProductFormEndpoint(commercialhttp.UnconfiguredIntake{}, productChannelRegistration)},
		{Pattern: "/commercial-product-channel-mapping-registrations", Handler: commercialhttp.NewRegisterProductChannelMappingEndpoint(commercialhttp.UnconfiguredIntake{}, productChannelRegistration)},
		// 渠道账号使用授权两口（ADR-0093）。撤销不叫 `-deactivations` 也不走 DELETE：它是往
		// 修订链上追加一条终止事实，册上那一行不会消失，而那两个名字都会让登记方以为会。
		// 分两口而不带动作字段的理由在端点族注释里：合一口之后载荷里会同时躺着动作与授权
		// 正文，等于把「后继修订不得改换账号或双方」那道门要挡的机会又递回给调用方。
		{Pattern: "/commercial-channel-account-use-registrations", Handler: commercialhttp.NewRegisterChannelAccountUseEndpoint(commercialhttp.UnconfiguredIntake{}, channelAccountUseRegistration)},
		{Pattern: "/commercial-channel-account-use-revocations", Handler: commercialhttp.NewRevokeChannelAccountUseEndpoint(commercialhttp.UnconfiguredIntake{}, channelAccountUseRegistration)},
		// VE 六类目录查阅与运营追踪查阅同属租户内运营读面，共用同一个
		// OperationsTrackingIntake 变量：隔离读准入（ADR-0078）启用时两行一起换值，
		// 判据同为那三条（消费所属上下文存储读面、零持久化、作用域为运营侧授权结果）。
		{Pattern: "/visibility-catalogues", Handler: visibilityhttp.NewQueryVisibilityCataloguesEndpoint(trackingProjectionsIntake, visibilityCatalogues)},
		// VE 六类配置登记写面（ADR-0085，票 admin-write-faces/02 切片 02d）：写准入不另
		// 立形，判据同上。路径取「读口册名 + 该类种类词 + -registrations」，种类词与查阅
		// 的 `?kind=` 同字（换成小写连字）——同一本册在读口与写口不换词；分诊那一格 CLI
		// 叫 triage-rules 而读口 kind 叫 TRIAGE_RULE，路径随读口，单复数不在此处再分叉。
		{Pattern: "/visibility-catalogue-milestone-mapping-registrations", Handler: visibilityhttp.NewRegisterMilestoneMappingEndpoint(visibilityhttp.UnconfiguredIntake{}, milestoneMappingRegistration)},
		{Pattern: "/visibility-catalogue-triage-rule-registrations", Handler: visibilityhttp.NewRegisterTriageRulesEndpoint(visibilityhttp.UnconfiguredIntake{}, triageRulesRegistration)},
		{Pattern: "/visibility-catalogue-notification-policy-registrations", Handler: visibilityhttp.NewRegisterNotificationPolicyEndpoint(visibilityhttp.UnconfiguredIntake{}, notificationPolicyRegistration)},
		{Pattern: "/visibility-catalogue-claim-eligibility-registrations", Handler: visibilityhttp.NewRegisterClaimEligibilityEndpoint(visibilityhttp.UnconfiguredIntake{}, claimEligibilityRegistration)},
		{Pattern: "/visibility-catalogue-claim-authorization-registrations", Handler: visibilityhttp.NewRegisterClaimAuthorizationEndpoint(visibilityhttp.UnconfiguredIntake{}, claimAuthorizationRegistration)},
		{Pattern: "/visibility-catalogue-disclosure-policy-registrations", Handler: visibilityhttp.NewRegisterDisclosurePolicyEndpoint(visibilityhttp.UnconfiguredIntake{}, disclosurePolicyRegistration)},
		// VE 案件侧三页（票 admin-skeleton-closure-batch/06）：分诊两册、案件单册、
		// 理赔追偿三册，与目录及追踪查阅同族同 Intake 变量。生产装配三口共用一个
		// CaseReview 读适配器（六方法一型），此处三参分收是为了让装配测试盖得住
		// 「某一口接错适配器」——参数分、实现合，两侧各取所需。
		{Pattern: "/exception-triage-records", Handler: visibilityhttp.NewQueryExceptionTriageRecordsEndpoint(trackingProjectionsIntake, exceptionTriageRecords)},
		{Pattern: "/exception-case-records", Handler: visibilityhttp.NewQueryExceptionCaseRecordsEndpoint(trackingProjectionsIntake, exceptionCaseRecords)},
		{Pattern: "/claims-recovery-records", Handler: visibilityhttp.NewQueryClaimsRecoveryRecordsEndpoint(trackingProjectionsIntake, claimsRecoveryRecords)},
		// 代收分户账册（票 admin-remainder-mechanism-batch/04）：分户账连派生余额与
		// 批次引用一次上列。册只一本，不设分派参数（裁决在端点文件头）；隔离读准入
		// 按同三条判据入格，随本上下文自己的 Intake 变量换值。
		{Pattern: "/collection-subledgers", Handler: collectionhttp.NewQueryCodSubledgersEndpoint(collectionCatalogueIntake, codSubledgers)},
		// 结算与核算四页（票 admin-skeleton-closure-batch/04）各立入口，四行共用本上下文
		// 自己的 Intake 变量：作用域形状同为租户一维，责任法人、结算账户与币种是账上的
		// 归属维不是查阅方身份，分设只会让装配点看起来能只配一半。哪几本册进哪个入口、
		// 资金冻结与运营结算余额为何不在其中，裁决在各构造函数的注释，此处不复述。
		{Pattern: "/settlement-charges", Handler: settlementhttp.NewQuerySettlementChargesEndpoint(settlementCatalogueIntake, settlementCharges)},
		{Pattern: "/settlement-statements", Handler: settlementhttp.NewQuerySettlementStatementsEndpoint(settlementCatalogueIntake, settlementStatements)},
		{Pattern: "/settlement-funds-applications", Handler: settlementhttp.NewQuerySettlementFundsApplicationsEndpoint(settlementCatalogueIntake, settlementFundsApplications)},
		{Pattern: "/settlement-operating-results", Handler: settlementhttp.NewQuerySettlementOperatingResultsEndpoint(settlementCatalogueIntake, settlementOperatingResults)},
		// 治理登记册三册一口（票 admin-skeleton-closure-batch/02）：治理无租户维是
		// 设计不是缺列（ADR-0083），Intake 因此是本上下文自己的一种准入形——隔离读
		// 启用与 SYN- 门禁同走一个开关，但开关值里的合成租户不进治理作用域。
		{Pattern: "/governance-registers", Handler: governancehttp.NewQueryGovernanceRegistersEndpoint(governanceRegistryIntake, governanceRegisters)},
	}
}

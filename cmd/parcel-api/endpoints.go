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
	disposition shipmenthttp.AuthorizedDispositionHandler,
	supplement shipmenthttp.SupplementHandler,
	amendment shipmenthttp.AmendmentHandler,
	reviewQueue shipmenthttp.AcceptanceReviewQueueReader,
	reviewJudgments shipmenthttp.RecordedJudgmentsReader,
	dispositionQueue shipmenthttp.AuthorizedDispositionQueueReader,
	labelTransactions shipmenthttp.LabelTransactionsReader,
	channelSelectionDecisions shipmenthttp.ChannelSelectionDecisionsReader,
	cancellation shipmenthttp.CancellationHandler,
	continuedAttemptDecisions shipmenthttp.ContinuedAttemptDecisionHandler,
	reception nodeopshttp.ReceptionHandler,
	nodeOperationsRecords nodeopshttp.ReviewCatalogueReader,
	delivery tfhttp.DeliveryHandler,
	transportFulfillmentRecords tfhttp.ReviewCatalogueReader,
	handoverScopeSummary tfhttp.HandoverScopeSummarizer,
	handover tfhttp.HandoverHandler,
	pickupRegistration tfhttp.PickupRegistrationHandler,
	pickupCorrection tfhttp.PickupCorrectionHandler,
	pickupAttempt tfhttp.PickupAttemptHandler,
	movementFact tfhttp.MovementFactHandler,
	segmentCloser tfhttp.SegmentCloser,
	dispatchTaskOpener tfhttp.DispatchTaskOpener,
	deliveryDispatchTrigger tfhttp.DeliveryDispatchTriggerer,
	loadAssigner tfhttp.LoadAssigner,
	participationEnder tfhttp.ParticipationEnder,
	credentialRegistration tfhttp.CredentialRegistrar,
	effectiveTimeRuleRegistration tfhttp.EffectiveTimeRuleRegistrar,
	externalTrackingFactReview tfhttp.ExternalTrackingFactReviewReader,
	effectiveTimeJudgment tfhttp.EffectiveTimeJudge,
	carrierPickupChain tfhttp.CarrierPickupChainReader,
	carrierPickupJudgment tfhttp.CarrierPickupJudge,
	masterDocumentRegistration tfhttp.MasterDocumentRegistrar,
	trackingViews visibilityhttp.TrackingViewReader,
	projectionViews visibilityhttp.OperationsProjectionReader,
	claims visibilityhttp.ClaimReceiver,
	results customshttp.ResultHandler,
	priceCards pricinghttp.PriceCardCatalogueReader,
	referenceSeries pricinghttp.ReferenceSeriesCatalogueReader,
	referenceSeriesCoverage pricinghttp.ReferenceSeriesCoverageReader,
	pendingSeriesEvaluations pricinghttp.PendingSeriesEvaluationReader,
	pricingEvaluations pricinghttp.EvaluationCatalogueReader,
	priceCardRegistration pricinghttp.PriceCardRegistrar,
	referenceSeriesRegistration pricinghttp.ReferenceSeriesRegistrar,
	referenceSeriesReview pricinghttp.ReferenceSeriesReviewer,
	referenceSeriesPreview pricinghttp.ReferenceSeriesPreviewer,
	referenceCatalogueRegistration pricinghttp.ReferenceCatalogueRegistrar,
	evaluationReplay pricinghttp.EvaluationReplayer,
	networkCatalog networkhttp.OperationsCatalogReader,
	routePlans networkhttp.RoutePlanCatalogueReader,
	networkCatalogRegistration networkhttp.CatalogRegistrar,
	complianceRules customshttp.RuleCatalogueReader,
	caseRegisters customshttp.CaseRegisterCatalogueReader,
	gateConditions customshttp.GateConditionCatalogueReader,
	portsPaths customshttp.PortsPathsCatalogueReader,
	credentials customshttp.CredentialCatalogueReader,
	dutyCollaborations customshttp.DutyCollaborationCatalogueReader,
	dutyVerifications customshttp.DutyVerificationCatalogueReader,
	interpretationRuleRegistration customshttp.InterpretationRuleRegistrar,
	gateCatalogRegistration customshttp.GateCatalogRegistrar,
	candidatePortRegistration customshttp.CandidatePortRegistrar,
	declarationPathRegistration customshttp.DeclarationPathRegistrar,
	caseRequirementRegistration customshttp.CaseRequirementRegistrar,
	regulatoryCredentialRegistration customshttp.RegulatoryCredentialRegistrar,
	dutyCollaborationRegistration customshttp.DutyCollaborationRegistrar,
	dutyPaymentVerificationRegistration customshttp.DutyPaymentVerificationRegistrar,
	serviceProducts commercialhttp.ServiceProductCatalogueReader,
	commercialPolicies commercialhttp.CommercialPolicyCatalogueReader,
	commercialRelations commercialhttp.CommercialRelationCatalogueReader,
	partyIdentities commercialhttp.PartyIdentityCatalogueReader,
	customerAccounts commercialhttp.CustomerAccountCatalogueReader,
	productChannelMappings commercialhttp.ProductChannelCatalogueReader,
	commercialPublication commercialhttp.CommercialAuthorityPublisher,
	commercialPublicationPreview commercialhttp.CommercialPublicationPreviewer,
	commercialPublicationDrafts commercialhttp.PublicationDraftOperator,
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
	exceptionDisclosureRulesRegistration visibilityhttp.ExceptionDisclosureRulesRegistrar,
	conflictSignalRuleRegistration visibilityhttp.ConflictSignalRuleRegistrar,
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
	channelSelectionDecisionIntake := shipmenthttp.ChannelSelectionDecisionQueryIntake(shipmenthttp.UnconfiguredIntake{})
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
		channelSelectionDecisionIntake = isolatedRead.channelSelectionDecisions
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
		// 受控关闭 / 重开决定两个命令口（票 label-channel/30）：同一只编排两条命令，运营侧写行，同挂字面量
		// UnconfiguredIntake{}；请求方与货主账户属 `PAR-INT-01`（实例半边），隔离读准入换不了写行。
		{Pattern: "/shipment-requests/continued-attempt-closures", Handler: shipmenthttp.NewFormControlledClosureEndpoint(shipmenthttp.UnconfiguredIntake{}, continuedAttemptDecisions)},
		{Pattern: "/shipment-requests/continued-attempt-reopenings", Handler: shipmenthttp.NewFormReopeningEndpoint(shipmenthttp.UnconfiguredIntake{}, continuedAttemptDecisions)},
		// 复核完成与主动拒绝两个命令口（票 09；ADR-0081 的命令面保留条款、ADR-0086）：
		// 与其余命令面同挂字面量 UnconfiguredIntake{}，隔离读准入换不了写行。
		{Pattern: "/shipment-requests/manual-review-completions", Handler: shipmenthttp.NewCompleteManualReviewEndpoint(shipmenthttp.UnconfiguredIntake{}, manualReview)},
		{Pattern: "/shipment-requests/rejections", Handler: shipmenthttp.NewRejectShipmentRequestEndpoint(shipmenthttp.UnconfiguredIntake{}, rejection)},
		// 授权处置命令口（票 sa-preacceptance-policy-view/04；ADR-0132）：授权角色对停在`等待授权处置`的
		// 委托选去向。运营侧写行，同挂字面量 UnconfiguredIntake{}；谁是处置人属 `PAR-INT-01`（实例半边）。
		{Pattern: "/shipment-requests/authorized-dispositions", Handler: shipmenthttp.NewDisposeShipmentRequestEndpoint(shipmenthttp.UnconfiguredIntake{}, disposition)},
		// 受控补充命令口（票 first-tenant-runway/09；ADR-0106 Decision 四）：客户在`已提交`委托上形成
		// 同一委托的新提交版本，编排随新版本落库同事务铸「新提交版本已形成」信封驱动续办。它是客户
		// 渠道的写行，同挂字面量 UnconfiguredIntake{}；谁能替哪个客户账户补充、基准版本怎么译，属
		// `PAR-INT-01` 接入契约（实例半边），隔离读准入与隔离提交放行都换不了这一行。
		{Pattern: "/shipment-requests/supplements", Handler: shipmenthttp.NewFormNewSubmissionVersionEndpoint(shipmenthttp.UnconfiguredIntake{}, supplement)},
		// 资料修订命令口（UC-PS-002；票 ps-port-remainder/04）：客户或其授权代表在`已接受`委托上形成客户原始
		// 资料新版本。客户渠道的写行，同挂字面量 UnconfiguredIntake{}；请求方与实际决定方怎么采信属
		// `BD-PS-009` / `PAR-INT-01`（实例半边），隔离读准入与隔离提交放行都换不了这一行。编排接的是
		// 未配置的授权与矩阵答复，越过 Intake 后如实停在授权未决——停点从「没有入口」变成「说得出停在哪」。
		{Pattern: "/shipment-requests/source-data-amendments", Handler: shipmenthttp.NewAmendCustomerSourceDataEndpoint(shipmenthttp.UnconfiguredIntake{}, amendment)},
		{Pattern: "/shipment-request-views", Handler: shipmenthttp.NewQueryShipmentRequestViewsEndpoint(shipmentViewsIntake, requestViews)},
		// 复核队列查阅（票 09）：委托查阅面的子集视图，Intake 沿用同一变量——隔离读
		// 准入（ADR-0078）启用时随委托查阅一起换值，不另立第二种准入形。
		{Pattern: "/acceptance-review-queue", Handler: shipmenthttp.NewQueryAcceptanceReviewQueueEndpoint(shipmentViewsIntake, reviewQueue, reviewJudgments)},
		// 授权处置队列查阅（票 sa-preacceptance-policy-view/04，ADR-0132 决定二）：同为委托查阅面的子集视图，
		// Intake 沿用同一变量；单份详情复用复核队列的 `?shipmentRequestId=` 分支，这里只有列表。
		{Pattern: "/authorized-disposition-queue", Handler: shipmenthttp.NewQueryAuthorizedDispositionQueueEndpoint(shipmentViewsIntake, dispositionQueue)},
		// 面单交易查阅（票 admin-skeleton-closure-batch/08，ADR-0084 决定七）：**另立一种
		// 准入形**而不是复用委托查阅那个变量。两者由同一个隔离读开关、同一个装配点换值
		// （决定七要的「沿用同一开关」），但接口不同：面单交易没有账户维可分，收一个必带
		// 账户维的作用域等于在类型上声称会按账户过滤而它不会。渠道墙未降前这一口读到的是
		// 空册，那是设计：写入方是渠道适配器，首发不进生产。
		{Pattern: "/label-transactions", Handler: shipmenthttp.NewQueryLabelTransactionsEndpoint(labelTransactionIntake, labelTransactions)},
		// 渠道择优决定查阅（票 label-channel/23）：并列冲突列表与按标识取一条共用一个端点、按 decisionId 分派。
		// 准入形同面单交易（只有租户维，本册没有账户维可分），自立 Intake 变量、随同一个隔离读开关换值；读口是
		// 决定登记册本尊（两个契约一只适配器）。择优编排今天无生产装配点，接线前这一口读到的是空册，空册是如实答案。
		{Pattern: "/channel-selection-decisions", Handler: shipmenthttp.NewQueryChannelSelectionDecisionsEndpoint(channelSelectionDecisionIntake, channelSelectionDecisions)},
		{Pattern: "/node-operations/receptions", Handler: nodeopshttp.NewReceiveDeliveredUnitEndpoint(nodeopshttp.UnconfiguredIntake{}, reception)},
		// 节点作业与运输履约查阅页（票 admin-skeleton-closure-batch/05）各一口按
		// registry 分派（NO 三册、TF 四册）：分派对应「一页里的页签」。命令端点在上，
		// 用的是字面量 UnconfiguredIntake{}；查阅行走本上下文自己的 Intake 变量，
		// 隔离读准入（ADR-0078）启用时只换查阅行，命令行换不了。
		{Pattern: "/node-operations-records", Handler: nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(nodeOperationsCatalogueIntake, nodeOperationsRecords)},
		{Pattern: "/transport-fulfillment/deliveries", Handler: tfhttp.NewRegisterEffectiveDeliveryEndpoint(tfhttp.UnconfiguredIntake{}, delivery)},
		{Pattern: "/transport-fulfillment/delivery-proof-corrections", Handler: tfhttp.NewCorrectDeliveryProofEndpoint(tfhttp.UnconfiguredIntake{}, delivery)},
		// 控制事实入口（票 tf-segment-lifecycle-closure/04）：交接一组（登记 + 更正）、揽收一组
		// （单对象登记 + 更正 + 多对象执行），按事实分组而不按 UC 分。它们是 CONTEXT 成立边界的来源事实，
		// 进段那道门（enterFulfillmentSegment）在生产上只从登记那几行走得到——接上之前它没有任何路。
		// 命令面，同挂字面量 UnconfiguredIntake{}；命令里带着段引用，这几行比交付更不能让隔离读
		// 开关换值：一条穿过去的请求会在段登记册上立出一个来源不明的实际履约段。
		// 揽收更正口（票 tf-segment-lifecycle-closure/08）落新版本回指前版、重交 PS 采认，不进段——段侧
		// 重派生另立票；它接的是自己那一格装配（assemble_offsite_pickup_correction.go），与首登共用一册。
		{Pattern: "/transport-fulfillment/handovers", Handler: tfhttp.NewRegisterTransportHandoverEndpoint(tfhttp.UnconfiguredIntake{}, handover)},
		{Pattern: "/transport-fulfillment/handover-corrections", Handler: tfhttp.NewCorrectTransportHandoverEndpoint(tfhttp.UnconfiguredIntake{}, handover)},
		{Pattern: "/transport-fulfillment/offsite-pickups", Handler: tfhttp.NewRegisterOffsitePickupEndpoint(tfhttp.UnconfiguredIntake{}, pickupRegistration)},
		{Pattern: "/transport-fulfillment/offsite-pickup-corrections", Handler: tfhttp.NewCorrectOffsitePickupEndpoint(tfhttp.UnconfiguredIntake{}, pickupCorrection)},
		{Pattern: "/transport-fulfillment/offsite-pickup-attempts", Handler: tfhttp.NewPerformOffsitePickupEndpoint(tfhttp.UnconfiguredIntake{}, pickupAttempt)},
		// 移动事实口（票 tf-segment-lifecycle-closure/05）：只收自营执行方的出发 / 移动 / 到达；外部承运
		// 轨迹**不从这里进**，走 TrackingSource 入站口的采纳执行器（label-channel/16 已落）。谁是自营
		// 执行方由 Intake 的认证结果说，渠道未就位前同挂字面量 UnconfiguredIntake{}。
		{Pattern: "/transport-fulfillment/movement-facts", Handler: tfhttp.NewRecordMovementFactEndpoint(tfhttp.UnconfiguredIntake{}, movementFact)},
		// TF 四个 admin 写面（ADR-0085，票 tf-segment-lifecycle-closure/07）：关段、建派送任务、装载分配、
		// 明确终止参与。它们是运营决定不是承运方回传口，所以路径取读面册名前缀 `transport-fulfillment-`
		// 而不是控制事实那组的 `/transport-fulfillment/...`。写准入不另立形，同挂字面量 UnconfiguredIntake{}。
		// 终止口只能铸终止那一路（tfhttp.ParticipationTermination 比应用命令窄），交付与交接两路是内部触发。
		{Pattern: "/transport-fulfillment-segment-closures", Handler: tfhttp.NewCloseFulfillmentSegmentEndpoint(tfhttp.UnconfiguredIntake{}, segmentCloser)},
		{Pattern: "/transport-fulfillment-dispatch-task-registrations", Handler: tfhttp.NewOpenDispatchTaskEndpoint(tfhttp.UnconfiguredIntake{}, dispatchTaskOpener)},
		// 末端派送任务内部触发执行器的生产入口（ADR-0114 决定二末句；票 tf-segment-lifecycle-closure/12「生产入口」）：谁按拍调、
		// 拍频多大属调用方（实例半边），同挂字面量 UnconfiguredIntake{} 如实答未配置；与上一行手工建任务是两件事。
		{Pattern: "/transport-fulfillment-delivery-dispatch-triggers", Handler: tfhttp.NewTriggerDeliveryDispatchEndpoint(tfhttp.UnconfiguredIntake{}, deliveryDispatchTrigger)},
		{Pattern: "/transport-fulfillment-load-assignment-registrations", Handler: tfhttp.NewFormLoadAssignmentEndpoint(tfhttp.UnconfiguredIntake{}, loadAssigner)},
		{Pattern: "/transport-fulfillment-participation-terminations", Handler: tfhttp.NewTerminateFulfillmentParticipationEndpoint(tfhttp.UnconfiguredIntake{}, participationEnder)},
		// 外部承运凭证登记两口（ADR-0085，票 label-channel/18）：登记一份凭证的首版，与对它此刻的当前版落
		// 一次作废 / 失效 / 替代。它们是运营登记不是承运方回传口，路径取读面册名前缀 `transport-fulfillment-`；
		// 改变口不叫 `-corrections`——作废、失效、替代改变的是适用关系而不是更正一个判断，原版本一字不动。
		// 写准入不另立形，同挂字面量 UnconfiguredIntake{}：一份凭证登进去就会被收编执行器用来把外部轨迹认到
		// 某个载运对象上，这两行比查阅行更不能让隔离读开关换值。
		{Pattern: "/transport-fulfillment-external-carrier-credential-registrations", Handler: tfhttp.NewRegisterExternalCarrierCredentialEndpoint(tfhttp.UnconfiguredIntake{}, credentialRegistration)},
		{Pattern: "/transport-fulfillment-external-carrier-credential-applicability-changes", Handler: tfhttp.NewChangeExternalCarrierCredentialApplicabilityEndpoint(tfhttp.UnconfiguredIntake{}, credentialRegistration)},
		// 有效时间规则登记（label-channel/19）与凭证两行同一格：一版规则登进去会让收编执行器替该源
		// 此后每一条素材形成有效时间，登记方身份没有可采信的渠道前 Intake 恒堵。
		{Pattern: "/transport-fulfillment-effective-time-rule-registrations", Handler: tfhttp.NewRegisterEffectiveTimeRuleEndpoint(tfhttp.UnconfiguredIntake{}, effectiveTimeRuleRegistration)},
		// 外部承运轨迹事实的有效时间判断面两口（ADR-0085，票 label-channel/21）。读口按（租户，轨迹源）上列当前版
		// （待判断 / 全部），是判断人的「该判哪几条」：零登记零编辑零披露，消费本上下文自己的存储读面，走运输履约
		// 查阅同一个 Intake 变量——隔离读准入（ADR-0078）启用时随查阅行一起换值。写口是所有者的显式判断（ADR-0102
		// 决定三第一种来源），一次判断就把事实交给 visibility-exception 进客户可见面，同挂字面量 UnconfiguredIntake{}：
		// 读开关换不了它。路径取读面册名前缀 `transport-fulfillment-`——两口都是运营侧动作，不是承运方回传口。
		{Pattern: "/transport-fulfillment-external-tracking-facts", Handler: tfhttp.NewQueryExternalTrackingFactsEndpoint(transportCatalogueIntake, externalTrackingFactReview)},
		{Pattern: "/transport-fulfillment-effective-time-judgments", Handler: tfhttp.NewJudgeEffectiveTimeEndpoint(tfhttp.UnconfiguredIntake{}, effectiveTimeJudgment)},
		// 实际承运商首次有效收寄的判断面两口（票 label-channel/31，ADR-0135 决定八）。读口按（租户，载运对象）上列整条
		// 收寄链，是判断人的「这个对象走到哪一版」：零登记零编辑零披露，消费本上下文自己的存储读面，走运输履约查阅同一个
		// Intake 变量。写口是判断方的显式读法，一次判断落的是控制事实——立段、结束取消权、交 parcel-shipment 形成终局，
		// 同挂字面量 UnconfiguredIntake{}：读开关换不了它。
		{Pattern: "/transport-fulfillment-carrier-first-effective-pickups", Handler: tfhttp.NewQueryCarrierFirstEffectivePickupsEndpoint(transportCatalogueIntake, carrierPickupChain)},
		{Pattern: "/transport-fulfillment-carrier-first-effective-pickup-judgments", Handler: tfhttp.NewJudgeCarrierFirstEffectivePickupEndpoint(tfhttp.UnconfiguredIntake{}, carrierPickupJudgment)},
		// 总单登记两口（ADR-0113 决定五；票 tf-carrier-master-document-register/01）：登记一份总单的首版，与对它此刻的
		// 当前版形成一次撤销 / 替代 / 关联重述。运营登记不是承运方回传口，路径取读面册名前缀 `transport-fulfillment-`；
		// 新版本口不叫 `-corrections`，理由同凭证。写准入不另立形，同挂字面量 UnconfiguredIntake{}：一份总单登进去就成了
		// parcel-pricing 主单级评价可引的身份（ADR-0111），这两行不能让隔离读开关换值。
		{Pattern: "/transport-fulfillment-carrier-master-document-registrations", Handler: tfhttp.NewRegisterCarrierMasterDocumentEndpoint(tfhttp.UnconfiguredIntake{}, masterDocumentRegistration)},
		{Pattern: "/transport-fulfillment-carrier-master-document-revisions", Handler: tfhttp.NewReviseCarrierMasterDocumentEndpoint(tfhttp.UnconfiguredIntake{}, masterDocumentRegistration)},
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
		// 覆盖地平线（票 pricing-reference-series-operations/05 第 1 项）：一条序列一行，
		// 答的是「还盖得住多久、谁欠一个动作」。时刻源在此处选定——组合根就是挑具体依赖
		// 的地方，端点自己收 CoverageClock 以便传输层测试注入固定时钟。
		{Pattern: "/pricing-reference-series-coverage", Handler: pricinghttp.NewQueryReferenceSeriesCoverageEndpoint(pricingCatalogueIntake, referenceSeriesCoverage, systemClock{})},
		// 被挂起评价联动（ADR-0105 Decision 五；票 05 第 2 项）：因序列未解析而待判断的评价数按序列种类分组，
		// 只读问题项子表；与覆盖地平线共用同一时刻源，摘要条并排两格说的才是同一刻。
		{Pattern: "/pricing-pending-series-evaluations", Handler: pricinghttp.NewQueryPendingSeriesEvaluationsEndpoint(pricingCatalogueIntake, pendingSeriesEvaluations, systemClock{})},
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
		// 序列登记前预览（票 pricing-reference-series-operations/08，ADR-0101 决定四）：不写库，
		// 但拟登本体要信封里的租户与登记责任方才立得住，等的与登记口是同一样东西，所以同挂
		// 字面量 UnconfiguredIntake{}，不走查阅行的 Intake 变量——隔离读放行装不进它（编译期）。
		{Pattern: "/pricing-reference-series-previews", Handler: pricinghttp.NewPreviewReferenceSeriesEndpoint(pricinghttp.UnconfiguredIntake{}, referenceSeriesPreview)},
		// 计价参考目录登记写面（ADR-0109 Decision 二，票 price-card-shape-gaps/01）：模板导入的命令行，
		// 同挂字面量 UnconfiguredIntake{}，判据同两个登记口。
		{Pattern: "/pricing-reference-catalogue-registrations", Handler: pricinghttp.NewRegisterReferenceCatalogueEndpoint(pricinghttp.UnconfiguredIntake{}, referenceCatalogueRegistration)},
		// 评价回放的治理触发面（ADR-0124 决定一，票 wiring-baseline-remainder/06）：治理动作也是命令行，
		// 同挂字面量 UnconfiguredIntake{}——触发者来自操作者信封，任何采信自报身份或替它填证据层级的
		// Intake 都不能有；路径按动词叫 `-replays`，判据同复核口叫 `-reviews`。回放结果不交结算，
		// 编排的依赖结构上就没有交付口。
		{Pattern: "/pricing-evaluation-replays", Handler: pricinghttp.NewReplayEvaluationEndpoint(pricinghttp.UnconfiguredIntake{}, evaluationReplay)},
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
		// 凭证、税费付款协作事项、税费付款核对三册（票 sa-cc/10）各立入口、不并进任何
		// registry 分派：凭证读签挂 customs-cases 页、协作与核对两签挂 customs-restrictions
		// 页——三册分落两页，且都不是既有分派封闭集里的册（案件配置四册 / 口岸路径两册），
		// 往那些集里加格等于让一个端点的参数集替两页说话。Intake 与关务运营读面同族同变量，
		// 三条判据同上（消费所属上下文存储读面、零持久化、作用域为运营侧授权结果）。
		{Pattern: "/customs-credentials", Handler: customshttp.NewQueryCredentialsEndpoint(complianceRulesIntake, credentials)},
		{Pattern: "/customs-duty-collaborations", Handler: customshttp.NewQueryDutyCollaborationsEndpoint(complianceRulesIntake, dutyCollaborations)},
		{Pattern: "/customs-duty-verifications", Handler: customshttp.NewQueryDutyVerificationsEndpoint(complianceRulesIntake, dutyVerifications)},
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
		// 凭证、税费付款协作事项、税费付款核对三册的在线登记口（ADR-0085 决定一，票 sa-cc/07
		// 步二；裁决「同族一致」三册都开）：写准入不另立形，同挂字面量 UnconfiguredIntake{}。
		// 路径的事物词取登记 CLI 的命令名（regulatory-credential / duty-collaboration /
		// duty-payment-verification），同一本册在 CLI 与端点两处不换词；不取读口的册名
		// （/customs-credentials 那组）——读口按册平铺，写口按命令命名，前五个登记口已是这么分的。
		// 协作与核对两口在生产上是同一只编排的两个方法，端点表仍各收一参：装配测试才盖得住
		// 「协作口接了核对编排」。外部资金事实没有在线口：它只经 settlement-accounting 的采用信封
		// 进 CC（ADR-0137 决定四）。
		{Pattern: "/customs-regulatory-credential-registrations", Handler: customshttp.NewRegisterRegulatoryCredentialEndpoint(customshttp.UnconfiguredIntake{}, regulatoryCredentialRegistration)},
		{Pattern: "/customs-duty-collaboration-registrations", Handler: customshttp.NewRegisterDutyCollaborationEndpoint(customshttp.UnconfiguredIntake{}, dutyCollaborationRegistration)},
		{Pattern: "/customs-duty-payment-verification-registrations", Handler: customshttp.NewRegisterDutyPaymentVerificationEndpoint(customshttp.UnconfiguredIntake{}, dutyPaymentVerificationRegistration)},
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
		// 运营操作者面发布路径四口（ADR-0126 Decision 三、四；票 admin-write-faces/08）：预览 + 载体录入 / 批准 / 发布。
		// 预览不落库，但拟录的壳要信封里的租户才立得住，等的与三个命令口是同一样东西，所以同挂字面量
		// UnconfiguredIntake{}、不走查阅行的 Intake 变量——隔离读放行装不进它（编译期，同 PP 预览口）。**批准那一口
		// 比其余更不能松**：批准者是审批职责规则的一半，任何采信自报身份的 Intake 都等于把那道门拆了。发布口不并进
		// `/commercial-publications`：那一口收受控批文（声明摘要 + 批准三格），这一口收载体引用，两口消费的用例不同。
		{Pattern: "/commercial-publication-previews", Handler: commercialhttp.NewPreviewCommercialPublicationEndpoint(commercialhttp.UnconfiguredIntake{}, commercialPublicationPreview)},
		{Pattern: "/commercial-publication-drafts", Handler: commercialhttp.NewSubmitPublicationDraftEndpoint(commercialhttp.UnconfiguredIntake{}, commercialPublicationDrafts)},
		{Pattern: "/commercial-publication-draft-approvals", Handler: commercialhttp.NewApprovePublicationDraftEndpoint(commercialhttp.UnconfiguredIntake{}, commercialPublicationDrafts)},
		{Pattern: "/commercial-publication-draft-publications", Handler: commercialhttp.NewPublishPublicationDraftEndpoint(commercialhttp.UnconfiguredIntake{}, commercialPublicationDrafts)},
		// 商业发布词表读口（票 admin-write-faces/20）：按 kind 答该册正文各封闭集的码，表单据此供下拉。它不要租户，
		// 却与四口同挂字面量 UnconfiguredIntake{}，理由不是预览口那条：它唯一的消费者是四口喂的表单，四口开不了时
		// 它单独开只让一张提交不了的表单多几行下拉；且它不读任何存储读面，不满足隔离读放行（ADR-0078）的入格判据
		// ——挂到 commercialCatalogueIntake 上会让 TestIsolatedReadAdmissionSwitchesOnlyOperationsReadLines 的二分
		// （放行 → 500 / 不放 → 403）多出一种 200 的形态，要加第三桶并改 ADR-0078 判据措辞，那是另一张票。
		{Pattern: "/commercial-publication-vocabularies", Handler: commercialhttp.NewQueryPublicationVocabularyEndpoint(commercialhttp.UnconfiguredIntake{})},
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
		// 异常披露规则（0023）与冲突信号规则（0025）两册写面（票 ve-disclosure-policy-view/02
		// 步二，口径「同族一致」）：路径种类词随读口 kind 原词 EXCEPTION_DISCLOSURE_RULE /
		// CONFLICT_SIGNAL_RULE 变形，写准入同上一律字面量 UnconfiguredIntake{}。
		{Pattern: "/visibility-catalogue-exception-disclosure-rule-registrations", Handler: visibilityhttp.NewRegisterExceptionDisclosureRulesEndpoint(visibilityhttp.UnconfiguredIntake{}, exceptionDisclosureRulesRegistration)},
		{Pattern: "/visibility-catalogue-conflict-signal-rule-registrations", Handler: visibilityhttp.NewRegisterConflictSignalRuleEndpoint(visibilityhttp.UnconfiguredIntake{}, conflictSignalRuleRegistration)},
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

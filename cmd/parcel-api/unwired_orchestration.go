package main

import (
	"context"
	"errors"
	"time"

	collectiondomain "go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	collectionports "go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	customsdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	customsports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	networkapp "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	networkdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	networkports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodeopsdomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	nodeopsports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	pricingports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	shipmentdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	shipmentports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	commercialapp "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	commercialdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	commercialports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	govports "go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
	settlementdomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	settlementports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	visibilitydomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	visibilityports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 各端点构造函数的第二参是应用编排；未配置 Intake 在它之前就拒了，因此本文件这几个
// 类型一个都到不了。它们存在只为回答「到不了的那一格填什么」。
//
// 不填 nil：nil 接口被调会 panic，路由层的 Recoverer 把它兜成一个没有稳定 code 的
// 500，读的人分不出那是进程坏了还是依赖坏了。显式交回错误则落在 ADR-0022 已有的那格
// ——没形成答案，5xx `NO_ANSWER_FORMED`，离线客户端照旧留队重发，而服务端记录里留着
// 下面这句话指明真正的原因。
//
// 它们也不是红线所禁的「开发用」实现：那条禁的是替租户拟一种认证方式、从请求内容铸造
// 来源信封（穿透 ADR-0003）。这几个类型不读请求、不构造任何东西、不作任何业务判断，
// 连一个零值结果都不交回。
//
// 换编排与换 Intake 是两笔可独立发生的工作：运行期未配置 Intake 拒在编排之前，装配期
// 把哪一格换成真编排不动 Intake。提交编排已按审计票 13 经真库与治理桥接真，撤回编排随
// UI 阶段 B 后端序列接真，NO 收寄、TF 交付、VE 索赔与 CC 外部结果四格编排按
// `.scratch/parcel-api-remaining-endpoint-wiring` 的接线票（01、02、04、05）接真，取消
// 编排随第十端点挂载接真（传输层随 UC-PS-006 切片先行落库），运营追踪查阅读口随
// ADR-0076 端点挂载接真库投影库，均由装配点入参交入，不再从本文件取。各格至此全部
// 接真：本文件全部类型只被装配测试用来钉「未配置面」的形状，生产装配没有任何一格再
// 从这里取。真渠道 Intake 就位那笔工作只替换 Intake 本身。
var errOrchestrationNotWired = errors.New("parcel-api: business orchestration is not wired; the unconfigured intake should have refused first")

type unwiredSubmission struct{}

func (unwiredSubmission) Handle(
	context.Context,
	shipmentapp.SubmitShipmentRequestCommand,
) (shipmentapp.SubmitShipmentRequestResult, error) {
	return shipmentapp.SubmitShipmentRequestResult{}, errOrchestrationNotWired
}

type unwiredWithdrawal struct{}

func (unwiredWithdrawal) Handle(
	context.Context,
	shipmentapp.WithdrawShipmentRequestCommand,
) (shipmentapp.WithdrawShipmentRequestResult, error) {
	return shipmentapp.WithdrawShipmentRequestResult{}, errOrchestrationNotWired
}

type unwiredReception struct{}

func (unwiredReception) Handle(
	context.Context,
	nodeopsapp.ReceiveDeliveredUnitCommand,
) (nodeopsapp.ReceiveDeliveredUnitResult, error) {
	return nodeopsapp.ReceiveDeliveredUnitResult{}, errOrchestrationNotWired
}

// unwiredNodeOperationsRecords 是节点作业查阅页三册读口的占位，方法表与
// nodeopsports.ReviewCatalogueRead 逐一对上（票 admin-skeleton-closure-batch/05）。
// 独立成形的理由同 unwiredPricingEvaluations：并进命令占位会盖不住「册接错适配器」。
type unwiredNodeOperationsRecords struct{}

func (unwiredNodeOperationsRecords) ListReceptions(
	context.Context,
	nodeopsdomain.TenantID,
	int,
) ([]nodeopsports.ReceptionCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredNodeOperationsRecords) ListUnidentifiedItems(
	context.Context,
	nodeopsdomain.TenantID,
	int,
) ([]nodeopsports.UnidentifiedItemCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredNodeOperationsRecords) ListConsolidationUnits(
	context.Context,
	nodeopsdomain.TenantID,
	int,
) ([]nodeopsports.ConsolidationUnitCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

// unwiredDelivery 一个类型顶两个端点：首登与更正共用 DeliveryHandler 这一个接口。
type unwiredDelivery struct{}

func (unwiredDelivery) Register(
	context.Context,
	tfapp.RegisterEffectiveDeliveryCommand,
) (tfapp.RegisterEffectiveDeliveryResult, error) {
	return tfapp.RegisterEffectiveDeliveryResult{}, errOrchestrationNotWired
}

func (unwiredDelivery) Correct(
	context.Context,
	tfapp.CorrectDeliveryProofCommand,
) (tfapp.RegisterEffectiveDeliveryResult, error) {
	return tfapp.RegisterEffectiveDeliveryResult{}, errOrchestrationNotWired
}

// unwiredHandover 一个类型顶两个端点：首登与更正共用 HandoverHandler（票
// tf-segment-lifecycle-closure/04），理由同 unwiredDelivery。
type unwiredHandover struct{}

func (unwiredHandover) Register(
	context.Context,
	tfapp.RegisterTransportHandoverCommand,
) (tfapp.RegisterTransportHandoverResult, error) {
	return tfapp.RegisterTransportHandoverResult{}, errOrchestrationNotWired
}

func (unwiredHandover) Correct(
	context.Context,
	tfapp.CorrectTransportHandoverCommand,
) (tfapp.RegisterTransportHandoverResult, error) {
	return tfapp.RegisterTransportHandoverResult{}, errOrchestrationNotWired
}

// unwiredPickupRegistration 与 unwiredPickupAttempt 分立：揽收登记与揽收执行是两条编排、两个
// 处理器接口，合成一个会让装配测试盖不住「单对象口接了多对象编排」。
type unwiredPickupRegistration struct{}

func (unwiredPickupRegistration) Register(
	context.Context,
	tfapp.RegisterOffsitePickupCommand,
) (tfapp.RegisterOffsitePickupResult, error) {
	return tfapp.RegisterOffsitePickupResult{}, errOrchestrationNotWired
}

// unwiredPickupCorrection 是揽收更正口的编排占位（票 tf-segment-lifecycle-closure/08）。与首登占位分立，
// 随传输层的两个接口：生产侧是同一条编排的两个入口、两格装配。
type unwiredPickupCorrection struct{}

func (unwiredPickupCorrection) Correct(
	context.Context,
	tfapp.CorrectOffsitePickupCommand,
) (tfapp.RegisterOffsitePickupResult, error) {
	return tfapp.RegisterOffsitePickupResult{}, errOrchestrationNotWired
}

type unwiredPickupAttempt struct{}

func (unwiredPickupAttempt) Handle(
	context.Context,
	tfapp.PerformOffsitePickupCommand,
) (tfapp.PerformOffsitePickupResult, error) {
	return tfapp.PerformOffsitePickupResult{}, errOrchestrationNotWired
}

// unwiredMovementFact 是移动事实口的编排占位（票 tf-segment-lifecycle-closure/05）。
type unwiredMovementFact struct{}

func (unwiredMovementFact) Record(
	context.Context,
	tfapp.RecordMovementFactCommand,
) (tfapp.RecordMovementFactResult, error) {
	return tfapp.RecordMovementFactResult{}, errOrchestrationNotWired
}

// TF 四个 admin 写面的编排占位（票 tf-segment-lifecycle-closure/07）。四个类型分立，随生产侧的四个事务
// 包装：合成一个会让装配测试盖不住「某一口接错了编排」。
type unwiredSegmentCloser struct{}

func (unwiredSegmentCloser) Close(
	context.Context,
	tfapp.CloseFulfillmentSegmentCommand,
) (tfapp.CloseFulfillmentSegmentResult, error) {
	return tfapp.CloseFulfillmentSegmentResult{}, errOrchestrationNotWired
}

type unwiredDispatchTaskOpener struct{}

func (unwiredDispatchTaskOpener) Open(
	context.Context,
	tfapp.OpenDispatchTaskCommand,
) (tfapp.OpenDispatchTaskResult, error) {
	return tfapp.OpenDispatchTaskResult{}, errOrchestrationNotWired
}

type unwiredLoadAssigner struct{}

func (unwiredLoadAssigner) Form(
	context.Context,
	tfapp.FormLoadAssignmentCommand,
) (tfapp.FormLoadAssignmentResult, error) {
	return tfapp.FormLoadAssignmentResult{}, errOrchestrationNotWired
}

type unwiredParticipationEnder struct{}

func (unwiredParticipationEnder) End(
	context.Context,
	tfapp.EndFulfillmentParticipationCommand,
) (tfapp.EndFulfillmentParticipationResult, error) {
	return tfapp.EndFulfillmentParticipationResult{}, errOrchestrationNotWired
}

// unwiredCredentialRegistration 一个类型顶两个端点：凭证首登与改变适用关系共用 CredentialRegistrar
// （票 label-channel/18）。
type unwiredCredentialRegistration struct{}

func (unwiredCredentialRegistration) Register(
	context.Context,
	tfapp.RegisterExternalCarrierCredentialCommand,
) (tfapp.RegisterExternalCarrierCredentialResult, error) {
	return tfapp.RegisterExternalCarrierCredentialResult{}, errOrchestrationNotWired
}

func (unwiredCredentialRegistration) ChangeApplicability(
	context.Context,
	tfapp.ChangeCredentialApplicabilityCommand,
) (tfapp.RegisterExternalCarrierCredentialResult, error) {
	return tfapp.RegisterExternalCarrierCredentialResult{}, errOrchestrationNotWired
}

// unwiredEffectiveTimeRuleRegistration 是轨迹源有效时间规则登记口的占位（票 label-channel/19）。
type unwiredEffectiveTimeRuleRegistration struct{}

func (unwiredEffectiveTimeRuleRegistration) Register(
	context.Context,
	tfapp.RegisterEffectiveTimeRuleCommand,
) (tfapp.RegisterEffectiveTimeRuleResult, error) {
	return tfapp.RegisterEffectiveTimeRuleResult{}, errOrchestrationNotWired
}

// unwiredExternalTrackingFactReview 是外部承运轨迹事实当前版读口的占位（票 label-channel/21 读半边）。
// 读不回交回稳定错误而不是空册：空册是「这家源此刻没有待判断的事实」这一真答案，一次读故障顶成它
// 会让两态在页面上同形。
type unwiredExternalTrackingFactReview struct{}

func (unwiredExternalTrackingFactReview) ListCurrentExternalTrackingFacts(
	context.Context,
	tfdomain.TenantID,
	tfdomain.TrackingSourceReference,
	tfports.EffectiveTimeReviewFilter,
	int,
) ([]tfports.ExternalTrackingFactReviewRow, error) {
	return nil, errOrchestrationNotWired
}

// unwiredEffectiveTimeJudgment 是有效时间显式判断口的编排占位（票 label-channel/21 写半边）。
type unwiredEffectiveTimeJudgment struct{}

func (unwiredEffectiveTimeJudgment) Judge(
	context.Context,
	tfapp.JudgeEffectiveTimeCommand,
) (tfapp.JudgeEffectiveTimeResult, error) {
	return tfapp.JudgeEffectiveTimeResult{}, errOrchestrationNotWired
}

// unwiredTransportFulfillmentRecords 是运输履约查阅页四册读口的占位，方法表与
// tfports.ReviewCatalogueRead 逐一对上（票 admin-skeleton-closure-batch/05）。
type unwiredTransportFulfillmentRecords struct{}

func (unwiredTransportFulfillmentRecords) ListTransportSchedules(
	context.Context,
	tfdomain.TenantID,
	int,
) ([]tfports.TransportScheduleCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredTransportFulfillmentRecords) ListCapacityPools(
	context.Context,
	tfdomain.TenantID,
	int,
) ([]tfports.CapacityPoolCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredTransportFulfillmentRecords) ListTransportHandovers(
	context.Context,
	tfdomain.TenantID,
	int,
) ([]tfports.TransportHandoverCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredTransportFulfillmentRecords) ListEffectiveDeliveries(
	context.Context,
	tfdomain.TenantID,
	int,
) ([]tfports.EffectiveDeliveryCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

// unwiredHandoverScopeSummary 是交接范围汇总读用例的占位，方法表与
// tfhttp.HandoverScopeSummarizer 对上（票 admin-web-audit-followups/06）。
//
// 它填的是**应用读用例**那一格而不是读口，因此不能照读口那样答一个零值结果：
// SummarizeHandoverScopeResult 的零值 outcome 是 Invalid，端点会把它判成
// UNNAMED_OUTCOME 的 5xx——那条路径本是用来抓「应用层漏了一格没具名」的，占位走
// 上去等于把一次未接线伪装成一处结果代数缺口。交回稳定错误落在 NO_ANSWER_FORMED，
// 与其余占位同形。
type unwiredHandoverScopeSummary struct{}

func (unwiredHandoverScopeSummary) Summarize(
	context.Context,
	tfapp.SummarizeHandoverScopeQuery,
) (tfapp.SummarizeHandoverScopeResult, error) {
	return tfapp.SummarizeHandoverScopeResult{}, errOrchestrationNotWired
}

// unwiredTrackingViews 只被装配测试使用：生产装配（main）把 VE 真库读适配器交进装配
// 点，这里的占位让「未配置面」测试不必开库。填法照旧：读不回按 ADR-0022 是「没形成
// 答案」的 5xx，绝不能顶成 VIEW_NOT_FOUND——那会把一次进程故障伪装成「查无此件」的
// 终局答案。
type unwiredTrackingViews struct{}

func (unwiredTrackingViews) FindCurrent(
	context.Context,
	visibilitydomain.TenantID,
	visibilitydomain.CustomerAccountReference,
	visibilitydomain.TrackedParcelReference,
) (visibilitydomain.CustomerTrackingView, bool, error) {
	return visibilitydomain.CustomerTrackingView{}, false, errOrchestrationNotWired
}

// unwiredRequestViews 只被装配测试使用：生产装配（main）把真库读适配器交进装配点，
// 这里的占位让「未配置面」测试不必开库。填法同 unwiredTrackingViews——读不回是
// 「没形成答案」的 5xx，绝不顶成一个空列表或统一不可见。
type unwiredRequestViews struct{}

func (unwiredRequestViews) ListVisible(
	context.Context,
	shipmentdomain.AuthorizedQueryScope,
	int,
) ([]shipmentports.ShipmentRequestSummaryRecord, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredRequestViews) FindVisibleByID(
	context.Context,
	shipmentdomain.AuthorizedQueryScope,
	shipmentdomain.ShipmentRequestID,
) (shipmentports.ShipmentRequestDetailRecord, bool, error) {
	return shipmentports.ShipmentRequestDetailRecord{}, false, errOrchestrationNotWired
}

type unwiredManualReview struct{}

func (unwiredManualReview) Handle(
	context.Context,
	shipmentapp.CompleteManualReviewCommand,
) (shipmentapp.CompleteManualReviewResult, error) {
	return shipmentapp.CompleteManualReviewResult{}, errOrchestrationNotWired
}

type unwiredRejection struct{}

func (unwiredRejection) Handle(
	context.Context,
	shipmentapp.RejectShipmentRequestCommand,
) (shipmentapp.RejectShipmentRequestResult, error) {
	return shipmentapp.RejectShipmentRequestResult{}, errOrchestrationNotWired
}

// unwiredSupplement 是受控补充命令口的编排占位（票 first-tenant-runway/09，ADR-0106 Decision 四）。
// 填法同其余命令占位：不交回零值业务答案——FormNewSubmissionVersionResult 的零值 outcome 是 Invalid，
// 端点会把它判成 UNNAMED_OUTCOME，那条路径本是用来抓「应用层漏了一格没具名」的；稳定错误让「越过了
// Intake」可观察为 NO_ANSWER_FORMED。
type unwiredSupplement struct{}

func (unwiredSupplement) Handle(
	context.Context,
	shipmentapp.FormNewSubmissionVersionCommand,
) (shipmentapp.FormNewSubmissionVersionResult, error) {
	return shipmentapp.FormNewSubmissionVersionResult{}, errOrchestrationNotWired
}

// unwiredReviewQueue 是复核队列读口的占位（票 09）。生产装配交入的是委托查阅适配器
// 本尊（同表同作用域纪律）；这里独立成形，装配测试才盖得住「队列口接错适配器」。
type unwiredReviewQueue struct{}

func (unwiredReviewQueue) ListAwaitingManualReview(
	context.Context,
	shipmentdomain.AuthorizedQueryScope,
	int,
) ([]shipmentports.AcceptanceReviewQueueRecord, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredReviewQueue) FindVisibleByID(
	context.Context,
	shipmentdomain.AuthorizedQueryScope,
	shipmentdomain.ShipmentRequestID,
) (shipmentports.ShipmentRequestDetailRecord, bool, error) {
	return shipmentports.ShipmentRequestDetailRecord{}, false, errOrchestrationNotWired
}

// unwiredLabelTransactions 是面单交易查阅读口的占位（票 admin-skeleton-closure-batch/08）。
// 生产装配交入的是本上下文自己的面单交易读适配器；独立成形，装配测试才盖得住「这一口
// 接错了适配器」。**读不回交回稳定错误而不是空册**：渠道墙未降前空册是真答案，一次读故障
// 顶成空册会让「登记册确实没有行」与「库连不上」在页面上长得一模一样。
type unwiredLabelTransactions struct{}

func (unwiredLabelTransactions) ListLabelTransactions(
	context.Context,
	shipmentdomain.TenantID,
	int,
) ([]shipmentports.LabelTransactionRecord, error) {
	return nil, errOrchestrationNotWired
}

// unwiredChannelSelectionDecisions 是渠道择优决定查阅读口的占位（票 label-channel/23）；读不回交回稳定错误
// 而不是空册，理由同 unwiredLabelTransactions。
type unwiredChannelSelectionDecisions struct{}

func (unwiredChannelSelectionDecisions) ListTiedChannelSelectionDecisions(
	context.Context,
	shipmentdomain.TenantID,
	shipmentports.TiedChannelSelectionFilter,
	int,
) ([]shipmentdomain.ChannelSelectionDecision, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredChannelSelectionDecisions) FindChannelSelectionDecision(
	context.Context,
	shipmentdomain.TenantID,
	shipmentdomain.ChannelSelectionDecisionID,
) (shipmentdomain.ChannelSelectionDecision, bool, error) {
	return shipmentdomain.ChannelSelectionDecision{}, false, errOrchestrationNotWired
}

type unwiredReviewJudgments struct{}

func (unwiredReviewJudgments) LoadRecordedJudgments(
	context.Context,
	shipmentdomain.TenantID,
	shipmentdomain.ShipmentRequestID,
) (shipmentports.RecordedJudgments, error) {
	return shipmentports.RecordedJudgments{}, errOrchestrationNotWired
}

// unwiredProjectionViews 只被装配测试使用：生产装配（main）把真库投影读适配器交进
// 装配点。填法同 unwiredTrackingViews——读不回按 ADR-0022 是「没形成答案」的 5xx，
// 绝不顶成空列表或「投影未形成」。
type unwiredProjectionViews struct{}

func (unwiredProjectionViews) ListCurrent(
	context.Context,
	visibilitydomain.TenantID,
	int,
) ([]visibilitydomain.TrackingProjection, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredProjectionViews) FindCurrent(
	context.Context,
	visibilitydomain.TenantID,
	visibilitydomain.TrackedParcelReference,
) (visibilitydomain.TrackingProjection, bool, error) {
	return visibilitydomain.TrackingProjection{}, false, errOrchestrationNotWired
}

func (unwiredProjectionViews) FindByVersion(
	context.Context,
	visibilitydomain.TenantID,
	visibilitydomain.ProjectionVersionID,
) (visibilitydomain.TrackingProjection, bool, error) {
	return visibilitydomain.TrackingProjection{}, false, errOrchestrationNotWired
}

// 主数据目录的 unwired* 同样只供装配测试。生产 main 为同一上下文的端点交入同一只
// 真库适配器；这里按上下文合成替身，既钉住接口形状，也避免把一次读故障伪装成空目录。
type unwiredPricingCatalogue struct{}

func (unwiredPricingCatalogue) ListPriceCards(
	context.Context,
	pricingdomain.TenantID,
	int,
) ([]pricingports.PriceCardCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredPricingCatalogue) ListReferenceSeries(
	context.Context,
	pricingdomain.TenantID,
	int,
) ([]pricingports.ReferenceSeriesCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

// unwiredReferenceSeriesCoverage 是覆盖地平线读口的占位（票
// pricing-reference-series-operations/05 切片 05a）。不并进 unwiredPricingCatalogue：
// 生产装配点上它是独立适配器（那两口只读版本表一张，这一口连版本与复核两张），并成一个
// 会让装配测试盖不住「这本册接错了适配器」这一格——判据同下面评价册那条。
type unwiredReferenceSeriesCoverage struct{}

func (unwiredReferenceSeriesCoverage) ListReferenceSeriesCoverage(
	context.Context,
	pricingdomain.TenantID,
	time.Time,
	int,
) ([]pricingports.ReferenceSeriesCoverageRow, error) {
	return nil, errOrchestrationNotWired
}

// unwiredPricingEvaluations 是评价册列表读口的占位，方法表与
// pricingports.EvaluationCatalogueRead 逐一对上（票 admin-skeleton-closure-batch/03）。
// 不并进 unwiredPricingCatalogue：生产装配点上评价册是独立适配器，并成一个会让装配
// 测试盖不住「这本册接错了适配器」这一格（判据同结算四占位）。
type unwiredPricingEvaluations struct{}

func (unwiredPricingEvaluations) ListEvaluations(
	context.Context,
	pricingdomain.TenantID,
	int,
) ([]pricingports.EvaluationCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

// 价卡与序列登记两格的命令占位（ADR-0085，票 admin-write-faces/01）：与其余命令占位
// 同形——不交回零值业务答案，稳定错误让「越过了 Intake」在传输层可观察为
// NO_ANSWER_FORMED。两个类型不合并，判据同生产侧的两个事务包装。
type unwiredPriceCardRegistration struct{}

func (unwiredPriceCardRegistration) Handle(
	context.Context,
	pricingapp.RegisterPriceCardCommand,
) (pricingapp.RegisterPriceCardOutcome, error) {
	return pricingapp.RegisterPriceCardOutcomeInvalid, errOrchestrationNotWired
}

type unwiredReferenceSeriesRegistration struct{}

func (unwiredReferenceSeriesRegistration) Handle(
	context.Context,
	pricingapp.RegisterReferenceSeriesCommand,
) (pricingapp.RegisterReferenceSeriesOutcome, error) {
	return pricingapp.RegisterReferenceSeriesOutcomeInvalid, errOrchestrationNotWired
}

// unwiredReferenceSeriesReview 是序列版本复核的命令占位（票
// pricing-reference-series-operations/04）。不与两个登记占位合并，判据同生产侧的三个
// 事务包装：合成一个会让装配测试盖不住「复核端点接了登记编排」。
type unwiredReferenceSeriesReview struct{}

func (unwiredReferenceSeriesReview) Handle(
	context.Context,
	pricingapp.ReviewReferenceSeriesCommand,
) (pricingapp.ReviewReferenceSeriesOutcome, error) {
	return pricingapp.ReviewReferenceSeriesOutcomeInvalid, errOrchestrationNotWired
}

// unwiredReferenceSeriesPreview 是序列登记前预览的占位（票 pricing-reference-series-operations/08）。
// 判据同上：不交回零值预览，稳定错误让「越过了 Intake」可观察为 NO_ANSWER_FORMED。
type unwiredReferenceSeriesPreview struct{}

func (unwiredReferenceSeriesPreview) Handle(
	context.Context,
	pricingapp.PreviewReferenceSeriesCommand,
) (pricingapp.ReferenceSeriesPreview, error) {
	return pricingapp.ReferenceSeriesPreview{}, errOrchestrationNotWired
}

// unwiredReferenceCatalogueRegistration 是计价参考目录登记的命令占位（票 price-card-shape-gaps/01）。
// 不与序列登记占位合并，判据同生产侧的事务包装。
type unwiredReferenceCatalogueRegistration struct{}

func (unwiredReferenceCatalogueRegistration) Handle(
	context.Context,
	pricingapp.RegisterReferenceCatalogueCommand,
) (pricingapp.RegisterReferenceCatalogueOutcome, error) {
	return pricingapp.RegisterReferenceCatalogueOutcomeInvalid, errOrchestrationNotWired
}

// 网络七族登记的命令占位（票 admin-write-faces/02 切片 02a）。七族一个类型，随生产侧
// 的 transactionalNetworkCatalogRegistration：传输层只要一个 CatalogRegistrar，拆成七个
// 换不来第二道保障。
type unwiredNetworkCatalogRegistration struct{}

func (unwiredNetworkCatalogRegistration) RegisterNodeVersion(
	context.Context,
	networkapp.RegisterNodeVersionCommand,
) (networkapp.RegisterCatalogResult, error) {
	return networkapp.RegisterCatalogResult{}, errOrchestrationNotWired
}

func (unwiredNetworkCatalogRegistration) RegisterConnectionVersion(
	context.Context,
	networkapp.RegisterConnectionVersionCommand,
) (networkapp.RegisterCatalogResult, error) {
	return networkapp.RegisterCatalogResult{}, errOrchestrationNotWired
}

func (unwiredNetworkCatalogRegistration) RegisterLineVersion(
	context.Context,
	networkapp.RegisterLineVersionCommand,
) (networkapp.RegisterCatalogResult, error) {
	return networkapp.RegisterCatalogResult{}, errOrchestrationNotWired
}

func (unwiredNetworkCatalogRegistration) RegisterServiceAreaVersion(
	context.Context,
	networkapp.RegisterServiceAreaVersionCommand,
) (networkapp.RegisterCatalogResult, error) {
	return networkapp.RegisterCatalogResult{}, errOrchestrationNotWired
}

func (unwiredNetworkCatalogRegistration) RegisterServiceCalendarVersion(
	context.Context,
	networkapp.RegisterServiceCalendarVersionCommand,
) (networkapp.RegisterCatalogResult, error) {
	return networkapp.RegisterCatalogResult{}, errOrchestrationNotWired
}

func (unwiredNetworkCatalogRegistration) RegisterAvailabilityAdjustment(
	context.Context,
	networkapp.RegisterAvailabilityAdjustmentCommand,
) (networkapp.RegisterCatalogResult, error) {
	return networkapp.RegisterCatalogResult{}, errOrchestrationNotWired
}

func (unwiredNetworkCatalogRegistration) RegisterRouteStrategyVersion(
	context.Context,
	networkapp.RegisterRouteStrategyVersionCommand,
) (networkapp.RegisterCatalogResult, error) {
	return networkapp.RegisterCatalogResult{}, errOrchestrationNotWired
}

// 关务四类配置登记的命令占位（票 admin-write-faces/02 切片 02b）。四个类型分立，随
// 生产侧的四个事务包装：合成一个会让装配测试盖不住「某一格接错了编排」。
type unwiredInterpretationRuleRegistration struct{}

func (unwiredInterpretationRuleRegistration) Handle(
	context.Context,
	customsapp.RegisterInterpretationRuleCommand,
) (customsapp.CaseConfigurationOutcome, error) {
	return customsapp.CaseConfigurationOutcomeInvalid, errOrchestrationNotWired
}

type unwiredGateCatalogRegistration struct{}

func (unwiredGateCatalogRegistration) Handle(
	context.Context,
	customsapp.RegisterGateCatalogCommand,
) (customsapp.CaseConfigurationOutcome, error) {
	return customsapp.CaseConfigurationOutcomeInvalid, errOrchestrationNotWired
}

type unwiredCandidatePortRegistration struct{}

func (unwiredCandidatePortRegistration) Handle(
	context.Context,
	customsapp.RegisterCandidatePortCommand,
) (customsapp.CaseConfigurationOutcome, error) {
	return customsapp.CaseConfigurationOutcomeInvalid, errOrchestrationNotWired
}

type unwiredDeclarationPathRegistration struct{}

func (unwiredDeclarationPathRegistration) Handle(
	context.Context,
	customsapp.RegisterDeclarationPathCommand,
) (customsapp.CaseConfigurationOutcome, error) {
	return customsapp.CaseConfigurationOutcomeInvalid, errOrchestrationNotWired
}

type unwiredCaseRequirementRegistration struct{}

func (unwiredCaseRequirementRegistration) Handle(
	context.Context,
	customsapp.RegisterCaseRequirementRuleCommand,
) (customsapp.CaseConfigurationOutcome, error) {
	return customsapp.CaseConfigurationOutcomeInvalid, errOrchestrationNotWired
}

// VE 六类配置登记的命令占位（票 admin-write-faces/02 切片 02d）。六个类型分立不是抄
// 六遍：六个 Registrar 契约的方法同名 `Handle` 而命令类型互不相同，一个类型实现不了
// 六个。
type unwiredMilestoneMappingRegistration struct{}

func (unwiredMilestoneMappingRegistration) Handle(
	context.Context,
	visibilityapp.RegisterMilestoneMappingCommand,
) (visibilityapp.RegisterCatalogResult, error) {
	return visibilityapp.RegisterCatalogResult{}, errOrchestrationNotWired
}

type unwiredTriageRulesRegistration struct{}

func (unwiredTriageRulesRegistration) Handle(
	context.Context,
	visibilityapp.RegisterTriageRulesCommand,
) (visibilityapp.RegisterCatalogResult, error) {
	return visibilityapp.RegisterCatalogResult{}, errOrchestrationNotWired
}

type unwiredNotificationPolicyRegistration struct{}

func (unwiredNotificationPolicyRegistration) Handle(
	context.Context,
	visibilityapp.RegisterNotificationPolicyCommand,
) (visibilityapp.RegisterCatalogResult, error) {
	return visibilityapp.RegisterCatalogResult{}, errOrchestrationNotWired
}

type unwiredClaimEligibilityRegistration struct{}

func (unwiredClaimEligibilityRegistration) Handle(
	context.Context,
	visibilityapp.RegisterClaimEligibilityCommand,
) (visibilityapp.RegisterCatalogResult, error) {
	return visibilityapp.RegisterCatalogResult{}, errOrchestrationNotWired
}

type unwiredClaimAuthorizationRegistration struct{}

func (unwiredClaimAuthorizationRegistration) Handle(
	context.Context,
	visibilityapp.RegisterClaimAuthorizationCommand,
) (visibilityapp.RegisterCatalogResult, error) {
	return visibilityapp.RegisterCatalogResult{}, errOrchestrationNotWired
}

type unwiredDisclosurePolicyRegistration struct{}

func (unwiredDisclosurePolicyRegistration) Handle(
	context.Context,
	visibilityapp.RegisterDisclosurePolicyCommand,
) (visibilityapp.RegisterCatalogResult, error) {
	return visibilityapp.RegisterCatalogResult{}, errOrchestrationNotWired
}

// 商业八类配置写面的命令占位（票 admin-write-faces/02 切片 02c）。按族分三个类型，随
// 生产侧的三个事务包装：同族的方法各自具名，一个类型装得下全族。
type unwiredCommercialPublication struct{}

func (unwiredCommercialPublication) Handle(
	context.Context,
	commercialapp.PublishCommercialAuthorityCommand,
) (commercialapp.PublishCommercialAuthorityResult, error) {
	return commercialapp.PublishCommercialAuthorityResult{}, errOrchestrationNotWired
}

type unwiredPartyIdentityRegistration struct{}

func (unwiredPartyIdentityRegistration) RegisterBusinessParty(
	context.Context,
	commercialapp.RegisterBusinessPartyCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialapp.PartyRegistryResult{}, errOrchestrationNotWired
}

func (unwiredPartyIdentityRegistration) RegisterLegalEntity(
	context.Context,
	commercialapp.RegisterLegalEntityCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialapp.PartyRegistryResult{}, errOrchestrationNotWired
}

func (unwiredPartyIdentityRegistration) RegisterCustomerAccount(
	context.Context,
	commercialapp.RegisterCustomerAccountCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialapp.PartyRegistryResult{}, errOrchestrationNotWired
}

func (unwiredPartyIdentityRegistration) RegisterRelationship(
	context.Context,
	commercialapp.RegisterPartyRelationshipCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialapp.PartyRegistryResult{}, errOrchestrationNotWired
}

func (unwiredPartyIdentityRegistration) Deactivate(
	context.Context,
	commercialapp.DeactivatePartyIdentityCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialapp.PartyRegistryResult{}, errOrchestrationNotWired
}

type unwiredProductChannelRegistration struct{}

func (unwiredProductChannelRegistration) RegisterServiceProductForm(
	context.Context,
	commercialapp.RegisterServiceProductFormCommand,
) (commercialapp.ProductChannelResult, error) {
	return commercialapp.ProductChannelResult{}, errOrchestrationNotWired
}

func (unwiredProductChannelRegistration) RegisterMapping(
	context.Context,
	commercialapp.RegisterProductChannelMappingCommand,
) (commercialapp.ProductChannelResult, error) {
	return commercialapp.ProductChannelResult{}, errOrchestrationNotWired
}

type unwiredChannelAccountUseRegistration struct{}

func (unwiredChannelAccountUseRegistration) Register(
	context.Context,
	commercialapp.RegisterChannelAccountUseCommand,
) (commercialapp.ChannelAccountUseResult, error) {
	return commercialapp.ChannelAccountUseResult{}, errOrchestrationNotWired
}

func (unwiredChannelAccountUseRegistration) Revoke(
	context.Context,
	commercialapp.RevokeChannelAccountUseCommand,
) (commercialapp.ChannelAccountUseResult, error) {
	return commercialapp.ChannelAccountUseResult{}, errOrchestrationNotWired
}

type unwiredNetworkCatalogue struct{}

func (unwiredNetworkCatalogue) ListNodeVersions(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.NodeDefinitionVersion, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredNetworkCatalogue) ListConnectionVersions(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.ConnectionDefinitionVersion, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredNetworkCatalogue) ListLineVersions(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.LineDefinitionVersion, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredNetworkCatalogue) ListServiceAreaVersions(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.ServiceAreaDefinitionVersion, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredNetworkCatalogue) ListServiceCalendarVersions(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.ServiceCalendarDefinitionVersion, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredNetworkCatalogue) ListAvailabilityAdjustments(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.AvailabilityAdjustmentStatement, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredNetworkCatalogue) ListRouteStrategyVersions(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.RouteStrategyDefinitionVersion, error) {
	return nil, errOrchestrationNotWired
}

// unwiredRoutePlans 是路由判断两册列表读口的占位，方法表与
// networkports.RoutePlanCatalogueRead 逐一对上（票 admin-skeleton-closure-batch/03）。
// 独立成形的理由同 unwiredPricingEvaluations。
type unwiredRoutePlans struct{}

func (unwiredRoutePlans) ListInitialRoutes(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.InitialRouteCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredRoutePlans) ListRouteReassessments(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.RouteReassessmentCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

// unwiredGovernanceRegisters 是治理登记册读口的占位，方法表与
// govports.GovernanceRegistryRead 逐一对上（票 admin-skeleton-closure-batch/02）。
// 方法无租户参不是漏写：治理登记册以对象范围四维为身份，无租户维（ADR-0083）。
type unwiredGovernanceRegisters struct{}

func (unwiredGovernanceRegisters) ListAuthorityIntervals(
	context.Context,
	int,
) ([]govports.AuthorityIntervalRegistryRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredGovernanceRegisters) ListSuspensions(
	context.Context,
	int,
) ([]govports.SuspensionRegistryRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredGovernanceRegisters) ListResumptions(
	context.Context,
	int,
) ([]govports.ResumptionRegistryRow, error) {
	return nil, errOrchestrationNotWired
}

type unwiredComplianceRules struct{}

func (unwiredComplianceRules) ListCaseRequirementRules(
	context.Context,
	customsdomain.TenantID,
	int,
) ([]customsports.CaseRequirementRuleEntry, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredComplianceRules) ListInterpretationRules(
	context.Context,
	customsdomain.TenantID,
	int,
) ([]customsports.InterpretationRuleEntry, error) {
	return nil, errOrchestrationNotWired
}

// unwiredCaseRegisters 是案件配置三册列表读口的占位，方法表与
// customsports.CaseRegisterCatalogueRead 逐一对上（票 admin-web-page-wiring-frontier/05）。
type unwiredCaseRegisters struct{}

func (unwiredCaseRegisters) ListReadinessJudgments(
	context.Context,
	customsdomain.TenantID,
	int,
) ([]customsdomain.ReadinessJudgment, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCaseRegisters) ListSubmissionAuthorities(
	context.Context,
	customsdomain.TenantID,
	int,
) ([]customsdomain.SubmissionAuthorization, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCaseRegisters) ListClosureObligations(
	context.Context,
	customsdomain.TenantID,
	int,
) ([]customsports.ClosureObligationCatalogueEntry, error) {
	return nil, errOrchestrationNotWired
}

// unwiredGateConditions 是门禁条件目录列表读口的占位，方法表与
// customsports.GateConditionCatalogueRead 逐一对上（票 admin-web-page-wiring-frontier/06）。
type unwiredGateConditions struct{}

func (unwiredGateConditions) ListGateConditions(
	context.Context,
	customsdomain.TenantID,
	int,
) ([]customsports.GateConditionCatalogueEntry, error) {
	return nil, errOrchestrationNotWired
}

// unwiredPortsPaths 是口岸与申报路径两册列表读口的占位，方法表与
// customsports.PortsPathsCatalogueRead 逐一对上（票 admin-remainder-mechanism-batch/03）。
type unwiredPortsPaths struct{}

func (unwiredPortsPaths) ListCandidatePorts(
	context.Context,
	customsdomain.TenantID,
	int,
) ([]customsports.CandidatePortEntry, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredPortsPaths) ListDeclarationPaths(
	context.Context,
	customsdomain.TenantID,
	int,
) ([]customsports.DeclarationPathEntry, error) {
	return nil, errOrchestrationNotWired
}

type unwiredCommercialCatalogue struct{}

func (unwiredCommercialCatalogue) ListServiceProducts(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.ServiceProductCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListAcceptanceRulePackages(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.AcceptanceRulePackageRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListPreAcceptanceControls(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.PreAcceptanceControlRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListPricePolicies(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.PricePolicyRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListSettlementPolicies(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.SettlementPolicyRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListAsOfPolicyDeclarations(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.AsOfPolicyRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListAuthorizationRules(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.AuthorizationRuleRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListCreditPolicies(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.CreditPolicyRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListCustomerServiceRules(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.CustomerServiceRuleRow, error) {
	return nil, errOrchestrationNotWired
}

// 货主客户账户册（票 admin-write-faces/04）：方法表与 commercialhttp.CustomerAccountCatalogueReader
// 对上。它与身份另两册同挂在本占位上，因为生产装配交入的也是同一只商业目录适配器。
func (unwiredCommercialCatalogue) ListCustomerAccounts(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.CustomerAccountRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListCustomerContracts(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.CustomerContractCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListSupplierAgreements(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.SupplierAgreementCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListBusinessParties(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.BusinessPartyRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListGroupLegalEntities(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.GroupLegalEntityRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListPartyRelationships(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.PartyRelationshipRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListProductChannelMappings(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.ProductChannelMappingRow, error) {
	return nil, errOrchestrationNotWired
}

// unwiredVisibilityCatalogue 是 VE 六类目录列表读口的占位，方法表与
// visibilityports.CatalogueListRead 逐一对上（票 admin-web-page-wiring-frontier/02）。
type unwiredVisibilityCatalogue struct{}

func (unwiredVisibilityCatalogue) ListMilestoneMappings(
	context.Context,
	visibilitydomain.TenantID,
	int,
) ([]visibilityports.MilestoneMappingCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredVisibilityCatalogue) ListTriageRules(
	context.Context,
	visibilitydomain.TenantID,
	int,
) ([]visibilityports.TriageRuleCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredVisibilityCatalogue) ListNotificationPolicies(
	context.Context,
	visibilitydomain.TenantID,
	int,
) ([]visibilityports.NotificationPolicyCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredVisibilityCatalogue) ListClaimEligibilities(
	context.Context,
	visibilitydomain.TenantID,
	int,
) ([]visibilityports.ClaimEligibilityCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredVisibilityCatalogue) ListClaimAuthorizations(
	context.Context,
	visibilitydomain.TenantID,
	int,
) ([]visibilityports.ClaimAuthorizationCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredVisibilityCatalogue) ListDisclosurePolicies(
	context.Context,
	visibilitydomain.TenantID,
	int,
) ([]visibilityports.DisclosurePolicyCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

// unwiredCaseReview 是 VE 案件侧三读口（分诊两册、案件单册、理赔追偿三册）的占位，
// 方法表与 visibilityports 的 TriageReviewRead/CaseReviewRead/ClaimsRecoveryReviewRead
// 逐一对上（票 admin-skeleton-closure-batch/06）。一型三用不违「盖住接错适配器」：
// 生产装配点上三口本就共用一个 CaseReview 适配器，占位与生产同形。
type unwiredCaseReview struct{}

func (unwiredCaseReview) ListSignalEpisodes(
	context.Context,
	visibilitydomain.TenantID,
	int,
) ([]visibilityports.SignalEpisodeCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCaseReview) ListDispositionRequests(
	context.Context,
	visibilitydomain.TenantID,
	int,
) ([]visibilityports.DispositionRequestCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCaseReview) ListExceptionCases(
	context.Context,
	visibilitydomain.TenantID,
	int,
) ([]visibilityports.ExceptionCaseCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCaseReview) ListCustomerNotifications(
	context.Context,
	visibilitydomain.TenantID,
	int,
) ([]visibilityports.CustomerNotificationCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCaseReview) ListClaimItems(
	context.Context,
	visibilitydomain.TenantID,
	int,
) ([]visibilityports.ClaimItemCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCaseReview) ListRecoveryMatters(
	context.Context,
	visibilitydomain.TenantID,
	int,
) ([]visibilityports.RecoveryMatterCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

// unwiredCodSubledgers 是代收分户账册列表读口的占位，方法表与
// collectionports.CodSubledgerCatalogueRead 逐一对上（票 admin-remainder-mechanism-batch/04）。
type unwiredCodSubledgers struct{}

func (unwiredCodSubledgers) ListCodSubledgers(
	context.Context,
	collectiondomain.TenantID,
	int,
) ([]collectionports.CodSubledgerCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

// 结算与核算四页的读口占位（票 admin-skeleton-closure-batch/04）。四个类型而不是一个：
// 生产装配点上它们也是四个各自成形的读适配器，占位并成一个会让装配测试盖不住「某一本
// 册接错了适配器」这一格。

// unwiredSettlementCharges 的方法表与 settlementports.ChargeCatalogueRead 逐一对上。
type unwiredSettlementCharges struct{}

func (unwiredSettlementCharges) ListCustomerCharges(
	context.Context,
	settlementdomain.TenantID,
	int,
) ([]settlementports.CustomerChargeCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredSettlementCharges) ListSupplierExpectedCosts(
	context.Context,
	settlementdomain.TenantID,
	int,
) ([]settlementports.SupplierExpectedCostCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

// unwiredSettlementStatements 的方法表与 settlementports.StatementCatalogueRead 逐一对上。
type unwiredSettlementStatements struct{}

func (unwiredSettlementStatements) ListCustomerStatements(
	context.Context,
	settlementdomain.TenantID,
	int,
) ([]settlementports.CustomerStatementCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredSettlementStatements) ListSupplierBillReceptions(
	context.Context,
	settlementdomain.TenantID,
	int,
) ([]settlementports.SupplierBillReceptionCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

// unwiredSettlementFundsApplications 的方法表与 settlementports.FundsApplicationCatalogueRead
// 逐一对上。
type unwiredSettlementFundsApplications struct{}

func (unwiredSettlementFundsApplications) ListExternalFundsFacts(
	context.Context,
	settlementdomain.TenantID,
	int,
) ([]settlementports.ExternalFundsFactCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

// unwiredSettlementOperatingResults 的方法表与 settlementports.OperatingCatalogueRead
// 逐一对上。
type unwiredSettlementOperatingResults struct{}

func (unwiredSettlementOperatingResults) ListOperatingResults(
	context.Context,
	settlementdomain.TenantID,
	int,
) ([]settlementports.OperatingResultCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredSettlementOperatingResults) ListCostAllocations(
	context.Context,
	settlementdomain.TenantID,
	int,
) ([]settlementports.CostAllocationCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

type unwiredCancellation struct{}

func (unwiredCancellation) Handle(
	context.Context,
	shipmentapp.CancelParcelCommand,
) (shipmentapp.CancelParcelResult, error) {
	return shipmentapp.CancelParcelResult{}, errOrchestrationNotWired
}

type unwiredClaims struct{}

func (unwiredClaims) ReceiveClaim(
	context.Context,
	visibilityapp.ReceiveClaimCommand,
) (visibilityapp.HandleClaimResult, error) {
	return visibilityapp.HandleClaimResult{}, errOrchestrationNotWired
}

type unwiredResults struct{}

func (unwiredResults) Handle(
	context.Context,
	customsapp.ReceiveExternalResultCommand,
) (customsapp.ReceiveExternalResultResult, error) {
	return customsapp.ReceiveExternalResultResult{}, errOrchestrationNotWired
}

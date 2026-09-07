package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	crpostgres "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/postgres"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	govpg "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
)

const (
	defaultAddress  = ":8080"
	shutdownTimeout = 10 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("Parcel API stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	address := os.Getenv("IDP_PARCEL_HTTP_ADDR")
	if address == "" {
		address = defaultAddress
	}

	// 隔离读面准入（ADR-0078）在开池之前解析：门禁不合规要在启动最早处带原因退出。
	// 放行必须出声（Decision 三）——启用与所用合成租户写进启动日志，事后可查。
	isolatedRead, err := buildIsolatedReadIntakes(os.Getenv)
	if err != nil {
		return err
	}
	if isolatedRead != nil {
		logger.Info("Isolated read admission enabled (ADR-0078): operations query endpoints answer with injected synthetic scope",
			"tenant", os.Getenv(isolatedReadTenantEnv))
	}

	// 隔离写路径准入（ADR-0091）与读面同处最早：它也带一道启动即拒的前缀门禁，且
	// 多一条两开关一致性校验。放行同样必须出声——写路径会落行，事后要查得出这些行
	// 是在哪一次启动、以哪个合成租户写下的。
	isolatedWrite, err := buildIsolatedWriteAdmission(os.Getenv)
	if err != nil {
		return err
	}
	if isolatedWrite != nil {
		logger.Info("Isolated write admission enabled (ADR-0091): production ownership resolves against the governance register with injected synthetic coordinates",
			"tenant", os.Getenv(isolatedWriteTenantEnv),
			"selfAuthority", isolatedWrite.selfAuthority)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 开池在起服务之前：部署坏了（DSN 缺失、库不可达、缺框架 schema）要让进程带原因
	// 退出，而不是先挂上监听端口再让每个请求各报一次错。ctx 由停机信号驱动，启动途中
	// 收到 SIGTERM 就当场停下。
	db, closeDB, err := openDatabase(ctx, os.Getenv)
	if err != nil {
		return err
	}
	defer closeDB()

	submission, err := buildSubmissionOrchestration(db, isolatedWrite)
	if err != nil {
		return err
	}
	isolatedSubmissionIntake, err := buildIsolatedSubmissionIntake(db, isolatedWrite)
	if err != nil {
		return err
	}
	withdrawal, err := buildWithdrawalOrchestration(db)
	if err != nil {
		return err
	}
	requestViews, err := pspostgres.NewShipmentRequestViews(db)
	if err != nil {
		return err
	}
	manualReview, err := buildManualReviewOrchestration(db)
	if err != nil {
		return err
	}
	rejection, err := buildRejectionOrchestration(db)
	if err != nil {
		return err
	}
	// 受控补充编排（ADR-0106 Decision 四）：此前它只活在测试里，接进装配后「新提交版本已形成」
	// 信封才有生产发布方，停在`等待受控补充`并已入账的委托才有人续办。
	supplement, err := buildCustomerSupplementOrchestration(db)
	if err != nil {
		return err
	}
	// 资料修订编排（UC-PS-002；票 ps-port-remainder/04）：此前它只活在测试里，接进装配后修订请求
	// 才有生产入口——今天越过 Intake 后停在授权未决，那是提供方那两半（PC）未立的如实停点。
	amendment, err := buildCustomerAmendmentOrchestration(db)
	if err != nil {
		return err
	}
	// 复核队列读口就是委托查阅适配器（ports.AcceptanceReviewQueue 在其上补齐，同表同
	// 作用域纪律）；判断读口与形成决定读的是同一批判断行。
	reviewJudgments, err := pspostgres.NewAcceptanceJudgments(db)
	if err != nil {
		return err
	}
	// 面单交易查阅读的是自己那张表（票 admin-skeleton-closure-batch/08）：与委托查阅
	// 不同表、不同作用域维度（只有租户），因此另立适配器而不是往上面那只补方法。
	labelTransactions, err := pspostgres.NewLabelTransactionViews(db)
	if err != nil {
		return err
	}
	// 渠道择优决定查阅读的是决定登记册本尊（票 label-channel/23）：两个契约一只适配器，读面不是编排。
	channelSelectionDecisions, err := buildChannelSelectionDecisionRead(db)
	if err != nil {
		return err
	}
	cancellation, err := buildCancellationOrchestration(db)
	if err != nil {
		return err
	}
	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		return err
	}
	delivery, err := buildDeliveryOrchestration(db)
	if err != nil {
		return err
	}
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		return err
	}
	pickupCorrection, err := buildOffsitePickupCorrectionOrchestration(db)
	if err != nil {
		return err
	}
	movementFact, err := buildMovementFactOrchestration(db)
	if err != nil {
		return err
	}
	segmentOps, err := buildSegmentOperations(db)
	if err != nil {
		return err
	}
	credentialRegistration, err := buildExternalCarrierCredentialRegistration(db)
	if err != nil {
		return err
	}
	effectiveTimeRuleRegistration, err := buildEffectiveTimeRuleRegistration(db)
	if err != nil {
		return err
	}
	// 外部承运轨迹事实的判断面（票 label-channel/21）：读口取事实登记册本尊——它同时实现写侧的幂等存取与
	// ports.ExternalTrackingFactReviewRead，两个契约一只适配器（判据同交接范围汇总那格）；写编排另建，事务边界
	// 归装配点。
	externalTrackingFactReview, err := tfpostgres.NewExternalTrackingFacts(db)
	if err != nil {
		return err
	}
	effectiveTimeJudgment, err := buildEffectiveTimeJudgment(db)
	if err != nil {
		return err
	}
	trackingViews, err := vepostgres.NewCustomerViews(db)
	if err != nil {
		return err
	}
	// 运营追踪查阅读的就是投影库本身（ADR-0076）：同一适配器同时是派生编排的
	// ProjectionStore 与查阅端点的读面，不造第二份数据。
	projectionViews, err := vepostgres.NewProjections(db)
	if err != nil {
		return err
	}
	claims, err := buildClaimsOrchestration(db)
	if err != nil {
		return err
	}
	results, err := buildExternalResultsOrchestration(db)
	if err != nil {
		return err
	}

	// 主数据目录查阅直接接所属上下文的存储读面（ADR-0077），不绕进应用编排。
	// 同一上下文的多个端点共享同一只读适配器，读的仍是各登记写口背后的那份库。
	pricingCatalog, err := pppostgres.NewOperationsCatalogue(db)
	if err != nil {
		return err
	}
	// 覆盖地平线读口（票 pricing-reference-series-operations/05 第 1 项）：与目录读面
	// 同库，但它要连版本与复核两张表，且在用版本由领域按时刻挑，故自成一只适配器。
	referenceSeriesCoverage, err := pppostgres.NewReferenceSeriesCoverage(db)
	if err != nil {
		return err
	}
	// 被挂起评价联动读口（ADR-0105 Decision 五）：只读问题项子表 evaluation_issue，不扫评价快照。
	pendingSeriesEvaluations, err := pppostgres.NewPendingSeriesEvaluations(db)
	if err != nil {
		return err
	}
	// 评价册与路由判断两册是业务事实册的查阅面（票 admin-skeleton-closure-batch/03）：
	// 与目录读面同上下文同库，但行形状归各自用例，适配器各自成形。
	pricingEvaluations, err := pppostgres.NewEvaluationCatalogue(db)
	if err != nil {
		return err
	}
	// 价卡与序列登记写面编排（ADR-0085，票 admin-write-faces/01）：接真不等墙降——
	// 未配置 Intake 拒在编排之前，墙降那笔工作在装配点换的只是 Intake。
	priceCardRegistration, err := buildPriceCardRegistrationOrchestration(db)
	if err != nil {
		return err
	}
	referenceSeriesRegistration, err := buildReferenceSeriesRegistrationOrchestration(db)
	if err != nil {
		return err
	}
	referenceSeriesReview, err := buildReferenceSeriesReviewOrchestration(db)
	if err != nil {
		return err
	}
	referenceSeriesPreview, err := buildReferenceSeriesPreviewOrchestration(db)
	if err != nil {
		return err
	}
	referenceCatalogueRegistration, err := buildReferenceCatalogueRegistrationOrchestration(db)
	if err != nil {
		return err
	}
	networkCatalog, err := nrpostgres.NewNetworkCatalog(db)
	if err != nil {
		return err
	}
	routePlans, err := nrpostgres.NewRoutePlanCatalogue(db)
	if err != nil {
		return err
	}
	// 网络七族、关务四类与 VE 六类登记写面编排（票 admin-write-faces/02 切片
	// 02a/02b/02d）：判据同价卡首切片，接真不等墙降。
	networkCatalogRegistration, err := buildNetworkCatalogRegistrationOrchestration(db)
	if err != nil {
		return err
	}
	customsRegistration, err := buildCustomsRegistrationOrchestration(db)
	if err != nil {
		return err
	}
	veRegistration, err := buildVERegistrationOrchestration(db)
	if err != nil {
		return err
	}
	commercialRegistration, err := buildCommercialRegistrationOrchestration(db)
	if err != nil {
		return err
	}
	complianceRules, err := ccpostgres.NewRuleCatalogue(db)
	if err != nil {
		return err
	}
	caseRegisters, err := ccpostgres.NewCaseRegisterCatalogue(db)
	if err != nil {
		return err
	}
	gateConditions, err := ccpostgres.NewGateConditionCatalogue(db)
	if err != nil {
		return err
	}
	portsPaths, err := ccpostgres.NewPortsPathsCatalogue(db)
	if err != nil {
		return err
	}
	commercialCatalog, err := pcpostgres.NewOperationsCatalogue(db)
	if err != nil {
		return err
	}
	visibilityCatalogues, err := vepostgres.NewOperationsCatalogue(db)
	if err != nil {
		return err
	}
	codSubledgers, err := crpostgres.NewCodSubledgerCatalogue(db)
	if err != nil {
		return err
	}
	// 结算与核算四页各接自己的读适配器：四本目录读的是同一份库，但行形状与所属用例
	// 各不相同，合成一个适配器就得把四组读口挤进一个类型（判据同各读口的分册裁决）。
	settlementCharges, err := sapostgres.NewChargeCatalogue(db)
	if err != nil {
		return err
	}
	settlementStatements, err := sapostgres.NewStatementCatalogue(db)
	if err != nil {
		return err
	}
	settlementFundsApplications, err := sapostgres.NewFundsApplicationCatalogue(db)
	if err != nil {
		return err
	}
	settlementOperatingResults, err := sapostgres.NewOperatingCatalogue(db)
	if err != nil {
		return err
	}
	// 治理登记册读面（票 admin-skeleton-closure-batch/02）：读口无租户参是设计
	// （ADR-0083），适配器读的就是治理登记 CLI 写入的那三张表。
	governanceRegisters, err := govpg.NewGovernanceRegisters(db)
	if err != nil {
		return err
	}
	// VE 案件侧三页读面（票 admin-skeleton-closure-batch/06）：一个适配器实现三读口，
	// 分诊、案件与理赔追偿六本册子照行转写。
	caseReview, err := vepostgres.NewCaseReview(db)
	if err != nil {
		return err
	}
	// 节点作业与运输履约查阅页读面（票 admin-skeleton-closure-batch/05）：只是查阅
	// 读口，与上面 buildReceptionOrchestration/buildDeliveryOrchestration 构造的命令
	// 编排互不相知——读面不是编排，查阅不触发判断、派生或披露。
	nodeOperationsRecords, err := nopostgres.NewReviewCatalogue(db)
	if err != nil {
		return err
	}
	transportFulfillmentRecords, err := tfpostgres.NewReviewCatalogue(db)
	if err != nil {
		return err
	}
	// 交接范围汇总读用例（票 admin-web-audit-followups/06）。读口取交接登记册本尊：它同时
	// 实现写侧的幂等存取与 ports.HandoverScopeView，两个契约一只适配器（分口的理由在
	// ports.HandoverScopeView 注释）。这一格与上面几只不同——交入装配点的是应用用例不是
	// 读口，因为计数只能由领域派生；在这里预先数一遍就等于为同一形状立第二个口径。
	transportHandovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return err
	}
	handoverScopeSummary := tfapp.NewSummarizeHandoverScopeHandler(transportHandovers)

	server := &http.Server{
		Addr: address,
		Handler: httpapi.NewWithEndpoints(buildinfo.Current(), assembleBusinessEndpoints(
			submission,
			withdrawal,
			requestViews,
			manualReview,
			rejection,
			supplement,
			amendment,
			requestViews,
			reviewJudgments,
			labelTransactions,
			channelSelectionDecisions,
			cancellation,
			reception,
			nodeOperationsRecords,
			delivery,
			transportFulfillmentRecords,
			handoverScopeSummary,
			controlFacts.handover,
			controlFacts.pickupRegistration,
			pickupCorrection,
			controlFacts.pickupAttempt,
			movementFact,
			segmentOps.closer,
			segmentOps.opener,
			segmentOps.assign,
			segmentOps.enderOf,
			credentialRegistration,
			effectiveTimeRuleRegistration,
			externalTrackingFactReview,
			effectiveTimeJudgment,
			trackingViews,
			projectionViews,
			claims,
			results,
			pricingCatalog,
			pricingCatalog,
			referenceSeriesCoverage,
			pendingSeriesEvaluations,
			pricingEvaluations,
			priceCardRegistration,
			referenceSeriesRegistration,
			referenceSeriesReview,
			referenceSeriesPreview,
			referenceCatalogueRegistration,
			networkCatalog,
			routePlans,
			networkCatalogRegistration,
			complianceRules,
			caseRegisters,
			gateConditions,
			portsPaths,
			customsRegistration.interpretationRule,
			customsRegistration.gateCatalog,
			customsRegistration.candidatePort,
			customsRegistration.declarationPath,
			customsRegistration.caseRequirement,
			commercialCatalog,
			commercialCatalog,
			commercialCatalog,
			commercialCatalog,
			commercialCatalog,
			commercialCatalog,
			commercialRegistration.publication,
			commercialRegistration.partyIdentity,
			commercialRegistration.productChannel,
			commercialRegistration.channelAccountUse,
			visibilityCatalogues,
			veRegistration.milestoneMapping,
			veRegistration.triageRules,
			veRegistration.notificationPolicy,
			veRegistration.claimEligibility,
			veRegistration.claimAuthorization,
			veRegistration.disclosurePolicy,
			caseReview,
			caseReview,
			caseReview,
			codSubledgers,
			settlementCharges,
			settlementStatements,
			settlementFundsApplications,
			settlementOperatingResults,
			governanceRegisters,
			isolatedRead,
			isolatedSubmissionIntake,
		)),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveResult := make(chan error, 1)
	go func() {
		logger.Info("Parcel API listening", "address", address)
		serveResult <- server.ListenAndServe()
	}()

	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-serveResult; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

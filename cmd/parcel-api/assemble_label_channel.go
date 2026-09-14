package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	pspricing "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/parcelpricing"
	pscommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	shipmentdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	shipmentports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/outbound"
)

// 本文件是面单渠道写链的组合根（票 label-channel/28 裁乙）：择优 → 建立 → 提交渠道 →（07 出向）→ 记录结果 →
// 票 26 的定案触发，整条链在这一处接线。它照既有 build*Orchestration 家族只接线不加规则：候选收窄归 PC、评价归 PP、
// 并列归 PS 比较器、七类依据的翻译归 adapters/partycommercial（票 29）。
//
// `main` 的 `run` 在启动时装配它（票 label-channel/34），**只为 fail-fast**：组合根里任一读适配器或 outbox 构造失败，
// 进程带原因退出，产物构造即丢。**今天没有运营端点、也没有进程内触发面调它**（乙路无运营端点；谁在什么业务时点发起
// 一笔面单交易的建立是产品题，触发面另票）。装配函数存在的意义是让「已装配、未触发」有一处可核：每一段的真适配器、
// 留痕三件、事务壳与显式未配置的缝都在这里，接触发面的那张票只需要调 labelChannelOrchestration.Flow。
//
// 三个取数口（渠道约束、计价输入、BUY 价卡）与翻译器的三个源（账号使用授权选法、供应商协议选法、接受时解析回指）
// 都是实例半边（`PAR-INT-02` / `PAR-COM-10` / `PAR-SET-03`）：生产装配一律交显式未配置的实现——链在择优那一格如实停在
// 「未配置」，不建立交易、不写决定记录；照 UnconfiguredIntakeQualificationEvidence{} 那一路，不为变绿种任何映射、
// 价卡或约束。

// labelChannelSources 是组合根留给实例半边的六个缝。任一为 nil 即按显式未配置装配——这不是默认值，是「租户尚未
// 登记」这一事实在装配点上的写法；测试替身把它们配上（合成串）才能走到择优落定之后。
type labelChannelSources struct {
	Constraints  pscommercial.ChannelConstraintSource
	PricingInput pspricing.PricingInputSource
	BuyPlans     pspricing.ChannelBuyPlanSource
	Accounts     pscommercial.ChannelAccountUseSource
	Agreements   pscommercial.SupplierAgreementSource
	Resolutions  pscommercial.AcceptanceResolutionSource
}

// labelChannelSeams 是 buildLabelChannelOrchestrationWith 收的全部可替换处：六个实例半边缝，加三个**只给测试用**的
// 端口级替身位（装配 / 成本 / 翻译，nil → 真适配器）。后三个存在的理由：真装配与真翻译读的是 PC 的映射、发布册、
// 账号使用授权与供应商协议，那些行是实例半边、仓里没有种子，验收路径要走到择优落定之后只能在端口这一层换合成
// 替身；生产装配从不填它们（buildLabelChannelOrchestration 交零值）。
type labelChannelSeams struct {
	labelChannelSources
	Assembly   shipmentports.ChannelCandidateAssembly
	Costs      shipmentports.ChannelCandidateCostSource
	Translator shipmentports.ChannelSelectionBasisTranslator
}

// labelChannelOrchestration 是接线的产物：前置步编排（择优 → 翻译 → 建立）、事务壳里的 06 五步、07 出向端口。
//
// Flow 内部已经套了事务壳（择优一段、建立一段各一个），调用方直接调；Transactions 是同一只 06 编排的事务壳，
// 供 Flow 之后的四步（提交渠道 / 结果不确定 / 记录结果 / 后续动作）用——它们都要事务，且后两步与判断意图入队同笔。
type labelChannelOrchestration struct {
	Flow         *shipmentapp.EstablishSelectedLabelTransactionHandler
	Transactions transactionalLabelTransactions
	Gateway      shipmentports.LabelChannelGateway
}

// buildLabelChannelOrchestration 是生产装配：六个实例半边缝全部显式未配置，三个端口都是真适配器。
func buildLabelChannelOrchestration(db *bentopg.DB) (labelChannelOrchestration, error) {
	return buildLabelChannelOrchestrationWith(db, labelChannelSeams{})
}

// buildLabelChannelOrchestrationWith 让测试把缝配上合成串走通链的后半段；生产不调它。
func buildLabelChannelOrchestrationWith(db *bentopg.DB, seams labelChannelSeams) (labelChannelOrchestration, error) {
	none := labelChannelOrchestration{}
	if db == nil {
		return none, fmt.Errorf("parcel-api: label channel: db is nil")
	}
	clock := systemClock{}
	transactor := db.Transactor()
	sources := seams.labelChannelSources

	// 择优编排：装配适配器读 PC 的产品—渠道映射与发布册，约束口按缝；成本适配器的两个取数口按缝。
	mappings, err := pcpostgres.NewProductChannelMappings(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: product channel mappings: %w", err)
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: commercial publications: %w", err)
	}
	var assembly shipmentports.ChannelCandidateAssembly = pscommercial.NewChannelCandidateAssembler(pscommercial.ChannelCandidateAssemblerDeps{
		Mappings:    mappings,
		Publication: publications,
		Constraints: constraintsOrUnconfigured(sources.Constraints),
	})
	if seams.Assembly != nil {
		assembly = seams.Assembly
	}
	var costs shipmentports.ChannelCandidateCostSource = pspricing.NewChannelCandidateCostAdapter(pspricing.ChannelCandidateCostDeps{
		Input: pricingInputOrUnconfigured(sources.PricingInput),
		Plans: buyPlansOrUnconfigured(sources.BuyPlans),
	})
	if seams.Costs != nil {
		costs = seams.Costs
	}
	// 留痕三件全装（票面红线原句）：登记册是 /channel-selection-decisions 读面的唯一来源。
	decisions, err := pspostgres.NewChannelSelectionDecisions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: channel selection decisions: %w", err)
	}
	decisionIDs, err := psidentity.NewChannelSelectionDecisions()
	if err != nil {
		return none, fmt.Errorf("parcel-api: channel selection decision identities: %w", err)
	}
	selection := shipmentapp.NewSelectChannelCandidateHandler(shipmentapp.SelectChannelCandidateDeps{
		Assembly:    assembly,
		Costs:       costs,
		Decisions:   decisions,
		DecisionIDs: decisionIDs,
		Clock:       clock,
	})

	// 票 29 的翻译器：两个 PC 读口必装（构造期拒 nil，票 lc/35 收 lc/29 评审那条），三个源按缝。
	authorizations, err := pcpostgres.NewChannelAccountUseAuthorizations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: channel account use authorizations: %w", err)
	}
	agreements, err := pcpostgres.NewSupplierAgreementContents(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: supplier agreement contents: %w", err)
	}
	basisTranslator, err := pscommercial.NewChannelSelectionBasisTranslator(pscommercial.ChannelSelectionBasisTranslatorDeps{
		Accounts:       sources.Accounts,
		Agreements:     sources.Agreements,
		Resolutions:    sources.Resolutions,
		Authorizations: authorizations,
		Contents:       agreements,
	})
	if err != nil {
		return none, fmt.Errorf("parcel-api: channel selection basis translator: %w", err)
	}
	var translator shipmentports.ChannelSelectionBasisTranslator = basisTranslator
	if seams.Translator != nil {
		translator = seams.Translator
	}

	// 06 编排：仓储、判断意图口（lc/26；构造器不校验它，装配点断言非 nil）、继续尝试登记册与当前有效终局的只读半边
	// （lc/32；构造器构造期拒 nil）、时钟。两个只读口接的是与 `/continued-attempt-closures` 写面和终局采用路径同款
	// 适配器的读半边（同一张表）——`Establish` 前核的就是那两处写下的关闭与终局，另接一处就看不见它们。
	transactions, err := pspostgres.NewLabelTransactions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: label transactions: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	judgments, err := pspostgres.NewOutboxLabelTransactionJudgmentHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: label transaction judgment handoff: %w", err)
	}
	if transactions == nil || judgments == nil {
		return none, fmt.Errorf("parcel-api: label transaction handler dependencies are nil")
	}
	registers, err := pspostgres.NewContinuedAttemptRegisters(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: continued attempt registers: %w", err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: final outcomes: %w", err)
	}
	handler, err := shipmentapp.NewLabelTransactionHandler(shipmentapp.LabelTransactionDeps{
		Transactions: transactions,
		Judgments:    judgments,
		Registers:    registers,
		Finals:       finals,
		Clock:        clock,
	})
	if err != nil {
		return none, fmt.Errorf("parcel-api: label transaction handler: %w", err)
	}
	steps := transactionalLabelTransactions{transactor: transactor, inner: handler}

	flow := shipmentapp.NewEstablishSelectedLabelTransactionHandler(shipmentapp.EstablishSelectedLabelTransactionDeps{
		// 建立前那一问接交易仓储的读半边（票 35 裁决 1 取甲）：与建立步写的是同一张表，另接一处就看不见刚建的那笔。
		Lookup:       transactions,
		Selector:     transactionalChannelSelection{transactor: transactor, inner: selection},
		Translator:   translator,
		Transactions: steps,
	})
	return labelChannelOrchestration{
		Flow:         flow,
		Transactions: steps,
		Gateway:      unconfiguredLabelChannelGateway{},
	}, nil
}

// transactionalChannelSelection 把择优那一段包进一笔事务：决定记录经登记册写口落库，按框架合同无事务即拒，
// 事务边界归装配点（ADR-0134 决定三的同一条纪律）。它与建立那一段分开成两笔：翻译停下时决定记录仍在——
// 择优留痕在择优那一步已写完（翻译适配器头注原句），不随翻译或建立的失败回滚。壳只管事务：Select 不返 error
// 就提交、返 error 就回滚——并列与无人参选是编排的结果格（票 35 做法二），随事务一起提交，壳不认任何领域哨兵。
type transactionalChannelSelection struct {
	transactor bentoapp.Transactor
	inner      *shipmentapp.SelectChannelCandidateHandler
}

var _ shipmentapp.ChannelSelector = transactionalChannelSelection{}

func (selection transactionalChannelSelection) Select(
	ctx context.Context,
	query shipmentports.ChannelSelectionQuery,
) (shipmentapp.ChannelSelectionResult, error) {
	var result shipmentapp.ChannelSelectionResult
	err := selection.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		selected, selectErr := selection.inner.Select(txCtx, query)
		if selectErr != nil {
			return selectErr
		}
		result = selected
		return nil
	})
	if err != nil {
		return shipmentapp.ChannelSelectionResult{}, err
	}
	return result, nil
}

// transactionalLabelTransactions 是 06 五步的事务壳：每步各一笔。后两步的判断意图入队与 Save 同笔（ADR-0134
// 决定三），编排返回 error 时整笔回滚，调用方重放这一步。
type transactionalLabelTransactions struct {
	transactor bentoapp.Transactor
	inner      *shipmentapp.LabelTransactionHandler
}

var _ shipmentapp.LabelTransactionEstablisher = transactionalLabelTransactions{}

func (steps transactionalLabelTransactions) Establish(
	ctx context.Context,
	command shipmentapp.EstablishLabelTransactionCommand,
) (shipmentapp.LabelTransactionResult, error) {
	return steps.within(ctx, func(txCtx context.Context) (shipmentapp.LabelTransactionResult, error) {
		return steps.inner.Establish(txCtx, command)
	})
}

func (steps transactionalLabelTransactions) SubmitToChannel(
	ctx context.Context,
	command shipmentapp.SubmitLabelTransactionCommand,
) (shipmentapp.LabelTransactionResult, error) {
	return steps.within(ctx, func(txCtx context.Context) (shipmentapp.LabelTransactionResult, error) {
		return steps.inner.SubmitToChannel(txCtx, command)
	})
}

func (steps transactionalLabelTransactions) MarkResultUncertain(
	ctx context.Context,
	command shipmentapp.MarkLabelResultUncertainCommand,
) (shipmentapp.LabelTransactionResult, error) {
	return steps.within(ctx, func(txCtx context.Context) (shipmentapp.LabelTransactionResult, error) {
		return steps.inner.MarkResultUncertain(txCtx, command)
	})
}

func (steps transactionalLabelTransactions) RecordChannelResult(
	ctx context.Context,
	command shipmentapp.RecordLabelChannelResultCommand,
) (shipmentapp.LabelTransactionResult, error) {
	return steps.within(ctx, func(txCtx context.Context) (shipmentapp.LabelTransactionResult, error) {
		return steps.inner.RecordChannelResult(txCtx, command)
	})
}

func (steps transactionalLabelTransactions) AppendFollowUpAction(
	ctx context.Context,
	command shipmentapp.AppendLabelFollowUpActionCommand,
) (shipmentapp.LabelTransactionResult, error) {
	return steps.within(ctx, func(txCtx context.Context) (shipmentapp.LabelTransactionResult, error) {
		return steps.inner.AppendFollowUpAction(txCtx, command)
	})
}

func (steps transactionalLabelTransactions) within(
	ctx context.Context,
	step func(context.Context) (shipmentapp.LabelTransactionResult, error),
) (shipmentapp.LabelTransactionResult, error) {
	var result shipmentapp.LabelTransactionResult
	err := steps.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, stepErr := step(txCtx)
		if stepErr != nil {
			return stepErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return shipmentapp.LabelTransactionResult{}, err
	}
	return result, nil
}

// unconfiguredLabelChannelGateway 是 07 出向端口的未配置壳：真渠道适配器一家都没有（ports.LabelChannelGateway
// 头注「本口今天没有生产实现，这是设计而不是欠账」），生产装配交入它——三个操作一律答 outbound.Unconfigured()，
// **不发起任何调用**（ADR-0090 决定五：未配置即不得发起）。它不读请求里的 Configuration：真适配器才按
// outbound.AdmitCall 判，这里连地址都没有。合成替身归票 08、只在测试包，红线不许进生产装配；本壳与替身
// 不同——它不模拟任何答复，只说「没配」。
type unconfiguredLabelChannelGateway struct{}

var _ shipmentports.LabelChannelGateway = unconfiguredLabelChannelGateway{}

func (unconfiguredLabelChannelGateway) SubmitLabelRequest(
	context.Context, shipmentports.LabelChannelRequest,
) (shipmentports.LabelSubmissionOutcome, error) {
	return shipmentports.LabelSubmissionOutcome{Outcome: outbound.Unconfigured()}, nil
}

func (unconfiguredLabelChannelGateway) FetchLabelDocuments(
	context.Context, shipmentports.LabelChannelRequest,
) (shipmentports.LabelDocumentFetchOutcome, error) {
	return shipmentports.LabelDocumentFetchOutcome{Outcome: outbound.Unconfigured()}, nil
}

func (unconfiguredLabelChannelGateway) QuerySubmission(
	context.Context, shipmentports.LabelChannelRequest,
) (shipmentports.LabelSubmissionQueryOutcome, error) {
	// 「这家渠道提不提供查询口」是渠道的事实；渠道都没配，这一问没有对象。Support 留零值不作任何主张——答「提供」
	// 是编一个事实，答「该源无查询口」会把这一笔交人对账；读的人先看 Outcome 的未配置格：调用没有发生过。
	return shipmentports.LabelSubmissionQueryOutcome{Outcome: outbound.Unconfigured()}, nil
}

// 三个取数口与三个源的显式未配置实现。它们不读请求、不构造任何东西、不作任何业务判断，只把「未配置」按各自
// 端口的形状交出来：约束口交零值（ChannelConstraint 零值即未配置，装配适配器据此答 ErrChannelConstraintNotConfigured）；
// 其余五口第二个返回值为 false。

type unconfiguredChannelConstraints struct{}

func (unconfiguredChannelConstraints) ChannelConstraintFor(
	context.Context, shipmentports.ChannelSelectionQuery,
) (pscommercial.ChannelConstraint, error) {
	return pscommercial.ChannelConstraint{}, nil
}

type unconfiguredPricingInput struct{}

func (unconfiguredPricingInput) PricingInputFor(
	context.Context, shipmentports.ChannelSelectionQuery,
) (pricingdomain.PricingInputSnapshot, bool, error) {
	return pricingdomain.PricingInputSnapshot{}, false, nil
}

type unconfiguredBuyPlans struct{}

func (unconfiguredBuyPlans) BuyPlanFor(
	context.Context, shipmentports.ChannelSelectionQuery, shipmentdomain.ChannelCandidateID,
) (pricingdomain.PlanEvaluationTarget, bool, error) {
	return pricingdomain.PlanEvaluationTarget{}, false, nil
}

func constraintsOrUnconfigured(source pscommercial.ChannelConstraintSource) pscommercial.ChannelConstraintSource {
	if source == nil {
		return unconfiguredChannelConstraints{}
	}
	return source
}

func pricingInputOrUnconfigured(source pspricing.PricingInputSource) pspricing.PricingInputSource {
	if source == nil {
		return unconfiguredPricingInput{}
	}
	return source
}

func buyPlansOrUnconfigured(source pspricing.ChannelBuyPlanSource) pspricing.ChannelBuyPlanSource {
	if source == nil {
		return unconfiguredBuyPlans{}
	}
	return source
}

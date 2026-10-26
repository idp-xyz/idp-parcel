// Package settlementaccounting 是 parcel-shipment 对 settlement-accounting 的消费侧适配器
// ——第二个跨上下文适配器（ADR-0025：只有本包可以同时导入两个上下文；适配器只翻译不判断，
// 翻译必须是全函数）。
package settlementaccounting

import (
	"context"
	"errors"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// ErrUntranslatableAnswer 表示某一侧交出了本适配器词汇表之外的内容——编程错误而不是业务
// 答案，理由同 partycommercial 适配器的同名哨兵。它包着端口的 ErrUntranslatableProviderAnswer：编排不导入本包，
// 凭端口那一个哨兵认出这一格，不把它折成依赖不可用（ADR-0081 决定三）。
var ErrUntranslatableAnswer = fmt.Errorf("parcel shipment settlementaccounting adapter: untranslatable answer: %w",
	psports.ErrUntranslatableProviderAnswer)

// 本适配器自有的停摆原因引用。都是消费方这一侧没形成的东西，不冠 SA- 前缀；提供方状态
// 译过来的引用冠 SA-（见 assessmentFor）。
//
// 作用域那几条按恢复动作分开：CONTROL_SCOPE_NOT_CONFIGURED 只说账户映射没登（ADR-0158 决定三钉的
// 就是这个码），登一行结算账户就推得动，而那次登记会发信封来重驱；解析没采用结算政策要改的是解析
// 键，没有登记动作推得动它，另报一个码、留在`未形成`；作用域源没接、或它的解析口与账户目录任一没接是
// 装配缺口，要改的是装配，同样留在`未形成`，也另报一个码——借账户那个码会让同一个码落进两格。
const (
	reasonScopeNotConfigured              = "CONTROL_SCOPE_NOT_CONFIGURED"
	reasonScopeSettlementPolicyNotAdopted = "CONTROL_SCOPE_SETTLEMENT_POLICY_NOT_ADOPTED"
	reasonScopeNotAssembled               = "CONTROL_SCOPE_NOT_ASSEMBLED"
)

// ControlScope 是一次控制请求的资金坐标连同它的商业来路。两者装在一个值里而不是分两个
// 方法取，是因为资金作用域本就是从那次商业解析的结算政策回显派生的：同出一次解析，就该
// 同进同出（ADR-0079 决定五）。拆成两口会给出两次解析的机会：施加时问控制策略用的回指与
// 冻结落在哪个作用域，就可能出自两次解析。释放不取它，按施加时随结果存下的那一份作用域认领
// （ADR-0282 决定四）。
type ControlScope struct {
	Settlement sadomain.SettlementScope
	Resolution sadomain.CommercialResolutionReference
}

// ControlScopeSource 为一份委托解析资金作用域（责任法人/结算账户/币种）与商业解析回指。
//
// 作用域该从已解析的结算政策来（ADR-0044 已让方式与六维范围可观察），账户映射是实例半边：没有
// 租户就没有映射。没形成时第二个返回值说缺的是哪一样，本适配器据以停下——不代拟一个账户去冻
// 别人的钱。
//
// 回指是 SA 控制策略视图向商业侧提问的键（sa-preacceptance-policy-view/01 裁决）：SA 不得
// 自行从资金作用域反查合同，那会成为账户映射的第二处定义。
type ControlScopeSource interface {
	FormControlScope(
		ctx context.Context,
		identity psdomain.SourceIdentity,
		shipmentRequestID psdomain.ShipmentRequestID,
		submissionVersion psdomain.SubmissionVersionID,
	) (ControlScope, ControlScopeGap, error)
}

// ControlScopeGap 是作用域形成的落点。没形成时按恢复动作分格（ADR-0029），不并成一个「没有」：
// 并了，一次登记推得动的缺口与一次登记推不动的缺口就只能落进同一个端口格。
type ControlScopeGap uint8

const (
	ControlScopeGapInvalid ControlScopeGap = iota
	ControlScopeFormed
	// ControlScopeAccountNotRegistered：结算账户册上没有与这次解析回显相符的应收行。
	ControlScopeAccountNotRegistered
	// ControlScopeSettlementPolicyNotAdopted：解析没带回已采用的结算政策，推不出法人与币种。
	ControlScopeSettlementPolicyNotAdopted
	// ControlScopeNotAssembled：作用域源的解析口或账户目录没接上。恢复动作是补装配，登记推不动它。
	ControlScopeNotAssembled
)

// PreAcceptanceControlAdapter 把 parcel-shipment 的接受前财务控制两个端口接到
// settlement-accounting 的应用编排上（施加与释放各一个，端口分开的理由在 PS ports 里）。
type PreAcceptanceControlAdapter struct {
	apply   *saapplication.ApplyPreAcceptanceControlHandler
	release *saapplication.ReleasePreAcceptanceControlHandler
	scopes  ControlScopeSource
}

type PreAcceptanceControlAdapterDeps struct {
	Apply   *saapplication.ApplyPreAcceptanceControlHandler
	Release *saapplication.ReleasePreAcceptanceControlHandler
	Scopes  ControlScopeSource
}

// NewPreAcceptanceControlAdapter 装配两侧。Scopes 允许为 nil：那是装配缺口，施加停在`未形成`、报
// CONTROL_SCOPE_NOT_ASSEMBLED。释放用不到它，只接释放半边的装配可以不接。
func NewPreAcceptanceControlAdapter(deps PreAcceptanceControlAdapterDeps) *PreAcceptanceControlAdapter {
	return &PreAcceptanceControlAdapter{
		apply:   deps.Apply,
		release: deps.Release,
		scopes:  deps.Scopes,
	}
}

var (
	_ psports.PreAcceptanceFinancialController = (*PreAcceptanceControlAdapter)(nil)
	_ psports.PreAcceptanceControlRelease      = (*PreAcceptanceControlAdapter)(nil)
)

// controlRequestIdentity 是交给提供方的控制请求身份，同时充当译回本上下文的结果标识。
//
// 冻结标识是提供方账本的内部编号，不随控制结果离开它的上下文；而`业务限制`根本没有冻结
// 标识。用请求身份作结果标识，三种已形成结果就有了同一来源的标识，释放时译回去即可按原
// 关联认领——不需要任何一侧发明第二套编号。
func controlRequestIdentity(
	shipmentRequestID psdomain.ShipmentRequestID,
	submissionVersion psdomain.SubmissionVersionID,
) string {
	return shipmentRequestID.String() + "/" + submissionVersion.String()
}

// ApplyPreAcceptanceFinancialControl 执行施加半边：作用域由消费方形成，交提供方执行控制，再把封闭
// 答复译回本上下文的两层代数。金额不在这一侧：提供方按它的采用登记形成估价（ADR-0179 决定一），本侧
// 只交委托与提交版本。
func (adapter *PreAcceptanceControlAdapter) ApplyPreAcceptanceFinancialControl(
	ctx context.Context,
	request psports.FinancialControlRequest,
) (psports.PreAcceptanceControlAssessment, error) {
	if adapter.scopes == nil {
		return notFormed(reasonScopeNotAssembled)
	}
	controlScope, gap, err := adapter.scopes.FormControlScope(
		ctx, request.Identity, request.ShipmentRequestID, request.SubmissionVersion)
	if err != nil {
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("form control scope: %w", err)
	}
	switch gap {
	case ControlScopeFormed:
	case ControlScopeAccountNotRegistered:
		// 结算账户登记与登记行同一事务交「参数已登记」类信封（settlement-accounting 的
		// RegisterSettlementAccountHandler），这一格因此有人来推。
		return refusedAssessment(psports.PreAcceptanceControlNotConfigured, reasonScopeNotConfigured)
	case ControlScopeSettlementPolicyNotAdopted:
		return notFormed(reasonScopeSettlementPolicyNotAdopted)
	case ControlScopeNotAssembled:
		return notFormed(reasonScopeNotAssembled)
	default:
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("%w: control scope gap %d",
			ErrUntranslatableAnswer, uint8(gap))
	}

	command, err := applyCommandFor(request, controlScope)
	if err != nil {
		return psports.PreAcceptanceControlAssessment{}, err
	}
	answer, err := adapter.apply.Handle(ctx, command)
	if err != nil {
		// 提供方的施加编排只为编程错误交 error（依赖答不出它自己折成`未形成`），重投得到同一个错误：与释放那一侧同落译不动
		// 那一格，编排上抛、不折成`财务控制不可用`（ADR-0282 决定一）。
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("%w: apply pre-acceptance control: %w",
			ErrUntranslatableAnswer, err)
	}
	return assessmentFor(request, controlScope.Settlement, answer)
}

// ReleasePreAcceptanceControl 执行释放半边：按施加时随结果存下的那份作用域认领，把提供方的封闭答复逐格译成端口的
// 三格（ADR-0282 决定一、四）。
//
// 作用域不在这里重新推导。释放那一刻再从商业解析与账户登记推一次，政策回显或账户登记在施加之后变了，推出来的就是
// 另一份作用域：提供方在那里找不到这一笔、答`无可释放`，本上下文当成已收口，原占用静默成孤儿。存量结果没存作用域
// 时照样交零值给提供方，由它短路答`未受理`——不在这里替它先答，与 applyCommandFor 同一条纪律。
func (adapter *PreAcceptanceControlAdapter) ReleasePreAcceptanceControl(
	ctx context.Context,
	request psports.ControlReleaseRequest,
) (psports.ControlReleaseAnswer, error) {
	if adapter.release == nil {
		return psports.ControlReleaseAnswer{}, errors.New(
			"parcel shipment settlementaccounting adapter: release handler is not assembled")
	}
	answer, err := adapter.release.Handle(ctx, saapplication.ReleasePreAcceptanceControlCommand{
		TenantID:  tenantFor(request.Identity),
		RequestID: releaseRequestIdentity(request.ControlResultID),
		Scope:     settlementScopeFor(request.Scope),
	})
	if err != nil {
		// 提供方没在自己的封闭集合里作答（刚找到的冻结释放不了：编程错误或时钟异常），它自己也写明不是调用方能据以
		// 行动的业务答案。
		return psports.ControlReleaseAnswer{}, fmt.Errorf("%w: release pre-acceptance control: %w",
			ErrUntranslatableAnswer, err)
	}
	return releaseAnswerFor(answer)
}

// releaseAnswerFor 是提供方释放答复到端口三格的全函数，逐取值落点，default 报错不吸收（ADR-0025）。
func releaseAnswerFor(answer saapplication.ReleasePreAcceptanceControlResult) (psports.ControlReleaseAnswer, error) {
	switch answer.Outcome() {
	case saapplication.ControlReleased, saapplication.NothingToRelease:
		// `无可释放`不是失败：这个关联下没有资金停留在占用状态（控制从未形成，或只形成过
		// 业务限制），补偿据此收口，而不是对着一笔不存在的占用无休止重试。
		return psports.ControlReleaseAnswer{Outcome: psports.ControlReleaseSettled}, nil
	case saapplication.ReleaseNotFormed:
		reason := answer.NotFormedReason().String()
		if reason == "" {
			return psports.ControlReleaseAnswer{}, fmt.Errorf("%w: release not-formed reason %d",
				ErrUntranslatableAnswer, uint8(answer.NotFormedReason()))
		}
		return releaseAnswer(psports.ControlReleaseNotFormed, "SA-"+reason)
	case saapplication.ReleaseRequestNotAccepted:
		// 提供方在读任何账本之前就拒收了：身份或作用域立不起来。身份是本适配器译的、作用域是施加时存下的，被拒收说明
		// 那一份引用坏了或根本没存下——重放同一请求得到同一个答复，不是一种可重试的未决。
		return releaseAnswer(psports.ControlReleaseNotAccepted, "SA-"+answer.Outcome().String())
	default:
		return psports.ControlReleaseAnswer{}, fmt.Errorf("%w: release outcome %d", ErrUntranslatableAnswer, answer.Outcome())
	}
}

func releaseAnswer(outcome psports.ControlReleaseOutcome, label string) (psports.ControlReleaseAnswer, error) {
	reason, err := psdomain.NewCheckReason(label)
	if err != nil {
		return psports.ControlReleaseAnswer{}, fmt.Errorf("%w: check reason: %v", ErrUntranslatableAnswer, err)
	}
	return psports.ControlReleaseAnswer{Outcome: outcome, Reason: reason}, nil
}

// settlementScopeFor 把存下的作用域译回提供方的值。没存下或译不动都交零值，由提供方短路答`未受理`。
func settlementScopeFor(scope psdomain.OccupationScope) sadomain.SettlementScope {
	if !scope.Recorded() {
		return sadomain.SettlementScope{}
	}
	legalEntity, err := sadomain.NewLegalEntityReference(scope.LegalEntity())
	if err != nil {
		return sadomain.SettlementScope{}
	}
	account, err := sadomain.NewSettlementAccountID(scope.Account())
	if err != nil {
		return sadomain.SettlementScope{}
	}
	currency, err := sadomain.NewCurrencyCode(scope.Currency())
	if err != nil {
		return sadomain.SettlementScope{}
	}
	settlement, err := sadomain.NewSettlementScope(legalEntity, account, currency)
	if err != nil {
		return sadomain.SettlementScope{}
	}
	return settlement
}

// applyCommandFor 把消费方请求译成提供方命令。身份立不起来的部分译成零值交提供方短路作答
// （`未受理`），不在这里替它先答——与 partycommercial 适配器同一条纪律。
func applyCommandFor(
	request psports.FinancialControlRequest,
	scope ControlScope,
) (saapplication.ApplyPreAcceptanceControlCommand, error) {
	identity := controlRequestIdentity(request.ShipmentRequestID, request.SubmissionVersion)
	command := saapplication.ApplyPreAcceptanceControlCommand{
		TenantID:          tenantFor(request.Identity),
		Scope:             scope.Settlement,
		ShipmentRequest:   request.ShipmentRequestID.String(),
		SubmissionVersion: request.SubmissionVersion.String(),
		Resolution:        scope.Resolution,
	}
	if requestID, err := sadomain.NewControlRequestID(identity); err == nil {
		command.RequestID = requestID
	}
	if association, err := sadomain.NewBusinessAssociationReference(identity); err == nil {
		command.Association = association
	}
	asOf, err := controlAsOfFor(request.AsOf)
	if err != nil {
		return saapplication.ApplyPreAcceptanceControlCommand{}, err
	}
	command.AsOf = asOf
	return command, nil
}

func tenantFor(identity psdomain.SourceIdentity) sadomain.TenantID {
	tenant, err := sadomain.NewTenantID(identity.TenantID().String())
	if err != nil {
		return sadomain.TenantID{}
	}
	return tenant
}

func releaseRequestIdentity(resultID psdomain.FinancialControlResultID) sadomain.ControlRequestID {
	rebuilt, err := sadomain.NewControlRequestID(resultID.String())
	if err != nil {
		return sadomain.ControlRequestID{}
	}
	return rebuilt
}

// controlAsOfFor 把已回显的判断时点译成提供方的控制时点。三件套一一对应；请求里的时点由
// 编排从权威回显得来，译不动只剩编程错误。
func controlAsOfFor(asOf psdomain.JudgmentAsOf) (sadomain.ControlAsOf, error) {
	semantic, err := sadomain.NewAsOfSemantic(asOf.Semantics().String())
	if err != nil {
		return sadomain.ControlAsOf{}, fmt.Errorf("%w: as-of semantic: %v", ErrUntranslatableAnswer, err)
	}
	strategy, err := sadomain.NewAsOfStrategyVersion(asOf.PolicyVersion().String())
	if err != nil {
		return sadomain.ControlAsOf{}, fmt.Errorf("%w: as-of strategy version: %v", ErrUntranslatableAnswer, err)
	}
	translated, err := sadomain.NewControlAsOf(semantic, asOf.At(), strategy)
	if err != nil {
		return sadomain.ControlAsOf{}, fmt.Errorf("%w: control as-of: %v", ErrUntranslatableAnswer, err)
	}
	return translated, nil
}

// assessmentFor 是提供方封闭答复到本上下文两层代数的全函数。每格显式落点，default 报错
// 不吸收（ADR-0025）。
func assessmentFor(
	request psports.FinancialControlRequest,
	scope sadomain.SettlementScope,
	answer saapplication.ApplyPreAcceptanceControlResult,
) (psports.PreAcceptanceControlAssessment, error) {
	switch answer.Outcome() {
	case saapplication.ControlApplied:
		return appliedControlAssessment(request, scope, answer)
	case saapplication.ControlNotApplicable:
		asOf, err := judgmentAsOfFor(answer.AsOf())
		if err != nil {
			return psports.PreAcceptanceControlAssessment{}, err
		}
		basis, err := psdomain.NewControlBasisReference(answer.ControlBasis().String())
		if err != nil {
			return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("%w: control basis: %v", ErrUntranslatableAnswer, err)
		}
		result, err := psdomain.NewInapplicableFinancialControlResult(basis, asOf)
		if err != nil {
			return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("%w: financial control result: %v", ErrUntranslatableAnswer, err)
		}
		return psports.PreAcceptanceControlAssessment{
			Outcome: psports.PreAcceptanceControlFormed,
			Result:  result,
		}, nil
	case saapplication.ControlRequestConflict:
		return refusedAssessment(psports.PreAcceptanceControlRequestConflict, "SA-REQUEST_CONFLICT")
	case saapplication.ControlRequestNotAccepted:
		return refusedAssessment(psports.PreAcceptanceControlRequestNotAccepted, "SA-REQUEST_NOT_ACCEPTED")
	case saapplication.ControlNotFormed:
		outcome, err := notFormedOutcomeFor(answer.NotFormedReason())
		if err != nil {
			return psports.PreAcceptanceControlAssessment{}, err
		}
		return refusedAssessment(outcome, "SA-"+answer.NotFormedReason().String())
	default:
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("%w: control outcome %d",
			ErrUntranslatableAnswer, answer.Outcome())
	}
}

// notFormedOutcomeFor 按恢复动作把提供方的未形成原因分到端口的两格（ADR-0094 决定一、二）：只有运营登记
// 推得动、且那次登记会发「参数已登记」信封来重驱的落`未配置`，其余落`未形成`，消费门照旧回滚重投。
// 逐取值列出、default 报错（ADR-0025）：提供方多一个原因时这里先炸，而不是让它静默继承某一格——那一格
// 决定的是烧不烧失败预算。
func notFormedOutcomeFor(reason saapplication.NotFormedReason) (psports.PreAcceptanceControlOutcome, error) {
	switch reason {
	case saapplication.ControlPolicyNotConfigured:
		// 合同声明或策略正文没登：发布其中任一样时 party-commercial 都交信封
		// （announceOperatorRegistrations）。解析键没要求控制策略那一种来由也报这个码，而键按（租户+客户）
		// 只有一行、改登即内容冲突，没有登记动作推得动它——在这一格里它是一条查得到的等待，等不来重驱。
		return psports.PreAcceptanceControlNotConfigured, nil
	case saapplication.CreditRatioBaseUndecided:
		// 存量正文没声明比例的基数，要用得上只能发新版信用政策正文，那条通道发布时同样交信封。
		return psports.PreAcceptanceControlNotConfigured, nil
	case saapplication.ControlAmountNotConfigured:
		// 采用册上没有这一户：登记采用行时 settlement-accounting 同一事务交「受理前估价采用已登记」，那一份登记续办门按租户
		// 重驱（ADR-0179 决定五、ADR-0094 决定四）。
		return psports.PreAcceptanceControlNotConfigured, nil
	case saapplication.ControlPolicyUnavailable,
		saapplication.BalanceUnavailable,
		saapplication.FreezeLedgerUnavailable,
		saapplication.CreditStandingUnavailable,
		saapplication.ExposureLedgerUnavailable,
		saapplication.CreditBasisUnavailable,
		saapplication.CreditRatioBaseUnavailable:
		return psports.PreAcceptanceControlNotFormed, nil
	case saapplication.CreditBasisNotConfigured:
		// 走到控制这一步时闭包已唯一解析，没采用信用政策只能是解析键没要求它——理由同上，改登即冲突，
		// 挪到`未配置`只换来一次没人来推的等待。
		return psports.PreAcceptanceControlNotFormed, nil
	case saapplication.CreditRatioBaseNotEstablished:
		// 等的是事实出现（运营余额、有效对账单），不是一次登记。
		return psports.PreAcceptanceControlNotFormed, nil
	case saapplication.EstimateAdoptionUnavailable,
		saapplication.EstimateTrialsUnavailable,
		saapplication.EstimateTraceUnavailable:
		return psports.PreAcceptanceControlNotFormed, nil
	case saapplication.EstimatePriceCardNotConfigured,
		saapplication.EstimatePriceCardConflict,
		saapplication.EstimateSubmissionNotVisible,
		saapplication.EstimateDeclarationIncomplete,
		saapplication.EstimateHasNoTrial,
		saapplication.EstimateTrialWithoutTotal,
		saapplication.EstimateCurrencyMismatch,
		saapplication.EstimateNotPositive,
		saapplication.EstimateTrialNegative,
		saapplication.EstimateSumOverflow:
		// 补价卡、补件或改卡都推得动它们，但没有一样会发信封来重驱这份委托：挪到`未配置`只换来一次没人来推的等待。
		return psports.PreAcceptanceControlNotFormed, nil
	default:
		return psports.PreAcceptanceControlOutcomeInvalid, fmt.Errorf("%w: not-formed reason %d",
			ErrUntranslatableAnswer, uint8(reason))
	}
}

// appliedControlAssessment 把`已执行`译回本上下文的采用结果。提供方自 ADR-0122 起按策略正文逐项
// 执行，冻结与暴露**可以同时在场**；本函数只翻译——每项已执行的控制译成一项 ControlItemResult、
// 共同通过条件译成本上下文的封闭集——多项怎么合起来看由领域构造期按条件推出（ADR-0125 决定三：
// CONTEXT 把这一步判给 parcel-shipment，判的是接受语言，所以住在领域层而不在适配器里）。
//
// 一项都没执行的`已执行`是阶段契约被打破；条件集外由领域拒绝，这里不先替它折。
//
// 施加用的那一份作用域随结果交回，由编排随结果存下：释放按它认领，不在释放那一刻重新推导（ADR-0282 决定四）。
func appliedControlAssessment(
	request psports.FinancialControlRequest,
	scope sadomain.SettlementScope,
	answer saapplication.ApplyPreAcceptanceControlResult,
) (psports.PreAcceptanceControlAssessment, error) {
	executed := answer.ExecutedControls()
	if len(executed) == 0 {
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf(
			"%w: applied control executed no control item", ErrUntranslatableAnswer)
	}
	items := make([]psdomain.ControlItemResult, 0, len(executed))
	for _, control := range executed {
		item, err := controlItemFor(control, answer)
		if err != nil {
			return psports.PreAcceptanceControlAssessment{}, err
		}
		items = append(items, item)
	}
	jointPass, err := jointPassConditionFor(answer.JointPassCondition())
	if err != nil {
		return psports.PreAcceptanceControlAssessment{}, err
	}
	asOf, err := judgmentAsOfFor(answer.AsOf())
	if err != nil {
		return psports.PreAcceptanceControlAssessment{}, err
	}
	resultID, err := psdomain.NewFinancialControlResultID(
		controlRequestIdentity(request.ShipmentRequestID, request.SubmissionVersion))
	if err != nil {
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("%w: control result ID: %v",
			ErrUntranslatableAnswer, err)
	}
	occupationScope, err := psdomain.NewOccupationScope(
		scope.LegalEntity().String(), scope.Account().String(), scope.Currency().String())
	if err != nil {
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("%w: occupation scope: %v",
			ErrUntranslatableAnswer, err)
	}
	result, err := psdomain.NewExecutedFinancialControlResult(psdomain.ExecutedFinancialControlSpec{
		ResultID:  resultID,
		Items:     items,
		JointPass: jointPass,
		AsOf:      asOf,
		Scope:     occupationScope,
	})
	if err != nil {
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("%w: financial control result: %v",
			ErrUntranslatableAnswer, err)
	}
	return psports.PreAcceptanceControlAssessment{
		Outcome: psports.PreAcceptanceControlFormed,
		Result:  result,
	}, nil
}

// controlItemFor 把一项已执行的控制译成本上下文的控制项结果。受限原因不在 ExecutedControl 上，
// 要按种类回同一答复的 Freeze / Exposure 取；那一格的状态与 ExecutedControl.Restricted 必须互相
// 印证——种类对应的占用不在场、状态与受限标记矛盾、或刚执行的控制已释放，都是阶段契约被打破。
func controlItemFor(
	control saapplication.ExecutedControl,
	answer saapplication.ApplyPreAcceptanceControlResult,
) (psdomain.ControlItemResult, error) {
	var (
		kind       psdomain.ControlItemKind
		restricted bool
		reason     string
	)
	switch control.Kind() {
	case sadomain.PrepaidFreezeControl:
		freeze, present := answer.Freeze()
		if !present {
			return psdomain.ControlItemResult{}, fmt.Errorf(
				"%w: an executed prepaid freeze carries no freeze", ErrUntranslatableAnswer)
		}
		kind = psdomain.PrepaidFreezeControlItem
		switch freeze.Status() {
		case sadomain.FreezeHeld:
		case sadomain.FreezeRestricted:
			restricted, reason = true, freeze.Reason().String()
		case sadomain.FreezeReleased:
			return psdomain.ControlItemResult{}, fmt.Errorf(
				"%w: an applied control came back released", ErrUntranslatableAnswer)
		default:
			return psdomain.ControlItemResult{}, fmt.Errorf(
				"%w: freeze status %d", ErrUntranslatableAnswer, freeze.Status())
		}
	case sadomain.CreditCheckControl:
		exposure, present := answer.Exposure()
		if !present {
			return psdomain.ControlItemResult{}, fmt.Errorf(
				"%w: an executed credit check carries no exposure", ErrUntranslatableAnswer)
		}
		kind = psdomain.CreditCheckControlItem
		switch exposure.Status() {
		case sadomain.ExposureRecorded:
		case sadomain.ExposureRestricted:
			restricted, reason = true, exposure.Reason().String()
		case sadomain.ExposureReleased:
			return psdomain.ControlItemResult{}, fmt.Errorf(
				"%w: an applied control came back released", ErrUntranslatableAnswer)
		default:
			return psdomain.ControlItemResult{}, fmt.Errorf(
				"%w: exposure status %d", ErrUntranslatableAnswer, exposure.Status())
		}
	default:
		return psdomain.ControlItemResult{}, fmt.Errorf(
			"%w: control kind %d", ErrUntranslatableAnswer, uint8(control.Kind()))
	}
	if restricted != control.Restricted() {
		return psdomain.ControlItemResult{}, fmt.Errorf(
			"%w: executed control %s reports restricted=%t while its ledger record says otherwise",
			ErrUntranslatableAnswer, control.Kind(), control.Restricted())
	}

	conclusion, basis := psdomain.ControlItemSatisfied, psdomain.ControlBasisReference{}
	if restricted {
		formed, err := psdomain.NewControlBasisReference(reason)
		if err != nil {
			return psdomain.ControlItemResult{}, fmt.Errorf("%w: restriction reason: %v", ErrUntranslatableAnswer, err)
		}
		conclusion, basis = psdomain.ControlItemRestricted, formed
	}
	item, err := psdomain.NewControlItemResult(kind, control.Order(), conclusion, basis)
	if err != nil {
		return psdomain.ControlItemResult{}, fmt.Errorf("%w: control item result: %v", ErrUntranslatableAnswer, err)
	}
	return item, nil
}

// jointPassConditionFor 是两侧封闭集之间的全函数。首发两侧都只有「全部通过」一值；提供方放宽出第二种
// 组合子时这里先炸，而不是让一个本上下文还不会算的组合子静默落成全部通过（ADR-0025）。
func jointPassConditionFor(condition sadomain.JointPassCondition) (psdomain.JointPassCondition, error) {
	switch condition {
	case sadomain.AllControlsPass:
		return psdomain.AllControlsPass, nil
	default:
		return psdomain.JointPassConditionInvalid, fmt.Errorf(
			"%w: joint pass condition %d", ErrUntranslatableAnswer, uint8(condition))
	}
}

// judgmentAsOfFor 从提供方回显的时点重建本上下文的判断时点。回显取自答复而不是转手请求——与
// partycommercial 适配器的回显纪律同一条。
func judgmentAsOfFor(echoed sadomain.ControlAsOf) (psdomain.JudgmentAsOf, error) {
	semantics, err := psdomain.NewAsOfSemanticsReference(echoed.Semantic().String())
	if err != nil {
		return psdomain.JudgmentAsOf{}, fmt.Errorf("%w: echoed semantics: %v", ErrUntranslatableAnswer, err)
	}
	policyVersion, err := psdomain.NewAsOfPolicyVersion(echoed.StrategyVersion().String())
	if err != nil {
		return psdomain.JudgmentAsOf{}, fmt.Errorf("%w: echoed strategy version: %v", ErrUntranslatableAnswer, err)
	}
	policy, err := psdomain.NewEchoedAsOfPolicy(psdomain.FinancialControlJudgmentKind, semantics, policyVersion)
	if err != nil {
		return psdomain.JudgmentAsOf{}, fmt.Errorf("%w: echoed policy: %v", ErrUntranslatableAnswer, err)
	}
	asOf, err := psdomain.NewJudgmentAsOf(echoed.At(), policy)
	if err != nil {
		return psdomain.JudgmentAsOf{}, fmt.Errorf("%w: judgment as-of: %v", ErrUntranslatableAnswer, err)
	}
	return asOf, nil
}

func notFormed(label string) (psports.PreAcceptanceControlAssessment, error) {
	return refusedAssessment(psports.PreAcceptanceControlNotFormed, label)
}

func refusedAssessment(
	outcome psports.PreAcceptanceControlOutcome,
	label string,
) (psports.PreAcceptanceControlAssessment, error) {
	reason, err := psdomain.NewCheckReason(label)
	if err != nil {
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("%w: check reason: %v", ErrUntranslatableAnswer, err)
	}
	return psports.PreAcceptanceControlAssessment{Outcome: outcome, Reason: reason}, nil
}

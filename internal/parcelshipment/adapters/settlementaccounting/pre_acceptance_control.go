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
// 答案，理由同 partycommercial 适配器的同名哨兵。
var ErrUntranslatableAnswer = errors.New("parcel shipment settlementaccounting adapter: untranslatable answer")

// 本适配器自有的停摆原因引用。两条都是消费方实例半边未配置，不冠 SA- 前缀；提供方状态
// 译过来的引用冠 SA-（见 reasonFor）。
const (
	reasonScopeNotConfigured  = "CONTROL_SCOPE_NOT_CONFIGURED"
	reasonAmountNotConfigured = "CONTROL_AMOUNT_NOT_CONFIGURED"
)

// ControlScopeSource 为一份委托解析资金作用域（责任法人/结算账户/币种）。
//
// 作用域该从已解析的结算政策来（ADR-0044 已让方式与六维范围可观察），接通那条缝之前它是
// 实例半边：没有租户就没有账户映射。nil 或第二个返回值为 false 都是「显式未配置」，本适配
// 器据以停在`未形成`——停下，不代拟一个账户去冻别人的钱。
type ControlScopeSource interface {
	FormControlScope(
		ctx context.Context,
		identity psdomain.SourceIdentity,
		shipmentRequestID psdomain.ShipmentRequestID,
		submissionVersion psdomain.SubmissionVersionID,
	) (sadomain.SettlementScope, bool, error)
}

// ControlAmountSource 形成本次控制要占用的金额（最小币单位）。金额该由估价形成（计价缝），
// 接通前属实例半边；未配置停在`未形成`，绝不拿零或一个拍脑袋的数去占客户资金。
type ControlAmountSource interface {
	FormControlAmount(ctx context.Context, request psports.FinancialControlRequest) (int64, bool, error)
}

// PreAcceptanceControlAdapter 把 parcel-shipment 的接受前财务控制两个端口接到
// settlement-accounting 的应用编排上（施加与释放各一个，端口分开的理由在 PS ports 里）。
type PreAcceptanceControlAdapter struct {
	apply   *saapplication.ApplyPreAcceptanceControlHandler
	release *saapplication.ReleasePreAcceptanceControlHandler
	scopes  ControlScopeSource
	amounts ControlAmountSource
}

type PreAcceptanceControlAdapterDeps struct {
	Apply   *saapplication.ApplyPreAcceptanceControlHandler
	Release *saapplication.ReleasePreAcceptanceControlHandler
	Scopes  ControlScopeSource
	Amounts ControlAmountSource
}

// NewPreAcceptanceControlAdapter 装配两侧。Scopes 与 Amounts 允许为 nil：今天既没有账户
// 映射也没有估价，nil 是「显式未配置」的诚实表达，届时施加停在`未形成`、释放报可重试错误
// ——那正是首发要停下的地方。
func NewPreAcceptanceControlAdapter(deps PreAcceptanceControlAdapterDeps) *PreAcceptanceControlAdapter {
	return &PreAcceptanceControlAdapter{
		apply:   deps.Apply,
		release: deps.Release,
		scopes:  deps.Scopes,
		amounts: deps.Amounts,
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

// ApplyPreAcceptanceFinancialControl 执行施加半边：作用域与金额由消费方形成（实例半边），
// 交提供方执行控制，再把封闭答复译回本上下文的两层代数。
func (adapter *PreAcceptanceControlAdapter) ApplyPreAcceptanceFinancialControl(
	ctx context.Context,
	request psports.FinancialControlRequest,
) (psports.PreAcceptanceControlAssessment, error) {
	if adapter.scopes == nil {
		return notFormed(reasonScopeNotConfigured)
	}
	scope, formedScope, err := adapter.scopes.FormControlScope(
		ctx, request.Identity, request.ShipmentRequestID, request.SubmissionVersion)
	if err != nil {
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("form control scope: %w", err)
	}
	if !formedScope {
		return notFormed(reasonScopeNotConfigured)
	}

	if adapter.amounts == nil {
		return notFormed(reasonAmountNotConfigured)
	}
	amount, formedAmount, err := adapter.amounts.FormControlAmount(ctx, request)
	if err != nil {
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("form control amount: %w", err)
	}
	if !formedAmount {
		return notFormed(reasonAmountNotConfigured)
	}

	command, err := applyCommandFor(request, scope, amount)
	if err != nil {
		return psports.PreAcceptanceControlAssessment{}, err
	}
	answer, err := adapter.apply.Handle(ctx, command)
	if err != nil {
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("apply pre-acceptance control: %w", err)
	}
	return assessmentFor(request, answer)
}

// ReleasePreAcceptanceControl 执行释放半边。`无可释放`与`已释放`都交回 nil：两者之下都没有
// 资金停留在占用状态，补偿要收口的正是这一点。读不回/存不回与作用域未配置都交回错误——
// 决定那一侧会留下补偿续办引用，配置齐或依赖恢复后重放同一请求即可。
func (adapter *PreAcceptanceControlAdapter) ReleasePreAcceptanceControl(
	ctx context.Context,
	request psports.ControlReleaseRequest,
) error {
	if adapter.scopes == nil {
		return fmt.Errorf("release pre-acceptance control: %s", reasonScopeNotConfigured)
	}
	scope, formed, err := adapter.scopes.FormControlScope(
		ctx, request.Identity, request.ShipmentRequestID, request.SubmissionVersion)
	if err != nil {
		return fmt.Errorf("form control scope: %w", err)
	}
	if !formed {
		return fmt.Errorf("release pre-acceptance control: %s", reasonScopeNotConfigured)
	}

	answer, err := adapter.release.Handle(ctx, saapplication.ReleasePreAcceptanceControlCommand{
		TenantID:  tenantFor(request.Identity),
		RequestID: releaseRequestIdentity(request.ControlResultID),
		Scope:     scope,
	})
	if err != nil {
		return fmt.Errorf("release pre-acceptance control: %w", err)
	}
	switch answer.Outcome() {
	case saapplication.ControlReleased, saapplication.NothingToRelease:
		// `无可释放`不是失败：这个关联下没有资金停留在占用状态（控制从未形成，或只形成过
		// 业务限制），补偿据此收口，而不是对着一笔不存在的占用无休止重试。
		return nil
	case saapplication.ReleaseNotFormed:
		return fmt.Errorf("release pre-acceptance control: %s", answer.NotFormedReason())
	case saapplication.ReleaseRequestNotAccepted:
		// 身份是本适配器译的，被提供方拒收说明翻译或调用方的引用坏了——不是一种可重试的
		// 未决。这条判断建立在「控制请求标识只由本适配器铸造」上（controlRequestIdentity）；
		// 若日后出现第二个铸造点，这格要重审。
		return fmt.Errorf("%w: release request not accepted", ErrUntranslatableAnswer)
	default:
		return fmt.Errorf("%w: release outcome %d", ErrUntranslatableAnswer, answer.Outcome())
	}
}

// applyCommandFor 把消费方请求译成提供方命令。身份立不起来的部分译成零值交提供方短路作答
// （`未受理`），不在这里替它先答——与 partycommercial 适配器同一条纪律。
func applyCommandFor(
	request psports.FinancialControlRequest,
	scope sadomain.SettlementScope,
	amount int64,
) (saapplication.ApplyPreAcceptanceControlCommand, error) {
	identity := controlRequestIdentity(request.ShipmentRequestID, request.SubmissionVersion)
	command := saapplication.ApplyPreAcceptanceControlCommand{
		TenantID:    tenantFor(request.Identity),
		Scope:       scope,
		AmountMinor: amount,
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
	answer saapplication.ApplyPreAcceptanceControlResult,
) (psports.PreAcceptanceControlAssessment, error) {
	switch answer.Outcome() {
	case saapplication.ControlApplied:
		freeze, present := answer.Freeze()
		if !present {
			return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("%w: applied control carries no freeze",
				ErrUntranslatableAnswer)
		}
		return appliedAssessment(request, answer, freeze)
	case saapplication.ControlNotApplicable:
		result, err := controlResultFor(
			psdomain.FinancialControlResultID{},
			psdomain.FinancialControlNotApplicable,
			answer.ControlBasis().String(),
			answer.AsOf(),
		)
		if err != nil {
			return psports.PreAcceptanceControlAssessment{}, err
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
		return refusedAssessment(psports.PreAcceptanceControlNotFormed, "SA-"+answer.NotFormedReason().String())
	default:
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("%w: control outcome %d",
			ErrUntranslatableAnswer, answer.Outcome())
	}
}

// appliedAssessment 译`已执行`的两种冻结状态。`已释放`不在施加答复的封闭集合里——刚执行
// 的控制不可能已经释放，走到那格是阶段契约被打破。
func appliedAssessment(
	request psports.FinancialControlRequest,
	answer saapplication.ApplyPreAcceptanceControlResult,
	freeze sadomain.FundsFreeze,
) (psports.PreAcceptanceControlAssessment, error) {
	identity := controlRequestIdentity(request.ShipmentRequestID, request.SubmissionVersion)
	resultID, err := psdomain.NewFinancialControlResultID(identity)
	if err != nil {
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("%w: control result ID: %v",
			ErrUntranslatableAnswer, err)
	}

	var result psdomain.FinancialControlResult
	switch freeze.Status() {
	case sadomain.FreezeHeld:
		result, err = controlResultFor(resultID, psdomain.FinancialControlHeld, "", answer.AsOf())
	case sadomain.FreezeRestricted:
		result, err = controlResultFor(resultID, psdomain.FinancialControlRestricted, freeze.Reason().String(), answer.AsOf())
	case sadomain.FreezeReleased:
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("%w: an applied control came back released",
			ErrUntranslatableAnswer)
	default:
		return psports.PreAcceptanceControlAssessment{}, fmt.Errorf("%w: freeze status %d",
			ErrUntranslatableAnswer, freeze.Status())
	}
	if err != nil {
		return psports.PreAcceptanceControlAssessment{}, err
	}
	return psports.PreAcceptanceControlAssessment{
		Outcome: psports.PreAcceptanceControlFormed,
		Result:  result,
	}, nil
}

// controlResultFor 从提供方回显的时点构造本上下文的控制结果。回显取自答复而不是转手请求
// ——与 partycommercial 适配器的回显纪律同一条。
func controlResultFor(
	resultID psdomain.FinancialControlResultID,
	outcome psdomain.FinancialControlOutcome,
	basisReference string,
	echoed sadomain.ControlAsOf,
) (psdomain.FinancialControlResult, error) {
	semantics, err := psdomain.NewAsOfSemanticsReference(echoed.Semantic().String())
	if err != nil {
		return psdomain.FinancialControlResult{}, fmt.Errorf("%w: echoed semantics: %v", ErrUntranslatableAnswer, err)
	}
	policyVersion, err := psdomain.NewAsOfPolicyVersion(echoed.StrategyVersion().String())
	if err != nil {
		return psdomain.FinancialControlResult{}, fmt.Errorf("%w: echoed strategy version: %v", ErrUntranslatableAnswer, err)
	}
	policy, err := psdomain.NewEchoedAsOfPolicy(psdomain.FinancialControlJudgmentKind, semantics, policyVersion)
	if err != nil {
		return psdomain.FinancialControlResult{}, fmt.Errorf("%w: echoed policy: %v", ErrUntranslatableAnswer, err)
	}
	asOf, err := psdomain.NewJudgmentAsOf(echoed.At(), policy)
	if err != nil {
		return psdomain.FinancialControlResult{}, fmt.Errorf("%w: judgment as-of: %v", ErrUntranslatableAnswer, err)
	}

	basis := psdomain.ControlBasisReference{}
	if basisReference != "" {
		basis, err = psdomain.NewControlBasisReference(basisReference)
		if err != nil {
			return psdomain.FinancialControlResult{}, fmt.Errorf("%w: control basis: %v", ErrUntranslatableAnswer, err)
		}
	}
	result, err := psdomain.NewFinancialControlResult(resultID, outcome, basis, asOf)
	if err != nil {
		return psdomain.FinancialControlResult{}, fmt.Errorf("%w: financial control result: %v", ErrUntranslatableAnswer, err)
	}
	return result, nil
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

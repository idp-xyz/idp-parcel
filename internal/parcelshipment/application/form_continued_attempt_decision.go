package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件是`面单继续尝试决定`写面的命令编排（票 label-channel/30）：受控关闭与重开两条命令一个 handler，
// 理由同 06 五步一个 handler——两条命令追加进的是同一条版本链，恢复动作也同形（授权怎么答、册读不回怎么答、
// 版本冲突怎么答），拆成两个 handler 就有两份会各自漂移的口径。
//
// 它只做四件：问授权、读事实（当前有效终局、既有册）、交聚合追加、落册后交出判断意图。一条业务规则都不加：
// 关闭 / 重开各自必备什么、重开指不到生效关闭怎么办、终局在场能不能重开，全在 `ContinuedAttemptRegister.Append`；
// 授权够不够格在 party-commercial（经 ContinuedAttemptDecisionAuthorizer）。PS 不自判决定方、不比等级
// （pc-gaps/13 裁决 ①）：四件（决定方 / 授权角色 / 授权依据快照 / 请求方）里前三件原样取自授权答复，第四件是
// 命令自带的事实，登录操作人不进任何一格（CONTEXT「登录操作人可以作为操作证据，但不能替代实际决定方和授权角色」）。
//
// **它不持 Transactor**（ADR-0134 决定三的同一条纪律）：`Insert` / `Save` 与判断意图入队都从 ctx 取事务执行器，
// 事务由组合根的事务壳开；写侧任一失败原样上抛，整步随事务回滚——不留「决定已落、判断意图丢了」的中间态。
//
// 系统不自动形成任何决定（CONTEXT 硬句）：这里没有任何不带命令的入口，也没有节拍。

// ContinuedAttemptDecisionOutcome 是两条命令共用的结果代数，按调用方的恢复动作分格（ADR-0029）。
type ContinuedAttemptDecisionOutcome uint8

const (
	ContinuedAttemptDecisionOutcomeInvalid ContinuedAttemptDecisionOutcome = iota
	// ContinuedAttemptDecisionFormed：决定已追加、册已落、判断意图已入队。
	ContinuedAttemptDecisionFormed
	// ContinuedAttemptDecisionNotAuthorized：party-commercial 明确不允许。它是确定的业务答案，续办补不出授权来。
	ContinuedAttemptDecisionNotAuthorized
	// ContinuedAttemptDecisionNotAdmitted：输入没问题，是这一册此刻不允许这一步（重开指不到生效关闭、当前有效终局
	// 在场）。恢复动作是去看这个包裹停在哪一格，不是改参数重试——与领域 ErrContinuedAttemptDecisionNotAdmitted 同一分界。
	ContinuedAttemptDecisionNotAdmitted
	// ContinuedAttemptDecisionNotAccepted：输入本身立不起来（缺项、责任来源种类认不出、货主重授权证据缺席或账户对不上、
	// 重开生效时间不晚于关闭）。恢复动作是改输入重来；具体哪一格见 Refusal。
	ContinuedAttemptDecisionNotAccepted
	// ContinuedAttemptDecisionUndecided：等一个依赖（授权规则未配置、授权口 / 终局口 / 登记册读不回、标识签不出）。
	// 未写任何东西；哪一个依赖见 PendingReason。
	ContinuedAttemptDecisionUndecided
	// ContinuedAttemptDecisionWriteConflict：开册撞上别人刚开的、或追加时预期版本对不上。版本冲突是业务答案（ADR-0031），
	// 调用方重读再重放；本层不代猜库里此刻是什么。
	ContinuedAttemptDecisionWriteConflict
)

func (outcome ContinuedAttemptDecisionOutcome) String() string {
	switch outcome {
	case ContinuedAttemptDecisionFormed:
		return "FORMED"
	case ContinuedAttemptDecisionNotAuthorized:
		return "NOT_AUTHORIZED"
	case ContinuedAttemptDecisionNotAdmitted:
		return "NOT_ADMITTED"
	case ContinuedAttemptDecisionNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case ContinuedAttemptDecisionUndecided:
		return "UNDECIDED"
	case ContinuedAttemptDecisionWriteConflict:
		return "REVISION_CONFLICT"
	default:
		return ""
	}
}

// ContinuedAttemptPendingReason 指名`未决`停在哪个依赖上。
type ContinuedAttemptPendingReason uint8

const (
	ContinuedAttemptPendingReasonNone ContinuedAttemptPendingReason = iota
	// ContinuedAttemptAuthorityRulesNotConfigured：这个范围此刻一条现行授权规则都没有。它不是拒绝——把「没有规则」答成
	// `不允许`同样是一次默认，只是方向朝紧（`PAR-COM-13` / `PAR-COM-14` 待提供）。
	ContinuedAttemptAuthorityRulesNotConfigured
	ContinuedAttemptAuthorityUnavailable
	ContinuedAttemptFinalOutcomeUnavailable
	ContinuedAttemptDecisionRegisterUnavailable
	ContinuedAttemptDecisionIdentityUnavailable
)

func (reason ContinuedAttemptPendingReason) String() string {
	switch reason {
	case ContinuedAttemptAuthorityRulesNotConfigured:
		return "AUTHORITY_RULES_NOT_CONFIGURED"
	case ContinuedAttemptAuthorityUnavailable:
		return "AUTHORITY_UNAVAILABLE"
	case ContinuedAttemptFinalOutcomeUnavailable:
		return "FINAL_OUTCOME_UNAVAILABLE"
	case ContinuedAttemptDecisionRegisterUnavailable:
		return "REGISTER_UNAVAILABLE"
	case ContinuedAttemptDecisionIdentityUnavailable:
		return "DECISION_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

// ContinuedAttemptRefusal 指名`输入未受理`是哪一格立不起来。只列本层自己核出的那几格加领域构造拒这一格；
// 领域拒的成因不再细分——它已经在领域错误上分过，本层不重新命名。
type ContinuedAttemptRefusal uint8

const (
	ContinuedAttemptRefusalNone ContinuedAttemptRefusal = iota
	// ContinuedAttemptDecisionInvalid：领域构造门拒了（缺必备项、重开生效时间不晚于所解关闭……）。
	ContinuedAttemptDecisionInvalid
	// ContinuedAttemptResponsibilitySourceUnknown：关闭责任来源的种类不在封闭集内，或主体为空。
	ContinuedAttemptResponsibilitySourceUnknown
	// ContinuedAttemptShipperReauthorizationMissing：原关闭因货主指令形成，重开命令没带该货主账户新的有效授权证据
	// （CONTEXT「因货主指令形成的关闭，重开还必须有同一货主账户新的有效授权」）。
	ContinuedAttemptShipperReauthorizationMissing
	// ContinuedAttemptShipperAccountMismatch：重开命令里的货主账户不是原关闭的请求方，或原关闭没记请求方——
	// 「同一货主账户」核不出来就不成立。
	ContinuedAttemptShipperAccountMismatch
)

func (refusal ContinuedAttemptRefusal) String() string {
	switch refusal {
	case ContinuedAttemptDecisionInvalid:
		return "DECISION_INVALID"
	case ContinuedAttemptResponsibilitySourceUnknown:
		return "RESPONSIBILITY_SOURCE_UNKNOWN"
	case ContinuedAttemptShipperReauthorizationMissing:
		return "SHIPPER_REAUTHORIZATION_MISSING"
	case ContinuedAttemptShipperAccountMismatch:
		return "SHIPPER_ACCOUNT_MISMATCH"
	default:
		return ""
	}
}

// ClosureResponsibilitySourceKind 是`关闭责任来源`的种类段。CONTEXT 把重开怎么处理按来源分开说：因货主指令形成的
// 关闭要同一货主账户新的有效授权；因运营企业操作形成的按同级或更高重开权限规则（那是 PC 的事）；监管、禁限运、
// 安全暂停、渠道账号撤销或到期等硬限制要先由来源责任方形成有效解除。三格取自那几句，不另造。
//
// 领域的 ClosureResponsibilitySourceReference 是一个不透明必填串，本层把它写成 `<种类>/<主体>`：重开时要认得出原关闭
// 是不是货主指令形成的，认不出就核不了「同一货主账户」那一步。硬限制那一格的「解除」今天没有来源可读，本编排不为
// 它加门——加一道读不到来源的门等于把重开一律拒掉，那是替来源责任方作答。
type ClosureResponsibilitySourceKind uint8

const (
	ClosureResponsibilitySourceKindInvalid ClosureResponsibilitySourceKind = iota
	ShipperInstructionResponsibilitySource
	OperatorActionResponsibilitySource
	ExternalRestrictionResponsibilitySource
)

func (kind ClosureResponsibilitySourceKind) String() string {
	switch kind {
	case ShipperInstructionResponsibilitySource:
		return "SHIPPER-INSTRUCTION"
	case OperatorActionResponsibilitySource:
		return "OPERATOR-ACTION"
	case ExternalRestrictionResponsibilitySource:
		return "EXTERNAL-RESTRICTION"
	default:
		return ""
	}
}

const closureResponsibilitySourceSeparator = "/"

// closureResponsibilitySourceKindOf 从已落册的关闭上读回种类段。读不出（旧格式、被人手改）按「认不出」处理，
// 由调用处决定那意味着什么——这里不猜。
func closureResponsibilitySourceKindOf(source domain.ClosureResponsibilitySourceReference) ClosureResponsibilitySourceKind {
	segment, _, found := strings.Cut(source.String(), closureResponsibilitySourceSeparator)
	if !found {
		return ClosureResponsibilitySourceKindInvalid
	}
	for _, kind := range []ClosureResponsibilitySourceKind{
		ShipperInstructionResponsibilitySource,
		OperatorActionResponsibilitySource,
		ExternalRestrictionResponsibilitySource,
	} {
		if kind.String() == segment {
			return kind
		}
	}
	return ClosureResponsibilitySourceKindInvalid
}

// FormControlledClosureCommand 携带一次受控关闭。Requester 可缺席（CONTEXT「请求方（如有）」——运营企业自行发起的关闭
// 没有外部请求方）；决定方、授权角色、授权依据快照不在命令里，它们由授权答复给出。EffectiveAt 由命令给：生效时间是
// 决定的一部分，不是本上下文铸的动作时刻。
type FormControlledClosureCommand struct {
	Identity                    domain.SourceIdentity
	Parcel                      domain.DeclaredParcelID
	Requester                   domain.RequesterReference
	Reason                      domain.ContinuedAttemptReasonReference
	EffectiveAt                 time.Time
	ResponsibilitySourceKind    ClosureResponsibilitySourceKind
	ResponsibilitySourceSubject string
}

// FormReopeningCommand 携带一次重开。RelatedPriorClosure 必填（CONTEXT「关联此前关闭」）。ShipperAccount 与
// ShipperAuthorizationEvidence 只在原关闭因货主指令形成时被读：核「与原关闭请求方同一账户」与「证据非空」两件
// （票 label-channel/30 做法 6）；原关闭不是货主指令形成的，两格不读。证据引用不落册——领域决定上没有它的格，
// 它只在这一步作门。
type FormReopeningCommand struct {
	Identity                     domain.SourceIdentity
	Parcel                       domain.DeclaredParcelID
	Requester                    domain.RequesterReference
	Reason                       domain.ContinuedAttemptReasonReference
	EffectiveAt                  time.Time
	RelatedPriorClosure          domain.ContinuedAttemptDecisionID
	ShipperAccount               domain.RequesterReference
	ShipperAuthorizationEvidence string
}

// ContinuedAttemptDecisionResult 交回这一步停在哪里。Decision 只在`已形成`时携带：其余各格都没有新决定可交，
// 交回既有册上别的决定会被当成本次的结果。
type ContinuedAttemptDecisionResult struct {
	outcome     ContinuedAttemptDecisionOutcome
	decision    domain.ContinuedAttemptDecision
	hasDecision bool
	pending     ContinuedAttemptPendingReason
	refusal     ContinuedAttemptRefusal
}

func (result ContinuedAttemptDecisionResult) Outcome() ContinuedAttemptDecisionOutcome {
	return result.outcome
}

func (result ContinuedAttemptDecisionResult) Decision() (domain.ContinuedAttemptDecision, bool) {
	return result.decision, result.hasDecision
}

// PendingReason 只在`未决`时非零。
func (result ContinuedAttemptDecisionResult) PendingReason() ContinuedAttemptPendingReason {
	return result.pending
}

// Refusal 只在`输入未受理`时非零。
func (result ContinuedAttemptDecisionResult) Refusal() ContinuedAttemptRefusal {
	return result.refusal
}

func continuedAttemptUndecided(reason ContinuedAttemptPendingReason) ContinuedAttemptDecisionResult {
	return ContinuedAttemptDecisionResult{outcome: ContinuedAttemptDecisionUndecided, pending: reason}
}

func continuedAttemptNotAccepted(refusal ContinuedAttemptRefusal) ContinuedAttemptDecisionResult {
	return ContinuedAttemptDecisionResult{outcome: ContinuedAttemptDecisionNotAccepted, refusal: refusal}
}

// FormContinuedAttemptDecisionDeps 收拢六口，全部必填：缺一口不是「这一段不做」，是装配缺件。
type FormContinuedAttemptDecisionDeps struct {
	Authorizer ports.ContinuedAttemptDecisionAuthorizer
	// Finals 只用它的 FindCurrentFinal：`currentFinalPresent` 由调用方从包裹当前有效终局取得（`Append` 头注），终局属本
	// 上下文但不属本册。
	Finals     ports.FinalOutcomeStore
	Registers  ports.ContinuedAttemptRegisterRepository
	Identities ports.ContinuedAttemptDecisionIdentity
	Handoff    ports.ContinuedAttemptDecisionHandoff
	Clock      ports.Clock
}

type FormContinuedAttemptDecisionHandler struct {
	deps FormContinuedAttemptDecisionDeps
}

// NewFormContinuedAttemptDecisionHandler 构造期逐口拒 nil：漏装的那一口到第一次真调用才 panic，而写面在
// `Insert` / `Save` 之后还要入队，半路 panic 留下的是一笔没有判断意图的决定。
func NewFormContinuedAttemptDecisionHandler(deps FormContinuedAttemptDecisionDeps) (*FormContinuedAttemptDecisionHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"continued attempt decision authorizer", deps.Authorizer == nil},
		{"final outcome store", deps.Finals == nil},
		{"continued attempt register repository", deps.Registers == nil},
		{"continued attempt decision identity", deps.Identities == nil},
		{"continued attempt decision handoff", deps.Handoff == nil},
		{"clock", deps.Clock == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("form continued attempt decision: %s is nil", dependency.name)
		}
	}
	return &FormContinuedAttemptDecisionHandler{deps: deps}, nil
}

// FormControlledClosure 形成一条受控关闭决定。
//
// 授权先于一切写动作与标识签发：未获授权的尝试不该消耗一个决定标识——标识是本上下文签发的稀缺身份（撤回编排
// 同一条理由）。`AuthoritativeCutoffBoundary` 的值就是本次关闭决定的标识（票 label-channel/30 裁决）：与关闭路径
// 终局的来源版本 `CONTINUED-ATTEMPT-CLOSURE/<决定标识>` 同一个标识，一个事一个名。
func (handler *FormContinuedAttemptDecisionHandler) FormControlledClosure(
	ctx context.Context,
	command FormControlledClosureCommand,
) (ContinuedAttemptDecisionResult, error) {
	if command.ResponsibilitySourceKind.String() == "" || strings.TrimSpace(command.ResponsibilitySourceSubject) == "" {
		return continuedAttemptNotAccepted(ContinuedAttemptResponsibilitySourceUnknown), nil
	}
	source, err := domain.NewClosureResponsibilitySourceReference(
		command.ResponsibilitySourceKind.String() + closureResponsibilitySourceSeparator + strings.TrimSpace(command.ResponsibilitySourceSubject))
	if err != nil {
		return continuedAttemptNotAccepted(ContinuedAttemptResponsibilitySourceUnknown), nil
	}

	return handler.form(ctx, formation{
		identity:  command.Identity,
		parcel:    command.Parcel,
		kind:      domain.ControlledClosureDecision,
		requester: command.Requester,
		reason:    command.Reason,
		at:        command.EffectiveAt,
		spec: func(id domain.ContinuedAttemptDecisionID, granted ports.ContinuedAttemptDecisionAuthorization) (domain.ContinuedAttemptDecisionSpec, error) {
			boundary, err := domain.NewAuthoritativeCutoffBoundary(id.String())
			if err != nil {
				return domain.ContinuedAttemptDecisionSpec{}, err
			}
			return domain.ContinuedAttemptDecisionSpec{
				ID:                          id,
				Kind:                        domain.ControlledClosureDecision,
				Requester:                   command.Requester,
				Decider:                     granted.Decider,
				AuthorityRole:               granted.AuthorityRole,
				AuthoritySnapshot:           granted.Authority,
				Reason:                      command.Reason,
				EffectiveAt:                 command.EffectiveAt,
				CutoffBoundary:              boundary,
				ClosureResponsibilitySource: source,
			}, nil
		},
	})
}

// FormReopening 形成一条重开决定。
//
// 重开比关闭多一道本层自己核的门（票 label-channel/30 做法 6）：原关闭因货主指令形成时，命令要带同一货主账户新的
// 有效授权证据——核「账户与原关闭请求方同一」与「证据非空」两件，只核这两件；等级不比（谁可重开由 PC 登进 `REOPENING`
// 那一格的等级集合承担，pc-gaps/13 裁决 ①），PC 仍只被问一次运营角色的 `REOPENING` 授权。其余门（指不到生效关闭、
// 终局在场、生效时间不晚于关闭）全在聚合的 `Append`。
func (handler *FormContinuedAttemptDecisionHandler) FormReopening(
	ctx context.Context,
	command FormReopeningCommand,
) (ContinuedAttemptDecisionResult, error) {
	if command.RelatedPriorClosure.String() == "" {
		return continuedAttemptNotAccepted(ContinuedAttemptDecisionInvalid), nil
	}
	return handler.form(ctx, formation{
		identity:  command.Identity,
		parcel:    command.Parcel,
		kind:      domain.ReopeningDecision,
		requester: command.Requester,
		reason:    command.Reason,
		at:        command.EffectiveAt,
		// 没开过册的包裹没有任何东西可重开：这不是重复领域规则（领域说的是「指不到生效关闭」），是「没有册可追加」
		// 这一层的事实——为它开一册空的再让 Append 拒，会为一次注定不成立的重开消耗一个决定标识。
		requiresRegister: true,
		beforeIdentity: func(register domain.ContinuedAttemptRegister) ContinuedAttemptDecisionResult {
			return handler.shipperReauthorizationGate(register, command)
		},
		spec: func(id domain.ContinuedAttemptDecisionID, granted ports.ContinuedAttemptDecisionAuthorization) (domain.ContinuedAttemptDecisionSpec, error) {
			return domain.ContinuedAttemptDecisionSpec{
				ID:                  id,
				Kind:                domain.ReopeningDecision,
				Requester:           command.Requester,
				Decider:             granted.Decider,
				AuthorityRole:       granted.AuthorityRole,
				AuthoritySnapshot:   granted.Authority,
				Reason:              command.Reason,
				EffectiveAt:         command.EffectiveAt,
				RelatedPriorClosure: command.RelatedPriorClosure,
			}, nil
		},
	})
}

// shipperReauthorizationGate 只在原关闭因货主指令形成时开口。原关闭不在册上时不在这里答——那是「指不到生效关闭」，
// 归聚合的 Append，本层不抢答；同理原关闭的种类段认不出时也不拦：能读到的关闭都是本编排写的、带种类段，认不出说明
// 有人绕过写面改了库，那要人去看，不是拿一次拒绝盖过去。
func (handler *FormContinuedAttemptDecisionHandler) shipperReauthorizationGate(
	register domain.ContinuedAttemptRegister,
	command FormReopeningCommand,
) ContinuedAttemptDecisionResult {
	var closure domain.ContinuedAttemptDecision
	found := false
	for _, decision := range register.Decisions() {
		if decision.ID() == command.RelatedPriorClosure && decision.Kind() == domain.ControlledClosureDecision {
			closure, found = decision, true
			break
		}
	}
	if !found || closureResponsibilitySourceKindOf(closure.ClosureResponsibilitySource()) != ShipperInstructionResponsibilitySource {
		return ContinuedAttemptDecisionResult{}
	}
	if strings.TrimSpace(command.ShipperAuthorizationEvidence) == "" {
		return continuedAttemptNotAccepted(ContinuedAttemptShipperReauthorizationMissing)
	}
	if closure.Requester().String() == "" || command.ShipperAccount != closure.Requester() {
		return continuedAttemptNotAccepted(ContinuedAttemptShipperAccountMismatch)
	}
	return ContinuedAttemptDecisionResult{}
}

// formation 是两条命令在共同路径上各自不同的那几格。
type formation struct {
	identity         domain.SourceIdentity
	parcel           domain.DeclaredParcelID
	kind             domain.ContinuedAttemptDecisionKind
	requester        domain.RequesterReference
	reason           domain.ContinuedAttemptReasonReference
	at               time.Time
	requiresRegister bool
	// beforeIdentity 在读回册之后、签发标识之前开口；交回非零结果即在此停下。
	beforeIdentity func(domain.ContinuedAttemptRegister) ContinuedAttemptDecisionResult
	spec           func(domain.ContinuedAttemptDecisionID, ports.ContinuedAttemptDecisionAuthorization) (domain.ContinuedAttemptDecisionSpec, error)
}

// form 是两条命令共用的路径：授权 → 当前有效终局 → 册 → 标识 → Append → Insert / Save → 交出判断意图。
//
// 读那几步失败答`未决`（什么都没写，重投会改变结果）；写那几步失败原样上抛（整步随事务回滚）；版本冲突是业务答案。
func (handler *FormContinuedAttemptDecisionHandler) form(ctx context.Context, plan formation) (ContinuedAttemptDecisionResult, error) {
	if plan.identity.TenantID().String() == "" || plan.parcel.String() == "" ||
		plan.reason.String() == "" || plan.at.IsZero() {
		return continuedAttemptNotAccepted(ContinuedAttemptDecisionInvalid), nil
	}
	tenant := plan.identity.TenantID()

	granted, err := handler.deps.Authorizer.AuthorizeContinuedAttemptDecision(ctx, ports.ContinuedAttemptDecisionAuthorizationQuery{
		Identity:  plan.identity,
		Parcel:    plan.parcel,
		Kind:      plan.kind,
		Requester: plan.requester,
		Reason:    plan.reason,
		At:        handler.deps.Clock.Now(),
	})
	if err != nil {
		return continuedAttemptUndecided(ContinuedAttemptAuthorityUnavailable), nil
	}
	switch granted.Outcome {
	case ports.AuthorizationGranted:
	case ports.AuthorizationRefused:
		return ContinuedAttemptDecisionResult{outcome: ContinuedAttemptDecisionNotAuthorized}, nil
	case ports.AuthorizationRulesNotConfigured:
		return continuedAttemptUndecided(ContinuedAttemptAuthorityRulesNotConfigured), nil
	default:
		// 逐取值分派，不留兜底：端口日后多一种答复时这里报错，而不是静默归入上面某一格。
		return ContinuedAttemptDecisionResult{}, ErrUnexpectedAuthorizationOutcome
	}

	current, found, err := handler.deps.Finals.FindCurrentFinal(ctx, tenant, plan.parcel)
	if err != nil {
		return continuedAttemptUndecided(ContinuedAttemptFinalOutcomeUnavailable), nil
	}
	currentFinalPresent := found && current.Finalized

	register, found, err := handler.deps.Registers.FindByParcel(ctx, tenant, plan.parcel)
	if err != nil {
		return continuedAttemptUndecided(ContinuedAttemptDecisionRegisterUnavailable), nil
	}
	if !found {
		if plan.requiresRegister {
			return ContinuedAttemptDecisionResult{outcome: ContinuedAttemptDecisionNotAdmitted}, nil
		}
		register, err = domain.OpenContinuedAttemptRegister(tenant, plan.parcel)
		if err != nil {
			return continuedAttemptNotAccepted(ContinuedAttemptDecisionInvalid), nil
		}
	}
	if plan.beforeIdentity != nil {
		if stopped := plan.beforeIdentity(register); stopped.outcome != ContinuedAttemptDecisionOutcomeInvalid {
			return stopped, nil
		}
	}

	id, err := handler.deps.Identities.NextContinuedAttemptDecisionID(ctx)
	if err != nil {
		return continuedAttemptUndecided(ContinuedAttemptDecisionIdentityUnavailable), nil
	}
	spec, err := plan.spec(id, granted)
	if err != nil {
		return continuedAttemptNotAccepted(ContinuedAttemptDecisionInvalid), nil
	}
	appended, err := register.Append(spec, currentFinalPresent)
	switch {
	case errors.Is(err, domain.ErrContinuedAttemptDecisionNotAdmitted):
		return ContinuedAttemptDecisionResult{outcome: ContinuedAttemptDecisionNotAdmitted}, nil
	case errors.Is(err, domain.ErrInvalidContinuedAttemptDecision):
		return continuedAttemptNotAccepted(ContinuedAttemptDecisionInvalid), nil
	case err != nil:
		return ContinuedAttemptDecisionResult{}, fmt.Errorf("append continued attempt decision: %w", err)
	}

	if found {
		saved, err := handler.deps.Registers.Save(ctx, appended)
		if err != nil {
			return ContinuedAttemptDecisionResult{}, fmt.Errorf("save continued attempt register: %w", err)
		}
		if saved != ports.ContinuedAttemptRegisterSaved {
			return ContinuedAttemptDecisionResult{outcome: ContinuedAttemptDecisionWriteConflict}, nil
		}
	} else {
		inserted, err := handler.deps.Registers.Insert(ctx, appended)
		if err != nil {
			return ContinuedAttemptDecisionResult{}, fmt.Errorf("insert continued attempt register: %w", err)
		}
		if inserted != ports.ContinuedAttemptRegisterInserted {
			// 两次并发的首次关闭撞在开册上：与预期版本对不上同一恢复动作，重读再重放。
			return ContinuedAttemptDecisionResult{outcome: ContinuedAttemptDecisionWriteConflict}, nil
		}
	}

	decision := appended.Decisions()[len(appended.Decisions())-1]
	// 只在落册成功之后：门拒与版本冲突都没落库，一封指着未落库决定的信不该出去。入队失败原样上抛，整步回滚
	// （ADR-0134 决定三）。
	if err := handler.deps.Handoff.HandOffContinuedAttemptDecision(ctx, ports.ContinuedAttemptDecisionHandoffIntent{
		Tenant:     tenant,
		Parcel:     plan.parcel,
		Decision:   decision.ID(),
		Kind:       decision.Kind(),
		OccurredAt: decision.EffectiveAt(),
	}); err != nil {
		return ContinuedAttemptDecisionResult{}, fmt.Errorf("hand off continued attempt decision: %w", err)
	}
	return ContinuedAttemptDecisionResult{outcome: ContinuedAttemptDecisionFormed, decision: decision, hasDecision: true}, nil
}

// Package application 在 settlement-accounting 领域内核与本上下文自有端口之上编排用例。
// 它不含持久化、事务或事件机制，那些仍阻断在 Bento 闸门之后（ADR-0017）。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ControlOutcome 是本用例接受前财务控制分支的应用处理结果。没有一个取值是接受判决：
// CONTEXT 把这条写死——本上下文形成的是估价、冻结、信用暴露或业务限制，是否据此阻断
// 委托由 `parcel-shipment` 决定，本上下文不得自行增设接单门槛。
//
// `已执行`与`业务限制`共用 `ControlApplied`：余额不足同样是一次已经执行过的控制，它的
// 结论装在 `FundsFreeze` 的状态里。分成两个应用结果会让「控制执行过没有」这个问题得靠
// 两处判断回答。
type ControlOutcome uint8

const (
	ControlOutcomeInvalid ControlOutcome = iota
	ControlApplied
	ControlNotApplicable
	ControlRequestConflict
	ControlRequestNotAccepted
	ControlNotFormed
)

func (outcome ControlOutcome) String() string {
	switch outcome {
	case ControlApplied:
		return "CONTROL_APPLIED"
	case ControlNotApplicable:
		return "CONTROL_NOT_APPLICABLE"
	case ControlRequestConflict:
		return "CONTROL_REQUEST_CONFLICT"
	case ControlRequestNotAccepted:
		return "CONTROL_REQUEST_NOT_ACCEPTED"
	case ControlNotFormed:
		return "CONTROL_NOT_FORMED"
	default:
		return ""
	}
}

// NotFormedReason 指名本次为何没有形成控制结果。它是封闭集合而非自由文本，这样待判断才能
// 按依赖阶段分类统计；取值与产生它的那条路径同时出现。
type NotFormedReason uint8

const (
	NotFormedReasonNone NotFormedReason = iota
	ControlPolicyUnavailable
	ControlPolicyNotConfigured
	BalanceUnavailable
	FreezeLedgerUnavailable
	CreditStandingUnavailable
	ExposureLedgerUnavailable
	// CreditBasisUnavailable / CreditBasisNotConfigured 是授信依据那一口的两格（ADR-0127）：调不通
	// 等重试，未配置等租户把解析键与信用政策正文补齐。CreditRatioBaseUndecided 是第三格：政策授的是
	// 比例额度而正文没有声明基数——自 ADR-0129 起构造门不再放出这种正文，它只剩一条来路：重建门读回的、
	// 那之前登进去的存量正文；不折成金额、不默认，要用得上只能发新版本。
	CreditBasisUnavailable
	CreditBasisNotConfigured
	CreditRatioBaseUndecided
	// CreditRatioBaseUnavailable / CreditRatioBaseNotEstablished 是基数取值那一口的两格（ADR-0129 决定三）：
	// 读不回等重试；尚无事实（未登记运营余额 / 尚无有效对账单）等事实出现——不是补配置，也不折 0 不折无限。
	// 两格不并成一格：并了就答不出该等谁。
	CreditRatioBaseUnavailable
	CreditRatioBaseNotEstablished
)

func (reason NotFormedReason) String() string {
	switch reason {
	case ControlPolicyUnavailable:
		return "CONTROL_POLICY_UNAVAILABLE"
	case ControlPolicyNotConfigured:
		return "CONTROL_POLICY_NOT_CONFIGURED"
	case BalanceUnavailable:
		return "BALANCE_UNAVAILABLE"
	case FreezeLedgerUnavailable:
		return "FREEZE_LEDGER_UNAVAILABLE"
	case CreditStandingUnavailable:
		return "CREDIT_STANDING_UNAVAILABLE"
	case ExposureLedgerUnavailable:
		return "EXPOSURE_LEDGER_UNAVAILABLE"
	case CreditBasisUnavailable:
		return "CREDIT_BASIS_UNAVAILABLE"
	case CreditBasisNotConfigured:
		return "CREDIT_BASIS_NOT_CONFIGURED"
	case CreditRatioBaseUndecided:
		return "CREDIT_RATIO_BASE_UNDECIDED"
	case CreditRatioBaseUnavailable:
		return "CREDIT_RATIO_BASE_UNAVAILABLE"
	case CreditRatioBaseNotEstablished:
		return "CREDIT_RATIO_BASE_NOT_ESTABLISHED"
	default:
		return ""
	}
}

// ContinuationReference 让调用方把停下的控制请求重新接上。它属应用层而不属领域：领域拥有
// 冻结与限制，不认识依赖失败这回事。
type ContinuationReference struct{ value string }

func (reference ContinuationReference) String() string {
	return reference.value
}

type ApplyPreAcceptanceControlCommand struct {
	TenantID    domain.TenantID
	RequestID   domain.ControlRequestID
	Scope       domain.SettlementScope
	AmountMinor int64
	Association domain.BusinessAssociationReference
	AsOf        domain.ControlAsOf
	// Resolution 回指调用方那次商业解析，控制策略视图凭它向商业侧提问
	// （sa-preacceptance-policy-view/01 裁决）。作用域与它同源：作用域正是从这次解析的
	// 结算政策回显派生的，所以要求调用方一并带上不是多一次索取，是把已有的那一份写明。
	Resolution domain.CommercialResolutionReference
}

// minimumIdentityEstablished 在读任何权威之前判断该不该读。金额算在受理条件里而不留给
// 领域构造期：非正数金额的请求根本不该去读这个客户的余额，而一次已经发出的读取收不回来。
//
// 商业解析回指同理算在受理条件里，而且理由更硬：少了它，控制策略视图根本无从提问，
// 而它答出的任何一格都会是假话——`未登记`会把「没问成」说成「商业侧没登记过」，等来的
// 是租户去补一份其实已经存在的声明。停在`未受理`才说得清是调用方少给了键。
func (command ApplyPreAcceptanceControlCommand) minimumIdentityEstablished() bool {
	return command.TenantID.String() != "" &&
		command.RequestID.String() != "" &&
		command.Scope.LegalEntity().String() != "" &&
		command.Scope.Account().String() != "" &&
		command.Scope.Currency().String() != "" &&
		command.Association.String() != "" &&
		command.AsOf.Valid() &&
		command.AmountMinor > 0 &&
		command.Resolution.String() != ""
}

type ApplyPreAcceptanceControlResult struct {
	outcome       ControlOutcome
	freeze        domain.FundsFreeze
	hasFreeze     bool
	exposure      domain.CreditExposure
	hasExposure   bool
	executed      []ExecutedControl
	jointPass     domain.JointPassCondition
	controlPolicy domain.ControlPolicyReference
	method        domain.SettlementMethod
	adoptedPolicy domain.AdoptedPolicyReference
	creditPolicy  domain.CreditPolicyReference
	controlledAt  time.Time
	asOf          domain.ControlAsOf
	basis         domain.ControlBasisReference
	reason        NotFormedReason
	continuation  ContinuationReference
}

// ExecutedControl 记策略正文里一项已经执行过的控制：哪一种、排第几、有没有形成`业务限制`。
// 它是执行记录不是判决：`业务限制`是这一项自己的结论（余额不足 / 超额 / 逾期），多项合起来
// 算不算通过由 parcel-shipment 按共同通过条件判（CONTEXT）。
type ExecutedControl struct {
	kind       domain.ControlKind
	order      uint32
	restricted bool
}

func (executed ExecutedControl) Kind() domain.ControlKind {
	return executed.kind
}

func (executed ExecutedControl) Order() uint32 {
	return executed.order
}

func (executed ExecutedControl) Restricted() bool {
	return executed.restricted
}

func (result ApplyPreAcceptanceControlResult) Outcome() ControlOutcome {
	return result.outcome
}

// Freeze 只在 PREPAID_FREEZE 那一项实际执行过时给出，`无控制`、冲突、未受理与待判断一律没有。
// 交回一个零值冻结，正是 CONTEXT 禁止的「用虚假冻结冒充控制」。同一版策略内一种控制至多一项
// （领域构造期守住），所以这里至多一个冻结。
func (result ApplyPreAcceptanceControlResult) Freeze() (domain.FundsFreeze, bool) {
	return result.freeze, result.hasFreeze
}

// Exposure 只在 CREDIT_CHECK 那一项实际执行过时给出。它与冻结**可以同时在场**：一份策略可以
// 同时要求预付冻结与信用校验（ADR-0115 允许的组合），调用方要两个都看，不能看到一个就停。
func (result ApplyPreAcceptanceControlResult) Exposure() (domain.CreditExposure, bool) {
	return result.exposure, result.hasExposure
}

// ExecutedControls 按判断顺序交回已经执行过的控制项。它可能短于策略的控制项：一项形成
// `业务限制`之后，共同通过条件为「全部通过」时后续项不再执行（见 Handle）。
func (result ApplyPreAcceptanceControlResult) ExecutedControls() []ExecutedControl {
	return append([]ExecutedControl(nil), result.executed...)
}

// JointPassCondition 原样带回策略的共同通过条件，供 parcel-shipment 据以形成接受判断；本
// 上下文不据它汇总。
func (result ApplyPreAcceptanceControlResult) JointPassCondition() domain.JointPassCondition {
	return result.jointPass
}

// ControlPolicy 是本次控制项所出自的接受前财务控制策略版本；Method 与 AdoptedPolicy 是实际
// 采用的结算方式与结算政策引用——CONTEXT 要求每项冻结与信用暴露都保存后两者，前者让事后能
// 回答「这几项控制凭哪一版策略执行」。
func (result ApplyPreAcceptanceControlResult) ControlPolicy() domain.ControlPolicyReference {
	return result.controlPolicy
}

func (result ApplyPreAcceptanceControlResult) Method() domain.SettlementMethod {
	return result.method
}

func (result ApplyPreAcceptanceControlResult) AdoptedPolicy() domain.AdoptedPolicyReference {
	return result.adoptedPolicy
}

// CreditPolicy 是 CREDIT_CHECK 那一项据以判额度的信用政策版本（ADR-0127）：额度出自它，暴露与
// 限制结果因此有一份出自政策版本的额度依据可比（AT-SA-171 的「分别保存政策」）。只在信用校验
// 实际执行过时给出；额度没有别的来源，所以它在场与暴露在场是同一件事。
func (result ApplyPreAcceptanceControlResult) CreditPolicy() domain.CreditPolicyReference {
	return result.creditPolicy
}

// ControlledAt 是本次控制作出的时间，与 AsOf 分开：`asOf` 决定按哪一版策略判断，控制时间
// 说明资金何时被占用。压成一个会让重放看起来像一次新的控制。
func (result ApplyPreAcceptanceControlResult) ControlledAt() time.Time {
	return result.controlledAt
}

func (result ApplyPreAcceptanceControlResult) AsOf() domain.ControlAsOf {
	return result.asOf
}

// ControlBasis 只在`无控制`时给出。CONTEXT 要求这个结果携带商业不适用依据——没有依据的
// 「无控制」看起来像信用通过，而它其实是一次未执行的控制。
func (result ApplyPreAcceptanceControlResult) ControlBasis() domain.ControlBasisReference {
	return result.basis
}

func (result ApplyPreAcceptanceControlResult) NotFormedReason() NotFormedReason {
	return result.reason
}

func (result ApplyPreAcceptanceControlResult) ContinuationReference() ContinuationReference {
	return result.continuation
}

type ApplyPreAcceptanceControlHandler struct {
	policy      ports.PreAcceptanceControlPolicyView
	balance     ports.OperationalBalanceView
	ledger      ports.FreezeLedgerRepository
	credit      ports.CreditStandingView
	creditBasis ports.CreditBasisView
	ratioBases  ports.CreditRatioBaseView
	exposures   ports.CreditExposureLedgerRepository
	clock       ports.Clock
}

// ApplyPreAcceptanceControlDeps 的每一件都是 mandatory：预付路与账期路各读各的账，但一份策略可以同时
// 要求两项控制（ADR-0115 允许的组合），装配时不知道租户会登记哪一种，缺任何一件都会在第一笔命中
// 那条路的委托到达时 panic。CreditBasis 是账期分支的授信额度来源（ADR-0127 决定四）；RatioBases 是
// 比例额度的基数在本上下文账本里的取值（ADR-0129 决定三）——额度是比例时没有它折不出金额，与别的
// 依赖同一道构造门。
type ApplyPreAcceptanceControlDeps struct {
	Policy      ports.PreAcceptanceControlPolicyView
	Balance     ports.OperationalBalanceView
	Freezes     ports.FreezeLedgerRepository
	Credit      ports.CreditStandingView
	CreditBasis ports.CreditBasisView
	RatioBases  ports.CreditRatioBaseView
	Exposures   ports.CreditExposureLedgerRepository
	Clock       ports.Clock
}

// ErrNilDependency 是构造门对缺件的唯一答复；哪一件缺在包装信息里点名。它必须是构造期的错误而不是
// 运行期的 panic 或静默降级：装配疏漏要在进程启动那一刻炸出来，而不是等某个租户第一笔账期委托到达
// 时才发现额度无处可取——那时它与「租户没登记信用政策」在结果上长得一模一样。
//
// 它是本包所有编排构造门共用的一枚（NewRequestBuyEvaluationHandler 同样包它），文本因此不带任何一条编排的
// 名字——哪条编排缺件由调用方的包装信息说，哨兵只说「缺件」这一件事。
var ErrNilDependency = errors.New("settlement accounting: dependency is nil")

func NewApplyPreAcceptanceControlHandler(deps ApplyPreAcceptanceControlDeps) (*ApplyPreAcceptanceControlHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"control policy view", deps.Policy == nil},
		{"operational balance view", deps.Balance == nil},
		{"freeze ledger repository", deps.Freezes == nil},
		{"credit standing view", deps.Credit == nil},
		{"credit basis view", deps.CreditBasis == nil},
		{"credit ratio base view", deps.RatioBases == nil},
		{"credit exposure ledger repository", deps.Exposures == nil},
		{"clock", deps.Clock == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: %s", ErrNilDependency, dependency.name)
		}
	}
	return &ApplyPreAcceptanceControlHandler{
		policy:      deps.Policy,
		balance:     deps.Balance,
		ledger:      deps.Freezes,
		credit:      deps.Credit,
		creditBasis: deps.CreditBasis,
		ratioBases:  deps.RatioBases,
		exposures:   deps.Exposures,
		clock:       deps.Clock,
	}, nil
}

// Handle 为一次委托接受前的财务控制请求占用资金。它不形成委托接受或拒绝：余额不足是本
// 上下文报告的业务限制，是否据此阻断委托由 parcel-shipment 决定。
func (handler *ApplyPreAcceptanceControlHandler) Handle(
	ctx context.Context,
	command ApplyPreAcceptanceControlCommand,
) (ApplyPreAcceptanceControlResult, error) {
	if !command.minimumIdentityEstablished() {
		return ApplyPreAcceptanceControlResult{outcome: ControlRequestNotAccepted}, nil
	}

	// 控制策略先于余额与登记册。合同规定本范围无财务控制时，连读余额都不该发生——那次
	// 读取既是白做的，也已经取了这个客户的资金状况。
	policy, configured, err := handler.policy.LoadControlPolicy(
		ctx, command.TenantID, command.Scope, command.Resolution)
	if err != nil {
		// 商业侧调不通形成待判断，不读成「不要求控制」。后者正是 CONTEXT 禁止的默认信用
		// 通过：一次商业故障会因此变成一个看起来通过了的接受前控制。
		return handler.notFormed(command, ControlPolicyUnavailable), nil
	}
	if !configured {
		// 未登记同样不是`无控制`（ADR-0054）。两者的恢复动作相反：这一格等商业侧登记
		// PAR-COM-15，而`无控制`是合同已经说过的话，据它可以放行接受判断。
		return handler.notFormed(command, ControlPolicyNotConfigured), nil
	}
	if !policy.ControlRequired() {
		return ApplyPreAcceptanceControlResult{
			outcome: ControlNotApplicable,
			asOf:    command.AsOf,
			basis:   policy.Basis(),
		}, nil
	}

	// 共同通过条件先穷举再执行（ADR-0025 全函数）：一个本上下文还不认识的组合子若等到出现
	// `业务限制`那一刻才发现，前面的项已经占了资金。条件在策略构造期已保证有效，落到 default
	// 只能是新增取值没接分支——编程错误上抛。
	switch policy.JointPassCondition() {
	case domain.AllControlsPass:
	default:
		return ApplyPreAcceptanceControlResult{}, fmt.Errorf(
			"pre-acceptance control: unhandled joint pass condition %d", uint8(policy.JointPassCondition()))
	}

	// 一次请求的所有控制项共用一个控制时刻：它们是同一次接受前控制的几步，不是几次控制。
	controlledAt := handler.clock.Now()
	result := ApplyPreAcceptanceControlResult{
		outcome:       ControlApplied,
		jointPass:     policy.JointPassCondition(),
		controlPolicy: policy.ControlPolicy(),
		method:        policy.Method(),
		adoptedPolicy: policy.AdoptedPolicy(),
		controlledAt:  controlledAt,
		asOf:          command.AsOf,
	}

	// 按策略正文的判断顺序逐项执行（ADR-0122 决定二）。控制种类决定走哪本账：预付冻结占资金、
	// 信用校验占额度，两本账互不借用（ADR-0047 的两本账，选路开关从结算方式换成了控制种类）。
	// 种类在策略构造期已保证有效，落到 default 只能是新增取值没接分支——编程错误上抛。
	for _, item := range policy.Items() {
		var step controlStep
		var halted *ApplyPreAcceptanceControlResult
		var err error
		switch item.Kind() {
		case domain.PrepaidFreezeControl:
			step, halted, err = handler.freezeFunds(ctx, command, controlledAt)
		case domain.CreditCheckControl:
			step, halted, err = handler.exposeCredit(ctx, command, controlledAt)
		default:
			return ApplyPreAcceptanceControlResult{}, fmt.Errorf(
				"pre-acceptance control: unhandled control kind %d", uint8(item.Kind()))
		}
		if err != nil {
			return ApplyPreAcceptanceControlResult{}, err
		}
		if halted != nil {
			// 依赖不可用、请求冲突、未受理：整个请求停在这一步。前面已经执行的项留在各自账本
			// 里——账本对同一请求身份幂等（重放交回原冻结 / 原暴露），续办时从头重走一遍不会
			// 二次占用，也就不需要在这里回滚。
			return *halted, nil
		}
		result.executed = append(result.executed, ExecutedControl{
			kind: item.Kind(), order: item.Order(), restricted: step.restricted,
		})
		switch item.Kind() {
		case domain.PrepaidFreezeControl:
			result.freeze, result.hasFreeze = step.freeze, true
		case domain.CreditCheckControl:
			result.exposure, result.hasExposure = step.exposure, true
			result.creditPolicy = step.creditPolicy
		}
		if step.restricted {
			// 「全部通过」之下，一项已形成`业务限制`，后面的项无论结果如何都改不了共同通过条件
			// 的答案；继续执行只会为一笔多半不会接受的委托占更多资金、给释放路径多一处要认领。
			// 这是判断顺序存在的意义，不是本上下文在汇总——每一项已执行的结果都原样交回。
			// 将来放宽出第二种组合子时，这一格要按那个组合子另判（上面的穷举会先炸出来）。
			return result, nil
		}
	}
	return result, nil
}

// controlStep 是一项控制执行完的产出：冻结或暴露之一，以及它有没有形成`业务限制`；信用校验
// 一项还带上额度出自哪一版信用政策。
type controlStep struct {
	freeze       domain.FundsFreeze
	exposure     domain.CreditExposure
	creditPolicy domain.CreditPolicyReference
	restricted   bool
}

// freezeFunds 执行 PREPAID_FREEZE 一项：读运营余额、在冻结账本上占用金额。第二个返回值非 nil
// 表示整个请求要停在这一步（待判断 / 冲突 / 未受理），调用方原样交回。
func (handler *ApplyPreAcceptanceControlHandler) freezeFunds(
	ctx context.Context,
	command ApplyPreAcceptanceControlCommand,
	controlledAt time.Time,
) (controlStep, *ApplyPreAcceptanceControlResult, error) {
	balance, err := handler.balance.LoadBalance(ctx, command.TenantID, command.Scope)
	if err != nil {
		return controlStep{}, handler.haltNotFormed(command, BalanceUnavailable), nil
	}

	ledger, err := handler.ledger.LoadForScope(ctx, command.TenantID, command.Scope)
	if err != nil || ledger == nil {
		return controlStep{}, handler.haltNotFormed(command, FreezeLedgerUnavailable), nil
	}

	request, err := domain.NewFreezeRequest(
		command.RequestID,
		command.Scope,
		command.AmountMinor,
		command.Association,
		controlledAt,
	)
	if err != nil {
		return controlStep{}, &ApplyPreAcceptanceControlResult{outcome: ControlRequestNotAccepted}, nil
	}

	freeze, err := ledger.Freeze(request, balance)
	if err != nil {
		// 请求冲突是业务答案而非技术故障：调用方必须能据以纠正，而不是当作故障重试。
		// 作用域错配则是编程错误，它意味着取回的余额根本不属于这个请求，必须上抛。
		if errors.Is(err, domain.ErrControlRequestConflict) {
			return controlStep{}, &ApplyPreAcceptanceControlResult{outcome: ControlRequestConflict}, nil
		}
		return controlStep{}, nil, fmt.Errorf("freeze funds: %w", err)
	}

	if err := handler.ledger.Save(ctx, command.TenantID, command.Scope, ledger); err != nil {
		// 控制没能落库就不算执行过。交回一个未落库的冻结，下游会引用一笔查不回来的占用。
		return controlStep{}, handler.haltNotFormed(command, FreezeLedgerUnavailable), nil
	}

	return controlStep{freeze: freeze, restricted: freeze.Status() == domain.FreezeRestricted}, nil, nil
}

// exposeCredit 执行 CREDIT_CHECK 一项：先向商业侧索取授信依据、比例额度再按声明的基数取本上下文自己的数
// 折成金额（authorizedMinorOf）、再读信用状况，在暴露账本上占用额度。逾期与超额形成`业务限制`装在暴露的
// 状态里，与预付冻结的余额不足同构。
//
// 授信依据先于信用状况（ADR-0127 决定四）：额度出自闭包采用的信用政策版本，已占用暴露与逾期
// 才是本上下文自己的事实；闭包没采用信用政策时连读状况都不该发生——那次读取既是白做的，也已
// 经取了这个客户的信用状况。这与「控制策略先于余额」是同一条顺序纪律。
func (handler *ApplyPreAcceptanceControlHandler) exposeCredit(
	ctx context.Context,
	command ApplyPreAcceptanceControlCommand,
	controlledAt time.Time,
) (controlStep, *ApplyPreAcceptanceControlResult, error) {
	basis, configured, err := handler.creditBasis.LoadCreditBasis(
		ctx, command.TenantID, command.Scope, command.Resolution)
	if err != nil {
		// 商业侧调不通形成待判断，不读成「有额度」或「零额度」——两者都是 CONTEXT 禁止本上下文
		// 替商业侧说的话。
		return controlStep{}, handler.haltNotFormed(command, CreditBasisUnavailable), nil
	}
	if !configured {
		return controlStep{}, handler.haltNotFormed(command, CreditBasisNotConfigured), nil
	}
	authorizedMinor, halted, err := handler.authorizedMinorOf(ctx, command, basis)
	if err != nil || halted != nil {
		return controlStep{}, halted, err
	}

	standing, err := handler.credit.LoadCreditStanding(ctx, command.TenantID, command.Scope)
	if err != nil {
		return controlStep{}, handler.haltNotFormed(command, CreditStandingUnavailable), nil
	}
	// 额度一律取信用依据（ADR-0127 决定五 contract 段）：登记状况里的 limit 自此只是一列登记值，
	// 不再有「依赖未接就拿它当额度」的分支。出处随额度一起换上，暴露入册时带着它落库。
	standing, err = standing.WithAuthorizedLimit(authorizedMinor, basis.Policy())
	if err != nil {
		// 作用域不合法或额度为负都进不了各自的构造门，走到这里是编程错误，上抛。
		return controlStep{}, nil, fmt.Errorf("expose credit: authorized limit: %w", err)
	}

	ledger, err := handler.exposures.LoadForScope(ctx, command.TenantID, command.Scope)
	if err != nil || ledger == nil {
		return controlStep{}, handler.haltNotFormed(command, ExposureLedgerUnavailable), nil
	}

	request, err := domain.NewExposureRequest(
		command.RequestID,
		command.Scope,
		command.AmountMinor,
		command.Association,
		controlledAt,
	)
	if err != nil {
		return controlStep{}, &ApplyPreAcceptanceControlResult{outcome: ControlRequestNotAccepted}, nil
	}

	exposure, err := ledger.Expose(request, standing)
	if err != nil {
		if errors.Is(err, domain.ErrControlRequestConflict) {
			return controlStep{}, &ApplyPreAcceptanceControlResult{outcome: ControlRequestConflict}, nil
		}
		return controlStep{}, nil, fmt.Errorf("expose credit: %w", err)
	}

	if err := handler.exposures.Save(ctx, command.TenantID, command.Scope, ledger); err != nil {
		return controlStep{}, handler.haltNotFormed(command, ExposureLedgerUnavailable), nil
	}

	return controlStep{
		exposure:     exposure,
		creditPolicy: basis.Policy(),
		restricted:   exposure.Status() == domain.ExposureRestricted,
	}, nil, nil
}

// authorizedMinorOf 把授信依据折成一个金额：金额额度原样取；比例额度按声明的基数向本上下文自己的账本取值、
// 交领域折算（ADR-0129 决定三）。第二个返回值非 nil 表示整个请求停在这一步。
//
// 基数在授信依据之后、信用状况之前取：没有基数折不出额度，连读状况都不该发生——与「授信依据先于信用状况」
// 是同一条顺序纪律。三格照端口：读不回停 CREDIT_RATIO_BASE_UNAVAILABLE（等重试）；尚无事实停
// CREDIT_RATIO_BASE_NOT_ESTABLISHED（等事实出现，不折 0 不折无限）。基数未声明的比例只剩存量正文一条来路，
// 停在 CREDIT_RATIO_BASE_UNDECIDED；折算本身报错（溢出 / 领域门拒）是编程错误，上抛。
func (handler *ApplyPreAcceptanceControlHandler) authorizedMinorOf(
	ctx context.Context,
	command ApplyPreAcceptanceControlCommand,
	basis domain.CreditBasis,
) (int64, *ApplyPreAcceptanceControlResult, error) {
	if minor, isAmount := basis.AmountMinor(); isAmount {
		return minor, nil, nil
	}
	base, declared := basis.RatioBase()
	if !declared {
		return 0, handler.haltNotFormed(command, CreditRatioBaseUndecided), nil
	}
	baseMinor, established, err := handler.ratioBases.LoadCreditRatioBase(ctx, command.TenantID, command.Scope, base)
	if err != nil {
		return 0, handler.haltNotFormed(command, CreditRatioBaseUnavailable), nil
	}
	if !established {
		return 0, handler.haltNotFormed(command, CreditRatioBaseNotEstablished), nil
	}
	minor, err := basis.LimitOnBase(baseMinor)
	if err != nil {
		return 0, nil, fmt.Errorf("expose credit: ratio limit on %s: %w", base, err)
	}
	return minor, nil, nil
}

func (handler *ApplyPreAcceptanceControlHandler) haltNotFormed(
	command ApplyPreAcceptanceControlCommand,
	reason NotFormedReason,
) *ApplyPreAcceptanceControlResult {
	halted := handler.notFormed(command, reason)
	return &halted
}

// notFormed 构造所有待判断共用的那一种形状，让它们全部带上原因与续办引用：一次停下的控制
// 请求能不能接回去，不该取决于它停在哪一步。
func (handler *ApplyPreAcceptanceControlHandler) notFormed(
	command ApplyPreAcceptanceControlCommand,
	reason NotFormedReason,
) ApplyPreAcceptanceControlResult {
	return ApplyPreAcceptanceControlResult{
		outcome:      ControlNotFormed,
		asOf:         command.AsOf,
		reason:       reason,
		continuation: continuationFor(command, reason),
	}
}

// continuationFor 由请求身份、作用域、金额与原因共同派生，因此同一请求因同一原因停滞时拿到
// 的引用始终相同——这正是调用方能查询原次尝试而不必靠猜的原因。
//
// 商业解析回指也在摘要里：它决定策略视图问到的是哪一份合同，换了回指就是换了一次问答，
// 两者共用一条续办引用会让续办方按引用查回来的是另一份商业依据下的停摆。
func continuationFor(command ApplyPreAcceptanceControlCommand, reason NotFormedReason) ContinuationReference {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		reason.String(),
		command.TenantID.String(),
		command.RequestID.String(),
		command.Scope.LegalEntity().String(),
		command.Scope.Account().String(),
		command.Scope.Currency().String(),
		fmt.Sprint(command.AmountMinor),
		command.Association.String(),
		command.AsOf.Semantic().String(),
		command.AsOf.StrategyVersion().String(),
		command.AsOf.At().Format(time.RFC3339Nano),
		command.Resolution.String(),
	}, "\x00")))
	return ContinuationReference{value: "CONT-" + hex.EncodeToString(digest[:8])}
}

package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// DecideDisclosureOutcome 是形成客户可见异常披露决定的应用处理结果。三态结论（披露、
// 暂不披露、待授权）都算「已决定」——它们是 `UC-VE-006` 步 3 与步 5 各自的结果，不是
// 没有结果；哪一态由结果携带的决定自己说。
type DecideDisclosureOutcome uint8

const (
	DecideDisclosureOutcomeInvalid DecideDisclosureOutcome = iota
	DisclosureDecided
	DisclosureExistingResult
	DecideDisclosureUndecided
	DecideDisclosureNotAccepted
)

func (outcome DecideDisclosureOutcome) String() string {
	switch outcome {
	case DisclosureDecided:
		return "DECIDED"
	case DisclosureExistingResult:
		return "EXISTING_RESULT"
	case DecideDisclosureUndecided:
		return "UNDECIDED"
	case DecideDisclosureNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// DecideDisclosureUndecidedReason 指名本轮停在哪一步。`规则未配置`与`规则答不出`分开，
// 理由同通知策略那一对：一个等租户登记 `PAR-VIS-07`，一个重试依赖。`客户归属不可确定`
// 自占一格：那不是故障也不是未配置——parcel-shipment 此刻没有已接受委托声明这件包裹，
// 「归属不可确定期间不形成视图，也不得发明账户」（CONTEXT）在披露决定上同样成立。
type DecideDisclosureUndecidedReason uint8

const (
	DecideDisclosureUndecidedReasonNone DecideDisclosureUndecidedReason = iota
	DisclosureEpisodeStoreUnavailable
	DisclosureCustomerAccountUnavailable
	DisclosureCustomerAccountUndeterminable
	DisclosureRuleUnavailable
	DisclosureRuleNotConfigured
	DisclosureDecisionStoreUnavailable
)

func (reason DecideDisclosureUndecidedReason) String() string {
	switch reason {
	case DisclosureEpisodeStoreUnavailable:
		return "DISCLOSURE_EPISODE_STORE_UNAVAILABLE"
	case DisclosureCustomerAccountUnavailable:
		return "DISCLOSURE_CUSTOMER_ACCOUNT_UNAVAILABLE"
	case DisclosureCustomerAccountUndeterminable:
		return "DISCLOSURE_CUSTOMER_ACCOUNT_UNDETERMINABLE"
	case DisclosureRuleUnavailable:
		return "DISCLOSURE_RULE_UNAVAILABLE"
	case DisclosureRuleNotConfigured:
		return "DISCLOSURE_RULE_NOT_CONFIGURED"
	case DisclosureDecisionStoreUnavailable:
		return "DISCLOSURE_DECISION_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// DecideDisclosureCommand 指名要对哪个信号形成披露决定：对象与信号类型定位发作期
// （SignalEpisodeStore 的键）。命令里没有客户账户——客户归属由 parcel-shipment 的当前
// 已接受委托给出，本上下文只按包裹反查（CONTEXT「客户归属」），调用方不替它断言。
// 也没有披露结论——结论由规则与用例形成，命令带结论就是让调用方替规则作判断。租户
// 显式随命令到达（ADR-0003）。
type DecideDisclosureCommand struct {
	TenantID domain.TenantID
	Parcel   domain.TrackedParcelReference
	Kind     domain.ExceptionSignalKindReference
}

type DecideDisclosureResult struct {
	outcome     DecideDisclosureOutcome
	decision    domain.DisclosureDecision
	hasDecision bool
	reason      DecideDisclosureUndecidedReason
}

func (result DecideDisclosureResult) Outcome() DecideDisclosureOutcome {
	return result.outcome
}

// Decision 只在决定成立（本轮或此前）时给出。
func (result DecideDisclosureResult) Decision() (domain.DisclosureDecision, bool) {
	return result.decision, result.hasDecision
}

func (result DecideDisclosureResult) UndecidedReason() DecideDisclosureUndecidedReason {
	return result.reason
}

type DecideDisclosureDeps struct {
	Episodes  ports.SignalEpisodeStore
	Customers ports.ParcelCustomerAccountView
	Rules     ports.ExceptionDisclosureRuleView
	Decisions ports.DisclosureDecisionStore
	Clock     ports.Clock
}

type DecideDisclosureHandler struct {
	deps DecideDisclosureDeps
}

func NewDecideDisclosureHandler(deps DecideDisclosureDeps) *DecideDisclosureHandler {
	return &DecideDisclosureHandler{deps: deps}
}

// Handle 是 `UC-VE-006` 的前半——从内部信号形成客户可见异常的披露决定：受理（对象与
// 类型缺一即未受理）→ 找最近发作期（没有信号就没有可披露的异常，未受理）→ 反查客户
// 归属（不可确定即未决，不发明账户）→ 幂等按（发作期+客户）→ 取当前适用的披露规则
// （未配置即未决，不虚构可见性）→ 由规则三格形成三态结论 → 决定落库。
//
// 决定与通知是两步（`UC-VE-006` 步 3–5 与步 6）：这里只形成决定，`披露`结论的通知由
// NotifyCustomerHandler 按已登记的决定另起一轮；查询与门户展示不经这里。`UC-VE-001`
// 步 7 的关务披露走同一函数、同一分层规则，只是另一个入口。
func (handler *DecideDisclosureHandler) Handle(
	ctx context.Context,
	command DecideDisclosureCommand,
) (DecideDisclosureResult, error) {
	if command.TenantID.String() == "" ||
		command.Parcel.String() == "" ||
		command.Kind.String() == "" {
		return DecideDisclosureResult{outcome: DecideDisclosureNotAccepted}, nil
	}

	episode, found, err := handler.deps.Episodes.FindLatest(ctx, command.TenantID, command.Parcel, command.Kind)
	if err != nil {
		return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureEpisodeStoreUnavailable}, nil
	}
	if !found {
		// 没有这一类的信号发作期就没有可披露的客户可见异常——异常案件存在都不自动要求
		// 披露（CONTEXT），何况连信号都没有。
		return DecideDisclosureResult{outcome: DecideDisclosureNotAccepted}, nil
	}

	customer, determinable, err := handler.deps.Customers.FindCustomerAccount(ctx, command.TenantID, command.Parcel)
	if err != nil {
		return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureCustomerAccountUnavailable}, nil
	}
	if !determinable {
		return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureCustomerAccountUndeterminable}, nil
	}

	existing, found, err := handler.deps.Decisions.FindCurrent(ctx, command.TenantID, episode.ID(), customer)
	if err != nil {
		return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureDecisionStoreUnavailable}, nil
	}
	if found {
		// 同一发作期对同一客户已经决定过：重放返回原决定，不重判。待授权转披露是授权
		// 那一步的事（另一个入口形成新版本），不由再跑一遍本用例完成。
		return DecideDisclosureResult{outcome: DisclosureExistingResult, decision: existing, hasDecision: true}, nil
	}

	snapshot := episode.Snapshot()
	rule, configured, err := handler.deps.Rules.RuleForSignal(ctx, customer, snapshot.Kind, snapshot.Confidence)
	if err != nil {
		return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureRuleUnavailable}, nil
	}
	if !configured {
		// 披露规则属待登记实例参数（`PAR-VIS-07`）。没有规则时既不能说「披露」也不能说
		// 「不披露」——后者同样是一次没人作过的披露决定，如实停在未决等租户登记。
		return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureRuleNotConfigured}, nil
	}

	conclusion, content := concludeDisclosure(rule)
	decision, err := domain.DecideDisclosure(
		episode.ID(),
		customer,
		rule.Policy,
		conclusion,
		content,
		handler.deps.Clock.Now(),
	)
	if err != nil {
		// 规则说披露却没给内容快照、或没给规则引用——那是端口坏答复（登记面本该拦下），
		// 上抛而不吞成某一态。
		return DecideDisclosureResult{}, fmt.Errorf("decide disclosure: %w", err)
	}

	saved, err := handler.deps.Decisions.Save(ctx, command.TenantID, decision)
	if err != nil {
		return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureDecisionStoreUnavailable}, nil
	}
	switch saved {
	case ports.DisclosureDecisionSaved:
		return DecideDisclosureResult{outcome: DisclosureDecided, decision: decision, hasDecision: true}, nil
	case ports.DisclosureDecisionAlreadyRecorded:
		existing, found, err := handler.deps.Decisions.FindCurrent(ctx, command.TenantID, episode.ID(), customer)
		if err != nil || !found {
			return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureDecisionStoreUnavailable}, nil
		}
		return DecideDisclosureResult{outcome: DisclosureExistingResult, decision: existing, hasDecision: true}, nil
	default:
		return DecideDisclosureResult{}, fmt.Errorf("decide disclosure: unexpected save outcome %d", saved)
	}
}

// concludeDisclosure 把规则三格译成三态结论。顺序即 `UC-VE-006` 的两道门：先问披露条件
// 成不成立（不成立即`暂不披露`，`AT-VE-099`），再问批准范围允不允许自动发布（不允许即
// `待授权`，`AT-VE-100`；允许才是`披露`，`AT-VE-102`）。默认人工确认是 `PAR-VIS-07` 的
// 原则——自动发布必须由规则明说，这里不替它放行。
func concludeDisclosure(rule ports.ExceptionDisclosureRule) (domain.DisclosureConclusion, domain.DisclosureContentReference) {
	switch {
	case !rule.Disclosable:
		return domain.NotYetDisclosable, domain.DisclosureContentReference{}
	case !rule.AutoRelease:
		return domain.AwaitingAuthorization, domain.DisclosureContentReference{}
	default:
		return domain.DiscloseToCustomer, rule.Content
	}
}

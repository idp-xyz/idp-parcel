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
	// DisclosureAuthorized 是授权确认真形成了一份`披露`决定。
	DisclosureAuthorized
	// DisclosureNotAwaitingAuthorization 是这条信号此刻没有一份停在`待授权`的决定可确认：还没判过，或判成了`暂不披露`。
	DisclosureNotAwaitingAuthorization
	// DisclosureConditionNoLongerMet 是决定停在`待授权`期间披露规则变了，此刻的规则说这条信号不披露。确认的人只放行
	// 发布，不替规则说披露条件成立。
	DisclosureConditionNoLongerMet
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
	case DisclosureAuthorized:
		return "AUTHORIZED"
	case DisclosureNotAwaitingAuthorization:
		return "NOT_AWAITING_AUTHORIZATION"
	case DisclosureConditionNoLongerMet:
		return "DISCLOSURE_CONDITION_NO_LONGER_MET"
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

// DecideDisclosureCommand 指名要对哪个信号形成披露决定：对象与信号类型取这一类最近的那一期
// （SignalEpisodeStore.FindLatest，不问因果条件）。命令里没有客户账户——客户归属由 parcel-shipment 的当前
// 已接受委托给出，本上下文只按包裹反查（CONTEXT「客户归属」），调用方不替它断言。
// 也没有披露结论——结论由规则与用例形成，命令带结论就是让调用方替规则作判断。租户
// 显式随命令到达（ADR-0003）。
type DecideDisclosureCommand struct {
	TenantID domain.TenantID
	Parcel   domain.TrackedParcelReference
	Kind     domain.ExceptionSignalKindReference
	// Episode 指名判哪一期。运营决定口必给：人判的是他看过的那一期。留空取这一类最近的那一期，给不经人的入口用。
	Episode domain.EpisodeID
	// Operator 是经运营决定口发起这次判断的操作者，只取认证结果。不经运营口的入口留空，那一行就没有决定人。
	Operator string
}

// AuthorizeDisclosureCommand 指名要确认发布哪条信号上那份停在`待授权`的决定。命令里没有披露内容：内容取确认这一刻
// 适用的披露规则，确认的人只放行、不另写一份给客户看的话。
type AuthorizeDisclosureCommand struct {
	TenantID domain.TenantID
	Parcel   domain.TrackedParcelReference
	Kind     domain.ExceptionSignalKindReference
	// Episode 必给：同一件包裹同一类信号可以同时有几期，确认的是确认人看过的那一期，不是此刻最近的那一期。
	Episode  domain.EpisodeID
	Operator string
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
	Episodes ports.SignalEpisodeStore
	// EpisodeByID 按标识找发作期。命令指名了哪一期而它不在是装配缺陷。
	EpisodeByID ports.SignalEpisodeLookup
	Customers   ports.ParcelCustomerAccountView
	Rules       ports.ExceptionDisclosureRuleView
	Decisions   ports.DisclosureDecisionStore
	// Ledger 记决定上由人作出的那一半。只有运营决定口后面的装配给它；命令带了操作者而它不在是装配缺陷。
	Ledger ports.DisclosureDecisionLedger
	Clock  ports.Clock
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

	if command.Operator != "" && command.Episode.String() == "" {
		return DecideDisclosureResult{outcome: DecideDisclosureNotAccepted}, nil
	}
	episode, found, err := handler.findEpisode(ctx, command.TenantID, command.Parcel, command.Kind, command.Episode)
	if err != nil {
		return DecideDisclosureResult{}, err
	}
	if episode == nil && found {
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
		if command.Operator != "" {
			if handler.deps.Ledger == nil {
				return DecideDisclosureResult{}, fmt.Errorf("decide disclosure: an operator decided but no ledger is wired")
			}
			if err := handler.deps.Ledger.RecordDecider(ctx, command.TenantID, decision, command.Operator); err != nil {
				return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureDecisionStoreUnavailable}, nil
			}
		}
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

// Authorize 是 `UC-VE-006` 步 5 的人工那一半：授权角色确认发布一份停在`待授权`的决定。找最近发作期与客户归属同
// Handle；当前决定已是`披露`即交回原决定（重放或另一个人先确认了）；不是`待授权`就没有可确认的；披露规则重取——
// 未配置即未决，规则此刻说不披露即如实答条件已不成立；内容快照与规则引用取此刻的规则。确认形成新的一份决定，原
// `待授权`那一份不改。
func (handler *DecideDisclosureHandler) Authorize(
	ctx context.Context,
	command AuthorizeDisclosureCommand,
) (DecideDisclosureResult, error) {
	if command.TenantID.String() == "" ||
		command.Parcel.String() == "" ||
		command.Kind.String() == "" ||
		command.Episode.String() == "" ||
		command.Operator == "" {
		return DecideDisclosureResult{outcome: DecideDisclosureNotAccepted}, nil
	}
	if handler.deps.Ledger == nil {
		return DecideDisclosureResult{}, fmt.Errorf("authorize disclosure: no ledger is wired")
	}

	episode, found, err := handler.findEpisode(ctx, command.TenantID, command.Parcel, command.Kind, command.Episode)
	if err != nil {
		return DecideDisclosureResult{}, err
	}
	if episode == nil && found {
		return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureEpisodeStoreUnavailable}, nil
	}
	if !found {
		return DecideDisclosureResult{outcome: DecideDisclosureNotAccepted}, nil
	}
	customer, determinable, err := handler.deps.Customers.FindCustomerAccount(ctx, command.TenantID, command.Parcel)
	if err != nil {
		return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureCustomerAccountUnavailable}, nil
	}
	if !determinable {
		return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureCustomerAccountUndeterminable}, nil
	}

	current, found, err := handler.deps.Decisions.FindCurrent(ctx, command.TenantID, episode.ID(), customer)
	if err != nil {
		return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureDecisionStoreUnavailable}, nil
	}
	if !found {
		return DecideDisclosureResult{outcome: DisclosureNotAwaitingAuthorization}, nil
	}
	switch current.Conclusion() {
	case domain.DiscloseToCustomer:
		return DecideDisclosureResult{outcome: DisclosureExistingResult, decision: current, hasDecision: true}, nil
	case domain.AwaitingAuthorization:
	default:
		return DecideDisclosureResult{outcome: DisclosureNotAwaitingAuthorization, decision: current, hasDecision: true}, nil
	}

	snapshot := episode.Snapshot()
	rule, configured, err := handler.deps.Rules.RuleForSignal(ctx, customer, snapshot.Kind, snapshot.Confidence)
	if err != nil {
		return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureRuleUnavailable}, nil
	}
	if !configured {
		return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureRuleNotConfigured}, nil
	}
	if !rule.Disclosable {
		return DecideDisclosureResult{outcome: DisclosureConditionNoLongerMet, decision: current, hasDecision: true}, nil
	}

	authorized, err := domain.AuthorizeDisclosure(current, rule.Policy, rule.Content, handler.deps.Clock.Now())
	if err != nil {
		// 规则说披露却没给内容快照，或时钟不晚于被确认的那一份——都是依赖坏答复，上抛而不吞成某一态。
		return DecideDisclosureResult{}, fmt.Errorf("authorize disclosure: %w", err)
	}
	saved, err := handler.deps.Ledger.SaveAuthorization(ctx, ports.DisclosureAuthorization{
		Tenant: command.TenantID, Decision: authorized, Authorizes: current.DecidedAt(), Operator: command.Operator,
	})
	if err != nil {
		return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureDecisionStoreUnavailable}, nil
	}
	switch saved {
	case ports.DisclosureDecisionSaved:
		return DecideDisclosureResult{outcome: DisclosureAuthorized, decision: authorized, hasDecision: true}, nil
	case ports.DisclosureDecisionAlreadyRecorded:
		// 另一个人先确认了：读回赢家。读回的不是`披露`说明库里的状态与刚才那次读对不上，不猜。
		winner, found, err := handler.deps.Decisions.FindCurrent(ctx, command.TenantID, episode.ID(), customer)
		if err != nil || !found || winner.Conclusion() != domain.DiscloseToCustomer {
			return DecideDisclosureResult{outcome: DecideDisclosureUndecided, reason: DisclosureDecisionStoreUnavailable}, nil
		}
		return DecideDisclosureResult{outcome: DisclosureExistingResult, decision: winner, hasDecision: true}, nil
	default:
		return DecideDisclosureResult{}, fmt.Errorf("authorize disclosure: unexpected save outcome %d", saved)
	}
}

// findEpisode 找命令指名的那一期；没指名就取这一类最近的那一期。三种答复：找到（期，true，nil）；没有（nil，false，
// nil）；发作期册读不动（nil，true，nil）——调用方据此答未决。指名了哪一期却没接按标识找的那一只是装配缺陷，上抛。
func (handler *DecideDisclosureHandler) findEpisode(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
	kind domain.ExceptionSignalKindReference,
	named domain.EpisodeID,
) (*domain.SignalEpisode, bool, error) {
	var (
		episode *domain.SignalEpisode
		found   bool
		err     error
	)
	if named.String() == "" {
		episode, found, err = handler.deps.Episodes.FindLatest(ctx, tenant, parcel, kind)
	} else {
		if handler.deps.EpisodeByID == nil {
			return nil, false, fmt.Errorf("decide disclosure: an episode is named but no episode lookup is wired")
		}
		episode, found, err = handler.deps.EpisodeByID.FindEpisode(ctx, tenant, parcel, kind, named)
	}
	if err != nil {
		return nil, true, nil
	}
	if !found || episode == nil {
		return nil, false, nil
	}
	return episode, true, nil
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

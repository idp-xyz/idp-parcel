package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var disclosureEpisodeAt = time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)

type customerAccountDouble struct {
	account domain.CustomerAccountReference
	found   bool
	err     error
	calls   int
}

func (double *customerAccountDouble) FindCustomerAccount(
	_ context.Context,
	_ domain.TenantID,
	_ domain.TrackedParcelReference,
) (domain.CustomerAccountReference, bool, error) {
	double.calls++
	if double.err != nil {
		return domain.CustomerAccountReference{}, false, double.err
	}
	return double.account, double.found, nil
}

type disclosureRuleDouble struct {
	rule       ports.ExceptionDisclosureRule
	configured bool
	err        error
	calls      int
	lastKind   domain.ExceptionSignalKindReference
	lastConf   domain.ConfidenceReference
}

func (double *disclosureRuleDouble) RuleForSignal(
	_ context.Context,
	_ domain.CustomerAccountReference,
	kind domain.ExceptionSignalKindReference,
	confidence domain.ConfidenceReference,
) (ports.ExceptionDisclosureRule, bool, error) {
	double.calls++
	double.lastKind, double.lastConf = kind, confidence
	if double.err != nil {
		return ports.ExceptionDisclosureRule{}, false, double.err
	}
	return double.rule, double.configured, nil
}

type decisionKey struct {
	tenant   domain.TenantID
	episode  domain.EpisodeID
	customer domain.CustomerAccountReference
}

type decisionStoreDouble struct {
	current map[decisionKey]domain.DisclosureDecision
	findErr error
	saveErr error
	saved   int
}

func newDecisionStore() *decisionStoreDouble {
	return &decisionStoreDouble{current: map[decisionKey]domain.DisclosureDecision{}}
}

func (double *decisionStoreDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	episode domain.EpisodeID,
	customer domain.CustomerAccountReference,
) (domain.DisclosureDecision, bool, error) {
	if double.findErr != nil {
		return domain.DisclosureDecision{}, false, double.findErr
	}
	decision, found := double.current[decisionKey{tenant: tenant, episode: episode, customer: customer}]
	return decision, found, nil
}

func (double *decisionStoreDouble) Save(
	_ context.Context,
	tenant domain.TenantID,
	decision domain.DisclosureDecision,
) (ports.DisclosureDecisionSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.DisclosureDecisionSaveOutcomeInvalid, double.saveErr
	}
	key := decisionKey{tenant: tenant, episode: decision.Episode(), customer: decision.Customer()}
	if existing, found := double.current[key]; found && existing.DecidedAt().Equal(decision.DecidedAt()) {
		return ports.DisclosureDecisionAlreadyRecorded, nil
	}
	double.current[key] = decision
	double.saved++
	return ports.DisclosureDecisionSaved, nil
}

type decideFixture struct {
	handler   *application.DecideDisclosureHandler
	episodes  *episodeStoreDouble
	customers *customerAccountDouble
	rules     *disclosureRuleDouble
	decisions *decisionStoreDouble
}

// newDecideFixture 预置一个活跃发作期（parcel-1 / ETA_BREACH_RISK）、可反查的客户账户与一条
// 「披露且批准自动发布」的规则——用例各自往反方向拧。
func newDecideFixture(t *testing.T) *decideFixture {
	t.Helper()
	episodes := newEpisodeStore()
	episode, err := domain.OpenEpisode(
		mustValue(t, domain.NewEpisodeID, "episode-1"),
		mustValue(t, domain.NewExceptionSignalKindReference, "ETA_BREACH_RISK"),
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		mustValue(t, domain.NewSignalRuleVersionReference, "signal-rules/v3"),
		mustValue(t, domain.NewConfidenceReference, "HIGH/route-deviation"),
		disclosureEpisodeAt,
	)
	if err != nil {
		t.Fatalf("open episode: %v", err)
	}
	episodes.latest[episodeKey{
		tenant: mustValue(t, domain.NewTenantID, "tenant-1"),
		parcel: episode.Snapshot().Parcel,
		kind:   episode.Snapshot().Kind,
	}] = episode

	fixture := &decideFixture{
		episodes: episodes,
		customers: &customerAccountDouble{
			account: mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
			found:   true,
		},
		rules: &disclosureRuleDouble{
			rule: ports.ExceptionDisclosureRule{
				Policy:      mustValue(t, domain.NewDisclosurePolicyReference, "exception-disclosure/v1"),
				Disclosable: true,
				AutoRelease: true,
				Content:     mustValue(t, domain.NewDisclosureContentReference, "disclosure-content/eta-breach/v1"),
			},
			configured: true,
		},
		decisions: newDecisionStore(),
	}
	fixture.handler = application.NewDecideDisclosureHandler(application.DecideDisclosureDeps{
		Episodes:  fixture.episodes,
		Customers: fixture.customers,
		Rules:     fixture.rules,
		Decisions: fixture.decisions,
		Clock:     fixedClock{at: disclosureEpisodeAt.Add(time.Hour)},
	})
	return fixture
}

func decideCommand(t *testing.T) application.DecideDisclosureCommand {
	t.Helper()
	return application.DecideDisclosureCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Parcel:   mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Kind:     mustValue(t, domain.NewExceptionSignalKindReference, "ETA_BREACH_RISK"),
	}
}

// Covers: `AT-VE-102`「批准范围允许自动发布→保存自动决定依据，只发布批准范围」与 CONTEXT
// 「客户可见性必须根据服务产品、客户合同、信号可信度……和信息披露规则形成版本化决定」
// ——规则说披露且批准自动发布时形成`披露`决定：发作期、反查到的客户账户、规则引用与
// 内容快照全部来自输入侧，决定落库；规则按信号的类型与可信度查。
func TestADisclosableAutoReleaseRuleFormsADiscloseDecision(t *testing.T) {
	fixture := newDecideFixture(t)

	result, err := fixture.handler.Handle(context.Background(), decideCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.DisclosureDecided {
		t.Fatalf("outcome = %q, want DECIDED", result.Outcome())
	}
	decision, present := result.Decision()
	if !present {
		t.Fatal("a decided result carries no decision")
	}
	if decision.Conclusion() != domain.DiscloseToCustomer {
		t.Fatalf("conclusion = %s, want DISCLOSE", decision.Conclusion())
	}
	content, disclosed := decision.Content()
	if !disclosed || content.String() != "disclosure-content/eta-breach/v1" {
		t.Fatal("a disclose decision must carry the rule's content snapshot reference")
	}
	if decision.Episode().String() != "episode-1" ||
		decision.Customer().String() != "customer-1" ||
		decision.Policy().String() != "exception-disclosure/v1" {
		t.Fatalf("decision = %+v; episode, customer and policy must come from the episode, the account lookup and the rule", decision)
	}
	if fixture.rules.lastKind.String() != "ETA_BREACH_RISK" || fixture.rules.lastConf.String() != "HIGH/route-deviation" {
		t.Fatal("the rule must be looked up by the episode's signal kind and confidence")
	}
	if fixture.decisions.saved != 1 {
		t.Fatalf("saved %d decisions, want exactly 1", fixture.decisions.saved)
	}
}

// Covers: `AT-VE-099`「案件存在但合同不要求披露→形成暂不披露，不自动发消息」——规则说
// 不披露时决定落`暂不披露`且不带内容；它是一份已作出、已登记的决定，不是没有决定。
func TestANonDisclosableRuleFormsANotYetDisclosableDecision(t *testing.T) {
	fixture := newDecideFixture(t)
	fixture.rules.rule = ports.ExceptionDisclosureRule{
		Policy:      mustValue(t, domain.NewDisclosurePolicyReference, "exception-disclosure/v1"),
		Disclosable: false,
	}

	result, err := fixture.handler.Handle(context.Background(), decideCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	decision, present := result.Decision()
	if result.Outcome() != application.DisclosureDecided || !present {
		t.Fatalf("outcome = %q; a not-yet-disclosable result is still a decision", result.Outcome())
	}
	if decision.Conclusion() != domain.NotYetDisclosable {
		t.Fatalf("conclusion = %s, want NOT_YET_DISCLOSABLE", decision.Conclusion())
	}
	if _, disclosed := decision.Content(); disclosed {
		t.Fatal("a not-yet-disclosable decision carried content")
	}
	if fixture.decisions.saved != 1 {
		t.Fatal("the not-yet-disclosable decision was not recorded")
	}
}

// Covers: `AT-VE-100`「客户影响明确但需要授权→形成草稿和待授权，不提交渠道」与 `UC-VE-006`
// 步 5「默认等待授权角色确认；只有批准范围才形成提交意图或自动发布决定」——规则说披露但
// 未批准自动发布时落`待授权`：不带内容快照（内容随授权那一步的决定版本走），不是`披露`。
func TestADisclosableRuleWithoutAutoReleaseAwaitsAuthorization(t *testing.T) {
	fixture := newDecideFixture(t)
	fixture.rules.rule.AutoRelease = false

	result, err := fixture.handler.Handle(context.Background(), decideCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	decision, present := result.Decision()
	if result.Outcome() != application.DisclosureDecided || !present {
		t.Fatalf("outcome = %q; awaiting authorization is still a decision", result.Outcome())
	}
	if decision.Conclusion() != domain.AwaitingAuthorization {
		t.Fatalf("conclusion = %s, want AWAITING_AUTHORIZATION; the default is human confirmation", decision.Conclusion())
	}
	if _, disclosed := decision.Content(); disclosed {
		t.Fatal("an awaiting-authorization decision carried content as if it were disclosed")
	}
}

// Covers: 派工约束「披露规则是实例参数（PAR-VIS-07）：未配置 → 未决不虚构可见性」——
// 没有规则时既不能说披露也不能说不披露，停在未决且不写库。
func TestAnUnconfiguredDisclosureRuleIsUndecidedWithoutInventingADecision(t *testing.T) {
	fixture := newDecideFixture(t)
	fixture.rules.configured = false

	result, err := fixture.handler.Handle(context.Background(), decideCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.DecideDisclosureUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.UndecidedReason() != application.DisclosureRuleNotConfigured {
		t.Fatalf("reason = %q, want DISCLOSURE_RULE_NOT_CONFIGURED", result.UndecidedReason())
	}
	if fixture.decisions.saved != 0 {
		t.Fatal("an unconfigured rule still produced a recorded decision")
	}
	if application.DisclosureRuleNotConfigured == application.DisclosureRuleUnavailable {
		t.Fatal("not-configured and unavailable share one reason; their recovery actions diverge")
	}
}

// Covers: CONTEXT「客户全程追踪视图以客户归属可确定为前提……归属不可确定期间不形成视图，
// 也不得发明账户」在披露决定上的同一条——反查不到当前已接受委托时停在未决，不查规则、
// 不写库；反查调不通是依赖故障，另占一格。
func TestAnUndeterminableCustomerAccountIsUndecidedWithoutInventingOne(t *testing.T) {
	fixture := newDecideFixture(t)
	fixture.customers.found = false

	result, err := fixture.handler.Handle(context.Background(), decideCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.DecideDisclosureUndecided ||
		result.UndecidedReason() != application.DisclosureCustomerAccountUndeterminable {
		t.Fatalf("outcome/reason = %q/%q, want UNDECIDED/DISCLOSURE_CUSTOMER_ACCOUNT_UNDETERMINABLE",
			result.Outcome(), result.UndecidedReason())
	}
	if fixture.rules.calls != 0 || fixture.decisions.saved != 0 {
		t.Fatal("an undeterminable account still reached the rule or the store")
	}

	fixture.customers.found = true
	fixture.customers.err = errors.New("account view unavailable")
	unavailable, err := fixture.handler.Handle(context.Background(), decideCommand(t))
	if err != nil {
		t.Fatalf("handle unavailable: %v", err)
	}
	if unavailable.UndecidedReason() != application.DisclosureCustomerAccountUnavailable {
		t.Fatalf("reason = %q, want DISCLOSURE_CUSTOMER_ACCOUNT_UNAVAILABLE", unavailable.UndecidedReason())
	}
}

// Covers: 幂等——同一发作期对同一客户已经决定过，重放返回原决定、不重判、不问规则、不再
// 写库。待授权转披露归授权那一步，不由重跑本用例完成。
func TestTheSameEpisodeAndCustomerAreNotDecidedTwice(t *testing.T) {
	fixture := newDecideFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, decideCommand(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstDecision, _ := first.Decision()
	rulesBefore := fixture.rules.calls

	replay, err := fixture.handler.Handle(ctx, decideCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}

	if replay.Outcome() != application.DisclosureExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT", replay.Outcome())
	}
	replayed, present := replay.Decision()
	if !present || !replayed.DecidedAt().Equal(firstDecision.DecidedAt()) ||
		replayed.Conclusion() != firstDecision.Conclusion() {
		t.Fatal("the replay did not return the decision that already exists")
	}
	if fixture.rules.calls != rulesBefore || fixture.decisions.saved != 1 {
		t.Fatal("a replay re-read the rule or recorded a second decision")
	}
}

// Covers: 受理半边——没有这一类的信号发作期就没有可披露的客户可见异常（异常案件存在都不
// 自动要求披露，何况连信号都没有）；对象或类型缺一同样未受理，不读任何依赖。
func TestWithoutAnEpisodeThereIsNothingToDisclose(t *testing.T) {
	fixture := newDecideFixture(t)

	unknownKind := decideCommand(t)
	unknownKind.Kind = mustValue(t, domain.NewExceptionSignalKindReference, "NEVER_RAISED")
	result, err := fixture.handler.Handle(context.Background(), unknownKind)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.DecideDisclosureNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED", result.Outcome())
	}
	if fixture.customers.calls != 0 || fixture.rules.calls != 0 {
		t.Fatal("a signal-less command still reached the account view or the rule")
	}

	missingParcel := decideCommand(t)
	missingParcel.Parcel = domain.TrackedParcelReference{}
	noParcel, err := fixture.handler.Handle(context.Background(), missingParcel)
	if err != nil {
		t.Fatalf("handle without parcel: %v", err)
	}
	if noParcel.Outcome() != application.DecideDisclosureNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED", noParcel.Outcome())
	}
}

// Covers: 端口封闭答复纪律——规则说披露却没给内容快照引用，是登记面漏了一格的端口坏
// 答复（领域构造门在场拦住）：上抛而不吞成某一态，也不写库。
func TestADisclosableRuleWithoutContentIsRaisedAsAPortDefect(t *testing.T) {
	fixture := newDecideFixture(t)
	fixture.rules.rule.Content = domain.DisclosureContentReference{}

	_, err := fixture.handler.Handle(context.Background(), decideCommand(t))
	if err == nil {
		t.Fatal("a disclosable rule without content was swallowed instead of raised")
	}
	if !errors.Is(err, domain.ErrInvalidDisclosure) {
		t.Fatalf("err = %v, want ErrInvalidDisclosure from the domain door", err)
	}
	if fixture.decisions.saved != 0 {
		t.Fatal("a defective rule still recorded a decision")
	}
}

// Covers: 未决语义的存储半边——决定库读不回时停在未决，不问规则；写不进时同样未决，重试
// 会重判同一份。
func TestAnUnreadableDecisionStoreIsUndecidedBeforeTheRule(t *testing.T) {
	fixture := newDecideFixture(t)
	fixture.decisions.findErr = errors.New("decision store unavailable")

	result, err := fixture.handler.Handle(context.Background(), decideCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.DecideDisclosureUndecided ||
		result.UndecidedReason() != application.DisclosureDecisionStoreUnavailable {
		t.Fatalf("outcome/reason = %q/%q, want UNDECIDED/DISCLOSURE_DECISION_STORE_UNAVAILABLE",
			result.Outcome(), result.UndecidedReason())
	}
	if fixture.rules.calls != 0 {
		t.Fatal("the round asked the rule before it could answer idempotently")
	}
}

package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 两个时点刻意取不同的值。取成同一个，「没有压成全局时间」这条就无从断言——两项判断共用一个
// 时刻时，正确实现与错误实现给出的结果一模一样。
var (
	reachabilityAsOfAt = time.Date(2026, 6, 2, 8, 0, 0, 0, time.UTC)
	financialAsOfAt    = time.Date(2026, 6, 2, 10, 15, 0, 0, time.UTC)
)

// asOfPolicyDouble 记录它被问到的租户与规则包：政策按租户隔离，而「问的是第一阶段选出的那个
// 规则包」只有在端口实际收到的参数上才验得出来。
type asOfPolicyDouble struct {
	policies     []domain.AsOfPolicy
	err          error
	askedTenant  domain.TenantID
	askedPackage domain.CommercialVersion
	loadCalled   int
}

func (double *asOfPolicyDouble) LoadAsOfPolicies(
	_ context.Context,
	tenant domain.TenantID,
	rulePackage domain.CommercialVersion,
) ([]domain.AsOfPolicy, error) {
	double.loadCalled++
	double.askedTenant = tenant
	double.askedPackage = rulePackage
	if double.err != nil {
		return nil, double.err
	}
	return double.policies, nil
}

func asOfPolicy(t *testing.T, judgment domain.JudgmentType, semantics, policyVersion string) domain.AsOfPolicy {
	t.Helper()
	policy, err := domain.NewAsOfPolicy(
		judgment,
		value(t, domain.NewAsOfSemanticsReference, semantics),
		value(t, domain.NewAsOfPolicyVersion, policyVersion),
	)
	if err != nil {
		t.Fatalf("new as-of policy: %v", err)
	}
	return policy
}

// resolvedWithRulePackage 走完第一阶段并选出接单规则包——第二阶段的输入是它，不是调用方另给
// 的一个包，否则规则包就成了自己被选中的理由。
func resolvedWithRulePackage(t *testing.T) domain.CommercialClosure {
	t.Helper()
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	effectiveIn(t, registry, domain.AcceptanceRulePackageObject, "rules-1", "v1", "sha256:r1", "scope-a")

	resolved, err := application.NewResolveCommercialBasisHandler(&authorityDouble{registry: registry}, fixedClock{at: judgedAt}).
		Handle(context.Background(), application.ResolveCommercialBasisCommand{
			Key: closureKey(t, "scope-a", domain.CustomerContractObject, domain.AcceptanceRulePackageObject),
		})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Closure().Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want the first phase to succeed", resolved.Closure().Outcome())
	}
	return resolved.Closure()
}

// resolutionStoreDouble 冒充「按解析标识取回已固定解析」那份责任（ADR-0027）。后续阶段只回指
// 标识，闭包由本上下文取回，因此夹具也得从这里给。
type resolutionStoreDouble struct {
	closure domain.CommercialClosure
	found   bool
	err     error
	asked   []domain.ResolutionID
}

func (double *resolutionStoreDouble) LoadResolution(
	_ context.Context,
	_ domain.TenantID,
	resolution domain.ResolutionID,
) (domain.CommercialClosure, bool, error) {
	double.asked = append(double.asked, resolution)
	if double.err != nil {
		return domain.CommercialClosure{}, false, double.err
	}
	return double.closure, double.found, nil
}

// storedResolution 把一份第一阶段结果放进取回端口，并交回指名它所需的调用方身份与标识。
// 身份取自解析键本身——调用方必须是这份解析的主人，否则取回那一步会判`输入未受理`。
func storedResolution(
	t *testing.T,
	prior domain.CommercialClosure,
) (*resolutionStoreDouble, application.CallerScope, domain.ResolutionID) {
	t.Helper()
	key := prior.ResolutionKey()
	return &resolutionStoreDouble{closure: prior, found: true},
		application.CallerScope{TenantID: key.TenantID, CustomerAccountID: key.CustomerAccountID},
		prior.ResolutionID()
}

// Covers: UC-PC-002 `AT-PC-023`「规则包选出后为网络与财务声明不同 `asOf` → 分别形成并校验，不压
// 成全局时间」与步骤 6「不使用一个全局时间代替」。
//
// 每项判断带回自己的语义引用与政策版本，权威提供方据以校验回显。压成一个全局时间，可达性会按财
// 务控制的时点判、或者反过来，而两者锚定的本就是不同的业务时刻。
func TestEachJudgmentFormsItsOwnAsOfRatherThanSharingOneGlobalTime(t *testing.T) {
	policies := &asOfPolicyDouble{policies: []domain.AsOfPolicy{
		asOfPolicy(t, domain.NetworkReachabilityJudgment, "SEMANTICS-ROUTE-EVALUATED-AT", "asof-policy-v1"),
		asOfPolicy(t, domain.PreAcceptanceFinancialControlJudgment, "SEMANTICS-CONTROL-APPLIED-AT", "asof-policy-v1"),
	}}

	store, caller, resolution := storedResolution(t, resolvedWithRulePackage(t))
	result, err := application.NewFormJudgmentAsOfHandler(store, policies).
		Handle(context.Background(), application.FormJudgmentAsOfCommand{
			Caller:     caller,
			Resolution: resolution,
			Judgments: []application.JudgmentAsOfRequest{
				{Judgment: domain.NetworkReachabilityJudgment, At: reachabilityAsOfAt},
				{Judgment: domain.PreAcceptanceFinancialControlJudgment, At: financialAsOfAt},
			},
		})
	if err != nil {
		t.Fatalf("form as-of: %v", err)
	}

	if result.Outcome() != application.JudgmentAsOfFormed {
		t.Fatalf("outcome = %q, want FORMED", result.Outcome())
	}
	reachability, formed := result.AsOfFor(domain.NetworkReachabilityJudgment)
	if !formed {
		t.Fatal("network reachability got no as-of although the rule package declared one")
	}
	financial, formed := result.AsOfFor(domain.PreAcceptanceFinancialControlJudgment)
	if !formed {
		t.Fatal("pre-acceptance financial control got no as-of although the rule package declared one")
	}

	if !reachability.At().Equal(reachabilityAsOfAt) || !financial.At().Equal(financialAsOfAt) {
		t.Fatalf(
			"reachability at %s and financial at %s; each judgment keeps the value its consumer formed",
			reachability.At(), financial.At(),
		)
	}
	if reachability.At().Equal(financial.At()) {
		t.Fatal("两项判断压成了同一个时点，而规则包为它们声明的是不同的时点锚")
	}
	if reachability.Policy().Semantics() == financial.Policy().Semantics() {
		t.Fatal("两项判断带回同一个语义引用，权威提供方据以校验回显时就分不出该按哪条政策核")
	}
}

// Covers: UC-PC-002 第二阶段失败边界「未配置」与 `AT-PC-018`「商业选择锚点策略未配置 → 解析未
// 决，不选系统当前时间默认」在时点政策一侧。
//
// 规则包没有为这项判断声明时点锚时，既不借用另一项判断的政策，也不拿此刻顶替。首发没有租户，
// 因此这条就是当前唯一走得到的真实分支——它必须停下，而不是给出一个能跑通的默认值。
func TestAJudgmentWithNoDeclaredPolicyStopsInsteadOfDefaultingToNow(t *testing.T) {
	policies := &asOfPolicyDouble{policies: []domain.AsOfPolicy{
		asOfPolicy(t, domain.NetworkReachabilityJudgment, "SEMANTICS-ROUTE-EVALUATED-AT", "asof-policy-v1"),
	}}

	store, caller, resolution := storedResolution(t, resolvedWithRulePackage(t))
	result, err := application.NewFormJudgmentAsOfHandler(store, policies).
		Handle(context.Background(), application.FormJudgmentAsOfCommand{
			Caller:     caller,
			Resolution: resolution,
			// 一项已声明、一项未声明：两项都请求，「全有或全无」才验得出来。只请求未声明的
			// 那一项时，另一项本来就不会形成，断言永远成立而咬不住任何东西。
			Judgments: []application.JudgmentAsOfRequest{
				{Judgment: domain.NetworkReachabilityJudgment, At: reachabilityAsOfAt},
				{Judgment: domain.PreAcceptanceFinancialControlJudgment, At: financialAsOfAt},
			},
		})
	if err != nil {
		t.Fatalf("form as-of: %v", err)
	}

	if result.Outcome() != application.JudgmentAsOfNotConfigured {
		t.Fatalf("outcome = %q, want NOT_CONFIGURED; an undeclared anchor must never fall back to a usable time", result.Outcome())
	}
	if _, formed := result.AsOfFor(domain.PreAcceptanceFinancialControlJudgment); formed {
		t.Fatal("形成了一个规则包并未声明的时点锚")
	}
	// 已声明的那一项也不放行：逐项部分成功会让调用方拿着半套时点去推进判断，而缺的那一项要等
	// 的是实例参数落地，不是重试。
	if _, formed := result.AsOfFor(domain.NetworkReachabilityJudgment); formed {
		t.Fatal("一项未配置却仍交回了另一项，调用方会据以只推进半边判断")
	}
}

// Covers: UC-PC-002 第二阶段失败边界「消费方无法形成值」——零值时点不得被读成「此刻」。
func TestAZeroAsOfValueIsRefusedRatherThanReadAsNow(t *testing.T) {
	policies := &asOfPolicyDouble{policies: []domain.AsOfPolicy{
		asOfPolicy(t, domain.NetworkReachabilityJudgment, "SEMANTICS-ROUTE-EVALUATED-AT", "asof-policy-v1"),
	}}

	store, caller, resolution := storedResolution(t, resolvedWithRulePackage(t))
	result, err := application.NewFormJudgmentAsOfHandler(store, policies).
		Handle(context.Background(), application.FormJudgmentAsOfCommand{
			Caller:     caller,
			Resolution: resolution,
			Judgments: []application.JudgmentAsOfRequest{
				{Judgment: domain.NetworkReachabilityJudgment},
			},
		})
	if err != nil {
		t.Fatalf("form as-of: %v", err)
	}

	if result.Outcome() != application.JudgmentAsOfValueInvalid {
		t.Fatalf("outcome = %q, want VALUE_INVALID", result.Outcome())
	}
}

// Covers: UC-PC-002「依赖调用超时……只能形成解析未决」在时点政策一侧——读不回与「规则包没声明」
// 是两回事：前者等重试，后者等 `PAR-COM-14` 落地。合成一格，调用方就不知道该催人还是该重试。
func TestUnreadableAsOfPoliciesArePendingRatherThanNotConfigured(t *testing.T) {
	policies := &asOfPolicyDouble{err: errors.New("as-of policy declaration unavailable")}

	store, caller, resolution := storedResolution(t, resolvedWithRulePackage(t))
	result, err := application.NewFormJudgmentAsOfHandler(store, policies).
		Handle(context.Background(), application.FormJudgmentAsOfCommand{
			Caller:     caller,
			Resolution: resolution,
			Judgments: []application.JudgmentAsOfRequest{
				{Judgment: domain.NetworkReachabilityJudgment, At: reachabilityAsOfAt},
			},
		})
	if err != nil {
		t.Fatalf("读取失败被当成技术错误抛出，而用例要求它形成未决: %v", err)
	}

	if result.Outcome() != application.JudgmentAsOfPending {
		t.Fatalf("outcome = %q, want PENDING; an unreadable declaration is not the same as an undeclared one", result.Outcome())
	}
}

// Covers: UC-PC-002 步骤 6「依据已选规则包」与两阶段契约「第二阶段才由已选规则包声明各下游判断
// 的 `asOf` 策略」。
//
// 问的必须是第一阶段选出的那个规则包，且带上租户。调用方另给一个包，规则包就成了自己被选中的
// 理由，而两阶段机制存在的全部意义正是避免这一点。
func TestTheDeclarationIsAskedForTheRulePackageTheFirstPhaseSelected(t *testing.T) {
	policies := &asOfPolicyDouble{policies: []domain.AsOfPolicy{
		asOfPolicy(t, domain.NetworkReachabilityJudgment, "SEMANTICS-ROUTE-EVALUATED-AT", "asof-policy-v1"),
	}}
	prior := resolvedWithRulePackage(t)
	adopted, present := prior.AdoptedFor(domain.AcceptanceRulePackageObject)
	if !present {
		t.Fatal("the first phase adopted no acceptance rule package")
	}

	store, caller, resolution := storedResolution(t, prior)
	if _, err := application.NewFormJudgmentAsOfHandler(store, policies).
		Handle(context.Background(), application.FormJudgmentAsOfCommand{
			Caller:     caller,
			Resolution: resolution,
			Judgments:  []application.JudgmentAsOfRequest{{Judgment: domain.NetworkReachabilityJudgment, At: reachabilityAsOfAt}},
		}); err != nil {
		t.Fatalf("form as-of: %v", err)
	}

	// 取回那一步问的必须是调用方指名的标识，不是别的：端口只回指标识，问错一个就等于换了
	// 一份依据。
	if len(store.asked) != 1 || store.asked[0] != resolution {
		t.Fatalf("asked %v, want exactly the named resolution %q", store.asked, resolution)
	}

	if policies.loadCalled != 1 {
		t.Fatalf("declaration loaded %d times, want exactly one per second phase", policies.loadCalled)
	}
	if policies.askedTenant != prior.ResolutionKey().TenantID {
		t.Fatalf("asked tenant = %q, want %q", policies.askedTenant, prior.ResolutionKey().TenantID)
	}
	if policies.askedPackage != adopted.Version() {
		t.Fatal("问的不是第一阶段选出的那个规则包")
	}
}

// Covers: 第二阶段的前提——第一阶段必须已经唯一解出，以及 ADR-0027「后续阶段只回指解析标识」。
//
// 按 ADR-0027 改成只回指标识之后，未唯一解出的第一阶段连一个能指名它的引用都不会有：适用冲突
// 与解析未决都不固定解析标识。所以「拿一份未唯一的闭包进第二阶段」这条路已经由构造关闭，剩下
// 的等价缺口是**指名一个取不回来的标识**。
//
// 它必须落在`依据未解析`——那要调用方回第一阶段——而不是去问政策：替一个尚未选出的规则包声明
// 时点锚，调用方拿到时点锚会以为商业依据已经定了。
func TestASecondPhaseOnAnUnknownResolutionRefusesWithoutAskingForPolicies(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-2", "v1", "sha256:c2", "scope-a")

	conflicted, err := application.NewResolveCommercialBasisHandler(&authorityDouble{registry: registry}, fixedClock{at: judgedAt}).
		Handle(context.Background(), application.ResolveCommercialBasisCommand{
			Key: closureKey(t, "scope-a", domain.CustomerContractObject),
		})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if conflicted.Closure().Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT as the precondition", conflicted.Closure().Outcome())
	}
	if conflicted.Closure().ResolutionID().String() != "" {
		t.Fatal("适用冲突固定了解析标识——调用方会据它进第二阶段，而那里没有已选规则包")
	}

	// 拿一份唯一解出的解析取得合法身份，再指名一个取不回来的标识：身份合法，标识不存在。
	policies := &asOfPolicyDouble{}
	_, caller, _ := storedResolution(t, resolvedWithRulePackage(t))
	known, _, resolution := storedResolution(t, resolvedWithRulePackage(t))
	known.found = false

	result, err := application.NewFormJudgmentAsOfHandler(known, policies).
		Handle(context.Background(), application.FormJudgmentAsOfCommand{
			Caller:     caller,
			Resolution: resolution,
			Judgments:  []application.JudgmentAsOfRequest{{Judgment: domain.NetworkReachabilityJudgment, At: reachabilityAsOfAt}},
		})
	if err != nil {
		t.Fatalf("form as-of: %v", err)
	}

	if result.Outcome() != application.JudgmentAsOfBasisNotResolved {
		t.Fatalf("outcome = %q, want BASIS_NOT_RESOLVED", result.Outcome())
	}
	if policies.loadCalled != 0 {
		t.Fatal("解析都没取回来，第二阶段却已经去问某个规则包声明了什么时点锚")
	}
}

// Covers: UC-PC-002 `AT-PC-028`「其他客户账户探测合同 → 输入未受理或范围拒绝，不泄露候选」在
// 第二阶段一侧（ADR-0027：解析标识不是能力凭证）。
//
// 只凭标识就交回闭包，任何拿到标识的人都能读走另一个客户的商业依据。这一支交回的取值与
// 「标识从未签发」**同为**`依据未解析`：两者的恢复动作相同，分开就等于告诉越权者这个标识是真的
// （ADR-0029）。交回「回第一阶段重解」对它没有用处——它拿自己的范围重解只会得到自己的标识。
func TestAResolutionNamedByAnotherCustomerIsNotAccepted(t *testing.T) {
	policies := &asOfPolicyDouble{}
	store, owner, resolution := storedResolution(t, resolvedWithRulePackage(t))

	intruder := owner
	intruder.CustomerAccountID = value(t, domain.NewCustomerAccountID, "customer-elsewhere")

	result, err := application.NewFormJudgmentAsOfHandler(store, policies).
		Handle(context.Background(), application.FormJudgmentAsOfCommand{
			Caller:     intruder,
			Resolution: resolution,
			Judgments:  []application.JudgmentAsOfRequest{{Judgment: domain.NetworkReachabilityJudgment, At: reachabilityAsOfAt}},
		})
	if err != nil {
		t.Fatalf("form as-of: %v", err)
	}

	if result.Outcome() != application.JudgmentAsOfBasisNotResolved {
		t.Fatalf("outcome = %q, want BASIS_NOT_RESOLVED——另一个客户凭标识读到了这份解析", result.Outcome())
	}
	if policies.loadCalled != 0 {
		t.Fatal("范围不符却仍去问了时点政策")
	}
}

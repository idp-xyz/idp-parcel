package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func closureKey(t *testing.T, scope string, required ...domain.CommercialObjectKind) domain.ClosureResolutionKey {
	t.Helper()
	base := resolutionKey(t, scope, required[0])
	key := domain.ClosureResolutionKey{
		TenantID:             base.TenantID,
		CustomerAccountID:    base.CustomerAccountID,
		LegalEntityCandidate: base.LegalEntityCandidate,
		Scope:                base.Scope,
		Purpose:              base.Purpose,
		Anchor:               base.Anchor,
		RequiredBases:        required,
	}
	for _, kind := range required {
		if kind == domain.SettlementPolicyObject {
			key.Settlement = settlementSelector(t)
		}
	}
	return key
}

// settlementSelector 与 registerSettlementPolicyIn 的适用范围逐维对齐：闭包请求结算依据时，
// 键上的精确范围要能命中夹具登记的那份政策（ADR-0044）。
func settlementSelector(t *testing.T) domain.SettlementSelector {
	t.Helper()
	return domain.SettlementSelector{
		Counterparty: commercialValue(t, domain.NewCounterpartyReference, "customer-1"),
		Contract:     commercialValue(t, domain.NewCommercialVersionLabel, "contract-1/v1"),
		ChargeScope:  commercialValue(t, domain.NewChargeScopeReference, "charge-express"),
		Currency:     commercialValue(t, domain.NewCurrencyCode, "SYN"),
	}
}

func registerSettlementPolicyIn(
	t *testing.T,
	registry *domain.CommercialRegistry,
	scope, objectID string,
	method domain.SettlementMethod,
) domain.SettlementPolicy {
	t.Helper()
	version := effectiveIn(t, registry, domain.SettlementPolicyObject, objectID, "v1", "sha256:"+objectID, scope)
	policy, err := domain.NewSettlementPolicy(version, method,
		applicability(t, "customer-1", "contract-1/v1", "charge-express", "SYN"))
	if err != nil {
		t.Fatalf("new settlement policy: %v", err)
	}
	registry.RegisterSettlementPolicy(policy)
	return policy
}

func seedClosure(t *testing.T, registry *domain.CommercialRegistry, scope string, kinds ...domain.CommercialObjectKind) {
	t.Helper()
	for _, kind := range kinds {
		if kind == domain.SettlementPolicyObject {
			// 结算依据经政策采用（ADR-0044）：光登记版本闭包看不见，政策一并登记。
			registerSettlementPolicyIn(t, registry, scope, "object-"+kind.String(), domain.PrepaidMethod)
			continue
		}
		effectiveIn(t, registry, kind, "object-"+kind.String(), "v1", "sha256:"+kind.String(), scope)
	}
}

var closureBases = []domain.CommercialObjectKind{
	domain.CustomerContractObject,
	domain.AcceptanceRulePackageObject,
	domain.SettlementPolicyObject,
}

// effectiveNaming 登记一个在正文里指名了对外引用的版本。引用属于一次受控发布固定下来的
// 内容，因此随规格给出，而不是登记之后再挂上去。
func effectiveNaming(
	t *testing.T,
	registry *domain.CommercialRegistry,
	kind domain.CommercialObjectKind,
	objectID, version, digest, scope string,
	references map[domain.CommercialObjectKind]string,
) domain.CommercialVersion {
	t.Helper()
	spec := commercialSpec(t, kind, objectID, version, digest)
	spec.Scope = commercialValue(t, domain.NewCommercialScopeReference, scope)
	spec.References = make(map[domain.CommercialObjectKind]domain.CommercialObjectID, len(references))
	for referencedKind, referencedID := range references {
		spec.References[referencedKind] = commercialValue(t, domain.NewCommercialObjectID, referencedID)
	}

	draft, err := domain.NewCommercialDraft(spec)
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	published, err := draft.Publish(
		approval(t, "approval-"+objectID+"-"+version),
		domain.ApprovalRoleConfirmed,
		time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		namedReferencesPublished(references),
	)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("register: %v", err)
	}
	return live
}

// Covers: `AT-PC-022`「合同唯一但引用规则包未发布 → 返回解析未决，不默认规则通过」。
//
// 合同指名的规则包只有草稿，登记册按规矩不收草稿；同范围里另有一个已发布的**别的**规则包。
// 按「范围 + 对象类型」各自独立解析时，那一个会被静静采用——它唯一、已生效、在范围内，
// 每一项都对，只是不是这份合同约定的那一个。
func TestAClosureRefusesARulePackageTheContractDoesNotName(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveNaming(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a",
		map[domain.CommercialObjectKind]string{domain.AcceptanceRulePackageObject: "rules-named"})
	effectiveIn(t, registry, domain.AcceptanceRulePackageObject, "rules-other", "v1", "sha256:other", "scope-a")

	closure := domain.ResolveCommercialClosure(registry,
		closureKey(t, "scope-a", domain.CustomerContractObject, domain.AcceptanceRulePackageObject), nil)

	if closure.Outcome() != domain.ResolutionPending {
		t.Fatalf("outcome = %q, want RESOLUTION_PENDING", closure.Outcome())
	}
	if adopted, present := closure.AdoptedFor(domain.AcceptanceRulePackageObject); present {
		t.Fatalf("闭包采用了合同没有指名的规则包 %q", adopted.Version().ObjectID())
	}
}

// Covers: `AT-PC-022` 的另一半——指名引用核得上时必须照常唯一解出。
//
// 少了这一条，「一律不确认」与「确认得对」在测试上无从分辨：上一条用例对一个恒假的
// 确认函数同样会绿，而那种实现会把每一次带指名引用的解析都判成未决。
func TestAClosureAdoptsTheRulePackageTheContractNames(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveNaming(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a",
		map[domain.CommercialObjectKind]string{domain.AcceptanceRulePackageObject: "rules-named"})
	effectiveIn(t, registry, domain.AcceptanceRulePackageObject, "rules-named", "v1", "sha256:named", "scope-a")

	closure := domain.ResolveCommercialClosure(registry,
		closureKey(t, "scope-a", domain.CustomerContractObject, domain.AcceptanceRulePackageObject), nil)

	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（reason=%q）", closure.Outcome(), closure.Reason())
	}
	adopted, present := closure.AdoptedFor(domain.AcceptanceRulePackageObject)
	if !present {
		t.Fatal("闭包没有采用任何规则包")
	}
	if adopted.Version().ObjectID().String() != "rules-named" {
		t.Fatalf("采用的规则包 = %q, want rules-named", adopted.Version().ObjectID())
	}
}

// Covers: 确认只覆盖本次要采用的类别这一条边界。
//
// 合同指名了规则包，但调用方本次只要合同。该类对象一个都不采用，也就没有采用错的风险；
// 判成未决就是闭包替调用方扩大了请求范围，而 RequiredBases 是调用方给的。
func TestANamedReferenceOutsideTheRequestedBasesDoesNotStallTheClosure(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveNaming(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a",
		map[domain.CommercialObjectKind]string{domain.AcceptanceRulePackageObject: "rules-named"})

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", domain.CustomerContractObject), nil)

	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（reason=%q）", closure.Outcome(), closure.Reason())
	}
}

// Covers: UC-PC-002 步骤 4 — 一次解析出合同引用的完整依据集合，每项各自唯一，并返回
// 采用版本、有效区间与当前修订标识。
func TestClosureResolvesEveryRequiredBasisUniquely(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", closureBases...), nil)

	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", closure.Outcome())
	}
	for _, kind := range closureBases {
		adopted, present := closure.AdoptedFor(kind)
		if !present || adopted.Kind() != kind {
			t.Fatalf("closure has no adopted version for %q", kind)
		}
	}
	if len(closure.Adopted()) != len(closureBases) {
		t.Fatalf("closure holds %d adopted versions, want %d", len(closure.Adopted()), len(closureBases))
	}
	if revision, present := closure.ViewRevision(); !present || revision.String() == "" {
		t.Fatal("closure carries no authority view revision")
	}
	if closure.ResolutionID().String() == "" {
		t.Fatal("a resolved closure carries no identity")
	}
}

// Covers: UC-PC-002 步骤 4「引用闭包不完整时未决或冲突」— 缺一项即整体不成立，且必须
// 指出缺的是哪一项，不得返回一个残缺的闭包让调用方自己发现。
func TestAnIncompleteClosureFailsAsAWholeAndNamesTheGap(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", domain.CustomerContractObject, domain.AcceptanceRulePackageObject)

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", closureBases...), nil)

	if closure.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS", closure.Outcome())
	}
	if len(closure.Adopted()) != 0 {
		t.Fatal("an incomplete closure still handed back partially adopted versions")
	}
	unresolved := closure.UnresolvedBases()
	if len(unresolved) != 1 || unresolved[0] != domain.SettlementPolicyObject {
		t.Fatalf("unresolved = %v, want exactly the settlement policy", unresolved)
	}
}

// Covers: UC-PC-002 — 任一必需依据存在多个候选时整体是适用冲突，不是部分成功。
func TestOneConflictingBasisMakesTheWholeClosureConflict(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)
	// 同一精确范围再登记一份账期政策：预付与账期同时命中即冲突（AT-PC-032 行为不变）。
	registerSettlementPolicyIn(t, registry, "scope-a", "rival-policy", domain.TermsMethod)

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", closureBases...), nil)

	if closure.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", closure.Outcome())
	}
	if len(closure.Adopted()) != 0 {
		t.Fatal("a conflicting closure still adopted the unambiguous members")
	}
	conflicting := closure.ConflictingBases()
	if len(conflicting) != 1 || conflicting[0] != domain.SettlementPolicyObject {
		t.Fatalf("conflicting = %v, want exactly the settlement policy", conflicting)
	}
}

// Covers: UC-PC-002 结果语义排序 — 同时存在冲突与缺失时报冲突，因为冲突要商业责任方
// 修正，而缺失只说明该范围没有对象；把冲突降级为缺失会让需要修正的问题看起来无需处理。
func TestConflictOutranksAMissingBasis(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", domain.CustomerContractObject, domain.SettlementPolicyObject)
	registerSettlementPolicyIn(t, registry, "scope-a", "rival-policy", domain.TermsMethod)

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", closureBases...), nil)

	if closure.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT to outrank the missing rule package", closure.Outcome())
	}
	if len(closure.UnresolvedBases()) != 1 {
		t.Fatal("the closure lost track of the missing basis while reporting the conflict")
	}
}

// Covers: 本次设计约束 — 闭包装的是异构采用引用，重复声明同一必需依据是键的错误，
// 不是解析结果。
func TestClosureKeyRefusesADuplicateRequiredBasis(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)

	key := closureKey(t, "scope-a", domain.CustomerContractObject, domain.CustomerContractObject)
	if got := domain.ResolveCommercialClosure(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
	}
}

// Covers: UC-PC-002 — 空的必需依据集合不构成一次解析请求。
func TestClosureKeyRequiresAtLeastOneBasis(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	key := closureKey(t, "scope-a", domain.CustomerContractObject)
	key.RequiredBases = nil

	if got := domain.ResolveCommercialClosure(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
	}
}

// Covers: UC-PC-002 — 权威读不到时整体未决，与「该范围没有适用对象」不同。
func TestUnreadableAuthorityMakesTheClosurePending(t *testing.T) {
	if got := domain.ResolveCommercialClosure(nil, closureKey(t, "scope-a", closureBases...), nil).Outcome(); got != domain.ResolutionPending {
		t.Fatalf("outcome = %q, want RESOLUTION_PENDING", got)
	}
}

// Covers: UC-PC-002 解析结果语义「解析未决 → 保存缺口并安全续办」与 S01-W02 契约中每个
// 未决都携带 reason 与 continuationRef。只报 outcome 说不出缺的是什么，也接不回去。
func TestEveryClosurePendingNamesItsReasonAndStaysResumable(t *testing.T) {
	unreadable := domain.ResolveCommercialClosure(nil, closureKey(t, "scope-a", closureBases...), nil)
	if unreadable.Reason() != domain.AuthorityUnreadable {
		t.Fatalf("reason = %q, want AUTHORITY_UNREADABLE", unreadable.Reason())
	}
	if unreadable.ContinuationReference().String() == "" {
		t.Fatal("权威读不到形成的未决无法续办")
	}

	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)
	unanchored := closureKey(t, "scope-a", closureBases...)
	unanchored.Anchor = domain.SelectionAnchor{}

	unconfigured := domain.ResolveCommercialClosure(registry, unanchored, nil)
	if unconfigured.Outcome() != domain.ResolutionPending {
		t.Fatalf("outcome = %q, want RESOLUTION_PENDING", unconfigured.Outcome())
	}
	if unconfigured.Reason() != domain.AnchorPolicyNotConfigured {
		t.Fatalf("reason = %q, want ANCHOR_POLICY_NOT_CONFIGURED", unconfigured.Reason())
	}
	if unconfigured.ContinuationReference().String() == "" {
		t.Fatal("锚点策略未配置形成的未决无法续办")
	}

	// 两种未决要采取的行动不同：锚点未配置等的是实例参数落地，权威读不到等的是重试。
	// 原因一旦压平，调用方就只能靠猜。
	if unreadable.ContinuationReference() == unconfigured.ContinuationReference() {
		t.Fatal("两种原因的未决共用了同一个续办引用")
	}
}

// Covers: UC-PC-002 — 同一输入因同一原因停滞时续办引用必须稳定，否则调用方查不回原次尝试；
// 而必需依据集合不同就是另一次解析，不该借用同一个引用。
func TestClosureContinuationIsStablePerInputAndReason(t *testing.T) {
	key := closureKey(t, "scope-a", closureBases...)

	first := domain.ResolveCommercialClosure(nil, key, nil)
	second := domain.ResolveCommercialClosure(nil, key, nil)
	if first.ContinuationReference() != second.ContinuationReference() {
		t.Fatalf("同一输入同一原因给出了不同续办引用: %q vs %q",
			first.ContinuationReference().String(), second.ContinuationReference().String())
	}

	fewer := domain.ResolveCommercialClosure(nil, closureKey(t, "scope-a", domain.CustomerContractObject), nil)
	if fewer.ContinuationReference() == first.ContinuationReference() {
		t.Fatal("必需依据集合不同的两次解析共用了续办引用")
	}
}

func pricingClosureKey(t *testing.T, scope string, direction domain.PriceDirection, required ...domain.CommercialObjectKind) domain.ClosureResolutionKey {
	t.Helper()
	key := closureKey(t, scope, required...)
	key.Purpose = domain.PricingPurpose
	key.PriceDirection = direction
	return key
}

// Covers: `AT-PC-035`（闭包级）「同一客户范围分别请求 SELL 计费和 BUY 成本 → 返回各自独立
// 的商业政策、方向和定价方案绑定，不复用另一方向结果」。
//
// 单元级已由 `ResolveCommercialPricePolicy` 钉住；本用例钉的是引用闭包成功路径必须经政策
// 解析，并把方向与绑定挂在 AdoptedBasis 上（ADR-0034）。
func TestClosureAdoptsIndependentPricePoliciesPerDirection(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)
	registerPricePolicy(t, registry, "policy-sell", domain.SellDirection, "scope-a", "plan-sell")
	registerPricePolicy(t, registry, "policy-buy", domain.BuyDirection, "scope-a", "plan-buy")

	required := append([]domain.CommercialObjectKind{}, closureBases...)
	required = append(required, domain.PriceRuleObject)

	sell := domain.ResolveCommercialClosure(registry,
		pricingClosureKey(t, "scope-a", domain.SellDirection, required...), allPlansAdoptable)
	buy := domain.ResolveCommercialClosure(registry,
		pricingClosureKey(t, "scope-a", domain.BuyDirection, required...), allPlansAdoptable)

	if sell.Outcome() != domain.UniquelyResolved || buy.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcomes sell=%q buy=%q", sell.Outcome(), buy.Outcome())
	}
	if sell.ResolutionID() == buy.ResolutionID() {
		t.Fatal("SELL and BUY closures shared one resolution identity")
	}

	sellPolicy, sellOK := mustAdoptedPricePolicy(t, sell)
	buyPolicy, buyOK := mustAdoptedPricePolicy(t, buy)
	if !sellOK || !buyOK {
		t.Fatal("closure adopted a price rule without a price policy binding")
	}
	if sellPolicy.Direction() != domain.SellDirection || sellPolicy.PricingPlan().String() != "plan-sell" {
		t.Fatalf("sell binding = %q/%q", sellPolicy.Direction(), sellPolicy.PricingPlan())
	}
	if buyPolicy.Direction() != domain.BuyDirection || buyPolicy.PricingPlan().String() != "plan-buy" {
		t.Fatalf("buy binding = %q/%q", buyPolicy.Direction(), buyPolicy.PricingPlan())
	}
	if sellPolicy.PricingPlan() == buyPolicy.PricingPlan() {
		t.Fatal("SELL reused the BUY pricing plan binding")
	}
}

func mustAdoptedPricePolicy(t *testing.T, closure domain.CommercialClosure) (domain.CommercialPricePolicy, bool) {
	t.Helper()
	adopted, present := closure.AdoptedFor(domain.PriceRuleObject)
	if !present {
		t.Fatal("closure did not adopt a price rule")
	}
	return adopted.PricePolicy()
}

// Covers: ADR-0034 — 计价闭包若只登记了版本、没有政策，不得靠版本冒充带方向的绑定。
func TestPricingClosureWithoutPolicyIsNoApplicableBasis(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)
	effectiveIn(t, registry, domain.PriceRuleObject, "price-only", "v1", "sha256:p", "scope-a")

	required := append([]domain.CommercialObjectKind{}, closureBases...)
	required = append(required, domain.PriceRuleObject)

	closure := domain.ResolveCommercialClosure(registry,
		pricingClosureKey(t, "scope-a", domain.SellDirection, required...), allPlansAdoptable)
	if closure.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS", closure.Outcome())
	}
	if _, present := closure.AdoptedFor(domain.PriceRuleObject); present {
		t.Fatal("a version-only price rule was adopted under pricing purpose")
	}
}

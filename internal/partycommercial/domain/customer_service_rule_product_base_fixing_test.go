package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件钉 ADR-0268 决定一：闭包采纳挂合同的那一版客户服务规则时，继承底座在同一次解析里按同一份键与锚点选出、随
// 闭包一并固定；多于一版是这一项的`适用冲突`；底座与采用的成员同受指名引用核对。

// inheritedBaseOf 交回闭包采纳的那一版客户服务规则上固定的继承底座；没采纳就当场失败。
func inheritedBaseOf(t *testing.T, closure domain.CommercialClosure) (domain.CommercialVersion, bool, bool) {
	t.Helper()
	adopted, present := closure.AdoptedFor(domain.CustomerServiceRuleObject)
	if !present {
		t.Fatalf("闭包没有采纳客户服务规则（outcome=%q，reason=%q，conflicting=%v）",
			closure.Outcome(), closure.Reason(), closure.ConflictingBases())
	}
	return adopted.InheritedProductBase()
}

// Covers: ADR-0268 决定一「恰一版：作为这一项采纳的继承底座随闭包一并固定」——底座记在采纳的那一版之下，采纳的
// 仍只有合同版；声明次序不改变选出的底座。
func TestTheClosureFixesTheProductBaseOfTheAdoptedContractTierRule(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerContract(t, registry, fixtureContractObjectID)
	registerServiceRule(t, registry, "csr-contract", "v1", namesContract(fixtureContractObjectID))
	registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))

	for name, bases := range map[string][]domain.CommercialObjectKind{
		"合同先声明": {domain.CustomerContractObject, domain.CustomerServiceRuleObject},
		"规则先声明": {domain.CustomerServiceRuleObject, domain.CustomerContractObject},
	} {
		t.Run(name, func(t *testing.T) {
			closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", bases...), nil)
			if closure.Outcome() != domain.UniquelyResolved {
				t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", closure.Outcome())
			}
			if got := adoptedServiceRule(t, closure); got != "csr-contract" {
				t.Fatalf("采纳的客户服务规则 = %q, want csr-contract", got)
			}
			base, hasBase, fixed := inheritedBaseOf(t, closure)
			if !fixed || !hasBase || base.ObjectID().String() != "csr-product" || base.Version().String() != "v1" {
				t.Fatalf("继承底座 = %q/%q（在场=%v，已固定=%v），want csr-product/v1 已固定",
					base.ObjectID(), base.Version(), hasBase, fixed)
			}
			if len(closure.Adopted()) != len(bases) {
				t.Fatalf("采用了 %d 项, want %d——底座不是第二个已采用依据", len(closure.Adopted()), len(bases))
			}
		})
	}
}

// Covers: ADR-0268 决定一「零版：记没有底座」——同范围没有产品层的版本时闭包照常唯一解析，底座一格如实记「已固定、
// 没有」，与「不知道」（未固定）分开。
func TestAContractTierRuleWithoutAProductBaseIsFixedAsHavingNone(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerContract(t, registry, fixtureContractObjectID)
	registerServiceRule(t, registry, "csr-contract", "v1", namesContract(fixtureContractObjectID))
	registerServiceRule(t, registry, "csr-other-contract", "v1", namesContract("contract-2"))

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", serviceRuleBases...), nil)
	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", closure.Outcome())
	}
	base, hasBase, fixed := inheritedBaseOf(t, closure)
	if !fixed || hasBase {
		t.Fatalf("继承底座 = %q（在场=%v，已固定=%v），want 已固定且没有——别户的合同版不是底座", base.ObjectID(), hasBase, fixed)
	}
}

// Covers: ADR-0268 决定一「多版：客户服务规则这一项答`适用冲突`……在接受时就挡」——合同层恰一版而产品层两版时，闭包
// 不采纳合同版、不挑一版底座，冲突清单里只有客户服务规则。
func TestTwoProductBaseCandidatesMakeTheContractTierRuleAnApplicabilityConflict(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerContract(t, registry, fixtureContractObjectID)
	registerServiceRule(t, registry, "csr-contract", "v1", namesContract(fixtureContractObjectID))
	registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))
	registerServiceRule(t, registry, "csr-bare", "v1", nil)

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", serviceRuleBases...), nil)
	if closure.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", closure.Outcome())
	}
	conflicting := closure.ConflictingBases()
	if len(conflicting) != 1 || conflicting[0] != domain.CustomerServiceRuleObject {
		t.Fatalf("conflicting = %v, want exactly the customer service rule", conflicting)
	}
	if len(closure.Adopted()) != 0 {
		t.Fatal("冲突的闭包仍交回了已采纳的成员")
	}
}

// Covers: ADR-0268 决定一「底座与采用的成员同受指名引用核对」——闭包键没声明服务产品、闭包采用 product-a，底座壳上
// 指名 product-b：闭包答`解析未决`（NamedReferenceNotConfirmed），与采用的成员指名错了同一格。声明了服务产品时
// 那一版底座在选法里就落选，闭包照常唯一解析、记没有底座。
func TestAProductBaseNamingAnotherServiceProductLeavesTheClosurePending(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerContract(t, registry, fixtureContractObjectID)
	product := registerServiceProductIn(t, registry, "scope-a", "product-a")
	registerServiceRule(t, registry, "csr-contract", "v1", namesContract(fixtureContractObjectID))
	registerServiceRule(t, registry, "csr-product-b", "v1", namesProduct("product-b"))

	bases := []domain.CommercialObjectKind{
		domain.ServiceProductObject, domain.CustomerContractObject, domain.CustomerServiceRuleObject,
	}
	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", bases...), nil)
	if closure.Outcome() != domain.ResolutionPending || closure.Reason() != domain.NamedReferenceNotConfirmed {
		t.Fatalf("outcome = %q reason = %q, want RESOLUTION_PENDING / NAMED_REFERENCE_NOT_CONFIRMED",
			closure.Outcome(), closure.Reason())
	}

	declared := closureKey(t, "scope-a", bases...)
	declared.ServiceProduct = product.Version().ObjectID()
	closure = domain.ResolveCommercialClosure(registry, declared, nil)
	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("声明 product-a：outcome = %q reason = %q, want UNIQUELY_RESOLVED", closure.Outcome(), closure.Reason())
	}
	if _, hasBase, fixed := inheritedBaseOf(t, closure); !fixed || hasBase {
		t.Fatalf("声明 product-a：底座在场=%v 已固定=%v，want 已固定且没有", hasBase, fixed)
	}
}

// Covers: ADR-0268 决定一只说「闭包采纳挂合同的那一版时」——采纳的是产品层那一版（含闭包没请求合同），它自己就是
// 产品标准，没有继承底座可固定。
func TestAProductTierAdoptionFixesNoProductBase(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerContract(t, registry, fixtureContractObjectID)
	registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))

	for name, bases := range map[string][]domain.CommercialObjectKind{
		"请求了合同": serviceRuleBases,
		"没请求合同": {domain.CustomerServiceRuleObject},
	} {
		t.Run(name, func(t *testing.T) {
			closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", bases...), nil)
			if _, hasBase, fixed := inheritedBaseOf(t, closure); fixed || hasBase {
				t.Fatalf("产品层那一版：底座在场=%v 已固定=%v，want 都不", hasBase, fixed)
			}
		})
	}
}

// rehydrationOfServiceRule 是重建门的一份最小闭包规格：客户合同加一版客户服务规则，底座各格由调用方填。
func rehydrationOfServiceRule(
	t *testing.T,
	rule domain.CommercialVersion,
	base domain.RehydrateAdoptedBasisSpec,
) domain.RehydrateCommercialClosureSpec {
	t.Helper()
	contract := effectiveIn(t, domain.NewCommercialRegistry(), domain.CustomerContractObject,
		fixtureContractObjectID, "v1", "sha256:"+fixtureContractObjectID, "scope-a")
	key := closureKey(t, "scope-a", serviceRuleBases...)
	ruleSpec := base
	ruleSpec.Kind = domain.CustomerServiceRuleObject
	ruleSpec.Version = rule
	return domain.RehydrateCommercialClosureSpec{
		Outcome:      domain.UniquelyResolved,
		ResolutionID: commercialValue(t, domain.NewResolutionID, "CLO-rehydrated"),
		Key:          key,
		Anchor:       key.Anchor,
		ViewRevision: commercialValue(t, domain.NewAuthorityViewRevision, "view-1"),
		Adopted: []domain.RehydrateAdoptedBasisSpec{
			{Kind: domain.CustomerContractObject, Version: contract},
			ruleSpec,
		},
	}
}

// Covers: ADR-0268 决定一「闭包快照记它的版本身份」的重建门一半——写得出的三种形（固定了一版、固定了没有、没有这一格）
// 都读得回且如实；写不出的形（底座挂在不是合同版的一格上、底座挂合同、底座属别的租户或范围、在场却未固定）一律拒，
// 不吸收成「没有底座」。
func TestTheRehydrationGateKeepsTheFixedProductBaseHonest(t *testing.T) {
	scratch := domain.NewCommercialRegistry()
	contractRule := registerServiceRule(t, scratch, "csr-contract", "v1", namesContract(fixtureContractObjectID))
	productRule := registerServiceRule(t, scratch, "csr-product", "v1", namesProduct("product-a"))
	otherContractRule := registerServiceRule(t, scratch, "csr-other-contract", "v1", namesContract("contract-2"))
	otherScopeRule := effectiveNaming(t, scratch, domain.CustomerServiceRuleObject,
		"csr-scope-b", "v1", "sha256:csr-scope-b", "scope-b", namesProduct("product-a"))
	otherTenantRule := otherTenantServiceRule(t, "csr-tenant-2")

	accepted := map[string]struct {
		rule        domain.CommercialVersion
		base        domain.RehydrateAdoptedBasisSpec
		wantBase    string
		wantHasBase bool
		wantFixed   bool
	}{
		"固定了一版": {contractRule, domain.RehydrateAdoptedBasisSpec{
			ProductBase: productRule, HasProductBase: true, ProductBaseFixed: true}, "csr-product", true, true},
		"固定了没有": {contractRule, domain.RehydrateAdoptedBasisSpec{ProductBaseFixed: true}, "", false, true},
		"没有这一格（本决定之前的快照）": {contractRule, domain.RehydrateAdoptedBasisSpec{}, "", false, false},
	}
	for name, tc := range accepted {
		t.Run(name, func(t *testing.T) {
			closure, err := domain.RehydrateCommercialClosure(rehydrationOfServiceRule(t, tc.rule, tc.base))
			if err != nil {
				t.Fatalf("重建门拒了一份写得出的快照：%v", err)
			}
			base, hasBase, fixed := inheritedBaseOf(t, closure)
			if hasBase != tc.wantHasBase || fixed != tc.wantFixed || (hasBase && base.ObjectID().String() != tc.wantBase) {
				t.Fatalf("底座 = %q（在场=%v，已固定=%v），want %q（在场=%v，已固定=%v）",
					base.ObjectID(), hasBase, fixed, tc.wantBase, tc.wantHasBase, tc.wantFixed)
			}
		})
	}

	refused := map[string]struct {
		rule domain.CommercialVersion
		base domain.RehydrateAdoptedBasisSpec
	}{
		"采纳的是产品层那一版": {productRule, domain.RehydrateAdoptedBasisSpec{ProductBaseFixed: true}},
		"在场却未固定": {contractRule, domain.RehydrateAdoptedBasisSpec{
			ProductBase: productRule, HasProductBase: true}},
		"底座挂合同": {contractRule, domain.RehydrateAdoptedBasisSpec{
			ProductBase: otherContractRule, HasProductBase: true, ProductBaseFixed: true}},
		"底座在别的范围": {contractRule, domain.RehydrateAdoptedBasisSpec{
			ProductBase: otherScopeRule, HasProductBase: true, ProductBaseFixed: true}},
		"底座属别的租户": {contractRule, domain.RehydrateAdoptedBasisSpec{
			ProductBase: otherTenantRule, HasProductBase: true, ProductBaseFixed: true}},
		"底座不是客户服务规则": {contractRule, domain.RehydrateAdoptedBasisSpec{
			ProductBase:    effectiveIn(t, scratch, domain.ServiceProductObject, "product-a", "v1", "sha256:product-a", "scope-a"),
			HasProductBase: true, ProductBaseFixed: true}},
	}
	for name, tc := range refused {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.RehydrateCommercialClosure(rehydrationOfServiceRule(t, tc.rule, tc.base)); !errors.Is(err, domain.ErrInvalidRehydratedResolution) {
				t.Fatalf("err = %v, want ErrInvalidRehydratedResolution", err)
			}
		})
	}

	t.Run("底座挂在客户合同那一格上", func(t *testing.T) {
		spec := rehydrationOfServiceRule(t, contractRule, domain.RehydrateAdoptedBasisSpec{})
		spec.Adopted[0].ProductBase, spec.Adopted[0].HasProductBase, spec.Adopted[0].ProductBaseFixed = productRule, true, true
		if _, err := domain.RehydrateCommercialClosure(spec); !errors.Is(err, domain.ErrInvalidRehydratedResolution) {
			t.Fatalf("err = %v, want ErrInvalidRehydratedResolution", err)
		}
	})
}

// Covers: ADR-0268 决定一「已接受委托读底座，只读闭包上固定的那一版」的领域一半——固定之后底座被同起点退役、新版
// 接替，同一份闭包上记的仍是接受时那一版；同一键此后重解才选到新版。
func TestTheFixedProductBaseDoesNotFollowALaterProductVersion(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerContract(t, registry, fixtureContractObjectID)
	registerServiceRule(t, registry, "csr-contract", "v1", namesContract(fixtureContractObjectID))
	registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))
	key := closureKey(t, "scope-a", serviceRuleBases...)

	fixed := domain.ResolveCommercialClosure(registry, key, nil)
	if base, _, _ := inheritedBaseOf(t, fixed); base.Version().String() != "v1" {
		t.Fatalf("接受时底座 = %q, want v1", base.Version())
	}

	// 有效性更正把 v1 的选用区间挪到锚点之后，再登 v2：此刻按同一锚点只剩 v2。
	v1, _ := registry.Lookup(key.TenantID, domain.CustomerServiceRuleObject,
		commercialValue(t, domain.NewCommercialObjectID, "csr-product"), commercialValue(t, domain.NewCommercialVersionLabel, "v1"))
	moved, err := domain.NewEffectiveInterval(key.Anchor.At().Add(24*time.Hour), v1.Effective().StartsAt().AddDate(1, 0, 0))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	correction, err := v1.CorrectEffectiveInterval(moved,
		commercialValue(t, domain.NewValidityCorrectionReference, "SYN-CORRECT-1"), key.Anchor.At().Add(48*time.Hour))
	if err != nil {
		t.Fatalf("区间更正：%v", err)
	}
	if _, err := registry.RegisterValidityCorrection(correction); err != nil {
		t.Fatalf("登记区间更正：%v", err)
	}
	registerServiceRule(t, registry, "csr-product", "v2", namesProduct("product-a"))

	if base, _, _ := inheritedBaseOf(t, fixed); base.Version().String() != "v1" {
		t.Fatalf("已固定的闭包上底座变成了 %q", base.Version())
	}
	again := domain.ResolveCommercialClosure(registry, key, nil)
	if base, _, _ := inheritedBaseOf(t, again); base.Version().String() != "v2" {
		t.Fatalf("同一键此刻重解底座 = %q, want v2——夹具没咬到", base.Version())
	}
}

// otherTenantServiceRule 造一版同范围、挂 product-a、却属 tenant-2 的已发布客户服务规则：除租户之外与合格底座一模一样，
// 重建门要单凭租户这一维拒它。
func otherTenantServiceRule(t *testing.T, objectID string) domain.CommercialVersion {
	t.Helper()
	spec := commercialSpec(t, domain.CustomerServiceRuleObject, objectID, "v1", "sha256:"+objectID)
	spec.TenantID = commercialValue(t, domain.NewTenantID, "tenant-2")
	spec.Scope = commercialValue(t, domain.NewCommercialScopeReference, "scope-a")
	spec.References = map[domain.CommercialObjectKind]domain.CommercialObjectID{
		domain.ServiceProductObject: commercialValue(t, domain.NewCommercialObjectID, "product-a"),
	}
	draft, err := domain.NewCommercialDraft(spec)
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	productPublished := func(domain.CommercialObjectKind, domain.CommercialObjectID) domain.NamedReferenceStanding {
		return domain.NamedReferencePublished
	}
	published, err := draft.Publish(approval(t, "approval-tenant-2-"+objectID), domain.ApprovalRoleConfirmed,
		time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), productPublished)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return published
}

package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证价格政策册（ADR-0034 / ADR-0057）：政策随整册一次取回、
// 发布期方案方向与转换原样交回构造函数、同内容重放、异内容冲突、租户隔离、缺政策是
// 合法缺席、登记推动 ViewRevision、库内封闭集 CHECK。

func TestPricePolicyRoundTripsWithTheRegistry(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-1", "v1", "digest-price-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	policy := pricePolicyOn(t, version, domain.SellDirection, domain.SellDirection, domain.PlanBindingConversionNone, "plan-sell")
	mustSavePricePolicy(t, transactor, ctx, repository, policy, domain.SellDirection, domain.PlanBindingConversionNone)

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	policies := registry.PricePolicies()
	if len(policies) != 1 {
		t.Fatalf("读回 %d 份价格政策，want 1", len(policies))
	}
	if policies[0].Direction() != domain.SellDirection {
		t.Fatalf("direction = %q", policies[0].Direction())
	}
	if policies[0].PricingPlan().String() != "plan-sell" {
		t.Fatalf("plan = %q", policies[0].PricingPlan())
	}
	if !policies[0].Version().SameVersionAs(version) {
		t.Fatal("政策挂回了另一个版本")
	}
}

// Covers: ADR-0057——SELL 政策绑 BUY 价卡并声明冻结采购评价，装载必须原样交回转换，
// 否则 checkPlanBinding 会在装载面把一次合法发布读成冲突。
func TestAFrozenBuyBindingRoundTripsThroughLoad(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-1", "v1", "digest-price-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	policy := pricePolicyOn(t, version, domain.SellDirection, domain.BuyDirection, domain.PlanBindingFrozenBuyEvaluation, "plan-buy")
	mustSavePricePolicy(t, transactor, ctx, repository, policy, domain.BuyDirection, domain.PlanBindingFrozenBuyEvaluation)

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	if len(registry.PricePolicies()) != 1 {
		t.Fatal("声明了转换的政策装不回来")
	}
}

func TestRegisteringAPricePolicyMovesTheScopeViewRevision(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()
	tenant, scope := pcTenant(t, "tenant-1"), pcScope(t)

	version := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-1", "v1", "digest-price-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	beforeRegistry, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("登记政策前读回：%v", err)
	}
	before := beforeRegistry.ViewRevision(tenant, scope)

	policy := pricePolicyOn(t, version, domain.SellDirection, domain.SellDirection, domain.PlanBindingConversionNone, "plan-sell")
	mustSavePricePolicy(t, transactor, ctx, repository, policy, domain.SellDirection, domain.PlanBindingConversionNone)

	afterRegistry, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("登记政策后读回：%v", err)
	}
	if afterRegistry.ViewRevision(tenant, scope) == before {
		t.Fatal("价格政策从缺席变为在场，范围修订却没动")
	}
}

func TestAPriceVersionWithoutARegisteredPolicyStillLoads(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-1", "v1", "digest-price-1")
	mustSaveVersion(t, transactor, ctx, repository, version)

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	if registry.Count() != 1 {
		t.Fatalf("版本数 = %d, want 1", registry.Count())
	}
	if policies := registry.PricePolicies(); len(policies) != 0 {
		t.Fatalf("没登记政策却读回 %d 份", len(policies))
	}
}

func TestSavingTheSamePricePolicyTwiceIsAReplay(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-1", "v1", "digest-price-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	policy := pricePolicyOn(t, version, domain.SellDirection, domain.SellDirection, domain.PlanBindingConversionNone, "plan-sell")
	mustSavePricePolicy(t, transactor, ctx, repository, policy, domain.SellDirection, domain.PlanBindingConversionNone)

	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SavePricePolicy(txCtx, policy, domain.SellDirection, domain.PlanBindingConversionNone)
		if err != nil {
			return err
		}
		if outcome != ports.PricePolicyAlreadyRegistered {
			t.Fatalf("replay outcome = %q, want ALREADY_REGISTERED", outcome)
		}
		return nil
	})
}

// Covers: ADR-0057 第四条——方案方向与转换参与同键内容判定。结构体上看不见这两项，
// 但同一版本从「同向无转换」改成「跨向冻结采购评价」是另一份发布内容。
func TestADifferentPlanBindingOnTheSameVersionConflicts(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-1", "v1", "digest-price-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	original := pricePolicyOn(t, version, domain.SellDirection, domain.SellDirection, domain.PlanBindingConversionNone, "plan-1")
	mustSavePricePolicy(t, transactor, ctx, repository, original, domain.SellDirection, domain.PlanBindingConversionNone)

	changed := pricePolicyOn(t, version, domain.SellDirection, domain.BuyDirection, domain.PlanBindingFrozenBuyEvaluation, "plan-1")
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SavePricePolicy(txCtx, changed, domain.BuyDirection, domain.PlanBindingFrozenBuyEvaluation)
		if err != nil {
			return err
		}
		if outcome != ports.PricePolicyContentConflict {
			t.Fatalf("conflict outcome = %q, want CONTENT_CONFLICT", outcome)
		}
		return nil
	})
}

func TestAPricePolicyInAnotherScopeDoesNotLoad(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-1", "v1", "digest-price-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	mustSavePricePolicy(t, transactor, ctx, repository,
		pricePolicyOn(t, version, domain.SellDirection, domain.SellDirection, domain.PlanBindingConversionNone, "plan-sell"),
		domain.SellDirection, domain.PlanBindingConversionNone)

	otherScope := pcValue(t, domain.NewCommercialScopeReference, "scope-other")
	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), otherScope)
	if err != nil {
		t.Fatalf("另一范围读回：%v", err)
	}
	if registry.Count() != 0 {
		t.Fatalf("另一范围读到 %d 个版本", registry.Count())
	}
	if policies := registry.PricePolicies(); len(policies) != 0 {
		t.Fatalf("另一范围读到 %d 份价格政策", len(policies))
	}
}

func TestAnotherTenantsPricePolicyDoesNotEnterThisScopesRegistry(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	mine := policyVersionInTenant(t, "tenant-1", domain.PriceRuleObject, "policy-1", "v1", "digest-mine")
	theirs := policyVersionInTenant(t, "tenant-2", domain.PriceRuleObject, "policy-1", "v1", "digest-theirs")
	mustSaveVersion(t, transactor, ctx, repository, mine)
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSavePricePolicy(t, transactor, ctx, repository,
		pricePolicyOn(t, theirs, domain.SellDirection, domain.SellDirection, domain.PlanBindingConversionNone, "plan-theirs"),
		domain.SellDirection, domain.PlanBindingConversionNone)

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	if registry.Count() != 1 {
		t.Fatalf("本租户册里有 %d 个版本，want 1", registry.Count())
	}
	if policies := registry.PricePolicies(); len(policies) != 0 {
		t.Fatalf("他租登记的价格政策进了本租户的册：%d 份", len(policies))
	}
}

func TestAPriceDirectionOutsideTheClosedSetIsRefused(t *testing.T) {
	repository, transactor, pool := newPublications(t)
	ctx := t.Context()
	version := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-1", "v1", "digest-price-1")
	mustSaveVersion(t, transactor, ctx, repository, version)

	_, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.commercial_price_policy
			(tenant_id, object_kind, object_id, version_label,
			 direction, plan_ref, plan_direction, binding_conversion,
			 policy_scope_ref, effective_starts_at)
		 VALUES ('tenant-1', 6, 'policy-1', 'v1',
		         'SPOT', 'plan-1', 'SELL', 'NONE', 'scope-1', now())`)
	if err == nil {
		t.Fatal("封闭集之外的价格方向进了价格政策册")
	}
}

func TestAPlanBindingConversionOutsideTheClosedSetIsRefused(t *testing.T) {
	repository, transactor, pool := newPublications(t)
	ctx := t.Context()
	version := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-1", "v1", "digest-price-1")
	mustSaveVersion(t, transactor, ctx, repository, version)

	_, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.commercial_price_policy
			(tenant_id, object_kind, object_id, version_label,
			 direction, plan_ref, plan_direction, binding_conversion,
			 policy_scope_ref, effective_starts_at)
		 VALUES ('tenant-1', 6, 'policy-1', 'v1',
		         'SELL', 'plan-1', 'BUY', 'IMPLICIT', 'scope-1', now())`)
	if err == nil {
		t.Fatal("封闭集之外的绑定转换进了价格政策册")
	}
}

func mustSavePricePolicy(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	policy domain.CommercialPricePolicy,
	planDirection domain.PriceDirection,
	conversion domain.PlanBindingConversion,
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SavePricePolicy(txCtx, policy, planDirection, conversion)
		if err != nil {
			return err
		}
		if outcome != ports.PricePolicySaved {
			t.Fatalf("save outcome = %q, want SAVED", outcome)
		}
		return nil
	})
}

func pricePolicyOn(
	t *testing.T,
	version domain.CommercialVersion,
	direction, planDirection domain.PriceDirection,
	conversion domain.PlanBindingConversion,
	plan string,
) domain.CommercialPricePolicy {
	t.Helper()
	policy, err := domain.NewCommercialPricePolicy(
		version,
		direction,
		pcValue(t, domain.NewPricingPlanReference, plan),
		planDirection,
		conversion,
		pcScope(t),
		version.Effective(),
	)
	if err != nil {
		t.Fatalf("new commercial price policy: %v", err)
	}
	return policy
}

func policyVersionInTenant(
	t *testing.T,
	tenant string,
	kind domain.CommercialObjectKind,
	objectID, label, digest string,
) domain.CommercialVersion {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-1"),
		pcValue(t, domain.NewCommercialSourceReference, "source-1"),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, tenant),
		Kind:          kind,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, digest),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAtRow,
	})
	if err != nil {
		t.Fatalf("重建版本：%v", err)
	}
	return version
}

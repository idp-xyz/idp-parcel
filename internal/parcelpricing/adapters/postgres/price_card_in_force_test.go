package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件对真实 PostgreSQL 16 证在用价卡解析读口（票 sa-cc/11 裁决 3）的三格封闭：唯一命中交回方案、
// 零命中答未配置、多于一版答适用冲突并列出候选——不挑、不排序、不种任何优先级。夹具全部为合成价卡
// （S 级），零生产默认。

func resolveInForce(t *testing.T, catalog ports.PriceCardInForceResolver, tenant string, direction domain.PricingDirection, purpose domain.PricingPurpose, at time.Time) ports.PriceCardInForceResolution {
	t.Helper()
	resolution, err := catalog.ResolveInForce(t.Context(),
		evaluationValue(t, domain.NewTenantID, tenant),
		evaluationValue(t, domain.NewPricingScopeID, "scope-1"),
		direction, purpose, at)
	if err != nil {
		t.Fatalf("解析在用价卡：%v", err)
	}
	return resolution
}

// TestInForcePriceCardResolvesTheOnlyApplicableVersion 证唯一命中：交回的方案就是登记的那一版，读回经
// 领域整图重验。
func TestInForcePriceCardResolvesTheOnlyApplicableVersion(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	ctx := t.Context()

	plan := catalogPlan(t, "plan-buy", "v1", domain.PricingDirectionBuy, "12")
	registerCard(t, catalog, transactor, ctx, cardRegistration(t, "tenant-a", plan))

	resolution := resolveInForce(t, catalog, "tenant-a", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, catalogAsOf)
	if resolution.Outcome != ports.PriceCardVersionInForce {
		t.Fatalf("outcome = %s, 想要 IN_FORCE", resolution.Outcome)
	}
	if !resolution.Plan.Reference().SameIdentity(plan.Reference()) || resolution.Plan.ContentDigest() != plan.ContentDigest() {
		t.Fatalf("交回的方案不是登记的那一版：%s@%s", resolution.Plan.Reference().ID(), resolution.Plan.Reference().Version())
	}
	if len(resolution.Candidates) != 0 {
		t.Fatalf("唯一命中不该带候选：%v", resolution.Candidates)
	}
}

// TestInForcePriceCardAnswersNotConfiguredHonestly 证零命中的三种来路都答未配置：册上没卡、只有另一方向
// 的卡、卡在但时点落在适用期外。三种都不是错误——缺的是价卡。
func TestInForcePriceCardAnswersNotConfiguredHonestly(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	ctx := t.Context()

	if got := resolveInForce(t, catalog, "tenant-a", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, catalogAsOf); got.Outcome != ports.PriceCardNotConfigured {
		t.Fatalf("空册 outcome = %s, 想要 NOT_CONFIGURED", got.Outcome)
	}

	registerCard(t, catalog, transactor, ctx,
		cardRegistration(t, "tenant-a", catalogPlan(t, "plan-sell", "v1", domain.PricingDirectionSell, "10")))
	if got := resolveInForce(t, catalog, "tenant-a", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, catalogAsOf); got.Outcome != ports.PriceCardNotConfigured {
		t.Fatalf("只有 SELL 卡时 BUY 侧 outcome = %s, 想要 NOT_CONFIGURED", got.Outcome)
	}

	registerCard(t, catalog, transactor, ctx,
		cardRegistration(t, "tenant-a", catalogPlan(t, "plan-buy", "v1", domain.PricingDirectionBuy, "12")))
	outside := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	if got := resolveInForce(t, catalog, "tenant-a", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, outside); got.Outcome != ports.PriceCardNotConfigured {
		t.Fatalf("适用期外 outcome = %s, 想要 NOT_CONFIGURED", got.Outcome)
	}
	if got := resolveInForce(t, catalog, "tenant-b", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, catalogAsOf); got.Outcome != ports.PriceCardNotConfigured {
		t.Fatalf("别的租户 outcome = %s, 想要 NOT_CONFIGURED（ADR-0003 隔离）", got.Outcome)
	}
}

// TestInForcePriceCardRefusesToChooseBetweenApplicableVersions 证多于一版同时适用答适用冲突：两张不同身份的
// BUY 卡都在期内，候选把两版都列出来，方案一格为空——不挑金额低的、不挑后登记的。
func TestInForcePriceCardRefusesToChooseBetweenApplicableVersions(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	ctx := t.Context()

	first := catalogPlan(t, "plan-buy-a", "v1", domain.PricingDirectionBuy, "12")
	second := catalogPlan(t, "plan-buy-b", "v1", domain.PricingDirectionBuy, "9")
	registerCard(t, catalog, transactor, ctx, cardRegistration(t, "tenant-a", first))
	registerCard(t, catalog, transactor, ctx, cardRegistration(t, "tenant-a", second))

	resolution := resolveInForce(t, catalog, "tenant-a", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, catalogAsOf)
	if resolution.Outcome != ports.PriceCardApplicabilityConflict {
		t.Fatalf("outcome = %s, 想要 APPLICABILITY_CONFLICT", resolution.Outcome)
	}
	if resolution.Plan.Reference().ID() != "" {
		t.Fatalf("冲突不该交回任何一版方案，交回了 %s", resolution.Plan.Reference().ID())
	}
	if len(resolution.Candidates) != 2 ||
		!resolution.Candidates[0].SameIdentity(first.Reference()) ||
		!resolution.Candidates[1].SameIdentity(second.Reference()) {
		t.Fatalf("候选没把两版都列出来：%v", resolution.Candidates)
	}
}

// TestInForcePriceCardTreatsTwoVersionsOfOnePlanAsConflict 证同一方案身份两版同时适用同样是冲突：发布责任方
// 没完成替代关系，本口不替它挑一版（与 LoadApplicable 交回 ErrAmbiguousPriceCard 同一理由，这里按封闭结果答）。
func TestInForcePriceCardTreatsTwoVersionsOfOnePlanAsConflict(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	ctx := t.Context()

	registerCard(t, catalog, transactor, ctx,
		cardRegistration(t, "tenant-a", catalogPlan(t, "plan-buy", "v1", domain.PricingDirectionBuy, "12")))
	registerCard(t, catalog, transactor, ctx,
		cardRegistration(t, "tenant-a", catalogPlan(t, "plan-buy", "v2", domain.PricingDirectionBuy, "13")))

	resolution := resolveInForce(t, catalog, "tenant-a", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, catalogAsOf)
	if resolution.Outcome != ports.PriceCardApplicabilityConflict {
		t.Fatalf("outcome = %s, 想要 APPLICABILITY_CONFLICT", resolution.Outcome)
	}
	if len(resolution.Candidates) != 2 {
		t.Fatalf("候选数 = %d, 想要 2", len(resolution.Candidates))
	}
}

// TestInForcePriceCardRejectsBlankKey 证键缺席是调用方错误，不是「未配置」：空租户或零时点答 error。
func TestInForcePriceCardRejectsBlankKey(t *testing.T) {
	catalog, _ := newPriceCards(t)

	if _, err := catalog.ResolveInForce(t.Context(), domain.TenantID{},
		evaluationValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, catalogAsOf); err == nil {
		t.Fatalf("空租户被接受了")
	}
	if _, err := catalog.ResolveInForce(t.Context(), evaluationValue(t, domain.NewTenantID, "tenant-a"),
		evaluationValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, time.Time{}); err == nil {
		t.Fatalf("零时点被接受了")
	}
}

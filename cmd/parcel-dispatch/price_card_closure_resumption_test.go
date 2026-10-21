package main

import (
	"context"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 16 在生产路由表上证价卡收尾（ADR-0234）接上 ADR-0229 的续办：只有新一版卡解得开的停因（这里是
// 价表上没有这一段），租户登同一方案的新一版——两版同时适用，后继登记那一封续办出来的下一代照实停在适用冲突；再用收尾口
// 把旧版收掉，收尾同笔交的那一封才把它续办到后继上、形成评价与预期成本。各代原样留着。夹具全是 `SYN-` 合成值，证据只记 S。

const bandedPlanID = "SYN-PLAN-BANDED-COST-01"

// bandedBuyCard 造同一方案下的一版 BUY·供应商成本卡：绑分区目录、Z1 一行 [0, maxKilograms] 公斤、声明了金额取整，起点由
// 用例给、无上界。v1 只到 1 公斤，接不住演示包裹申报的 1.2 公斤——那就是要靠新一版卡补上的缺重量段。
func bandedBuyCard(t *testing.T, version string, starts time.Time, maxKilograms string) ppdomain.PriceCardRegistration {
	t.Helper()
	base := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, bandedPlanID, version),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, bandedPlanID+"-TABLE", version),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, bandedPlanID+"-WEIGHT", version),
		Scope:                 "SYN-SCOPE-01",
		Direction:             ppdomain.PricingDirectionBuy,
		Purpose:               ppdomain.PricingPurposeSupplierCost,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
		Period:                pptest.Period{StartsAt: starts},
		RateEntryID:           bandedPlanID + "-Z1-" + version,
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      maxKilograms,
		RateAmount:            "40",
		WeightRounding:        ppdomain.RoundingNone,
		WeightStepKilograms:   "1",
		AmountRounding: &pptest.AmountRoundingSpec{
			Mode: ppdomain.RoundingHalfUp, Increment: "0.01", Points: []ppdomain.AmountRoundingPoint{ppdomain.AmountRoundingTotal},
		},
	})
	link, err := ppdomain.NewReferenceCatalogueLink(ppdomain.CatalogueKindZone, "SYN-CAT-ZONE-CN-SG")
	if err != nil {
		t.Fatal(err)
	}
	structures, err := base.Structures().WithReferenceCatalogues(link)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := ppdomain.NewPricingPlanVersion(base.Reference(), base.Scope(), base.Direction(), base.Purpose(),
		base.BaseChargeCode(), base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), nil, structures)
	if err != nil {
		t.Fatalf("卡 %s：%v", version, err)
	}
	source, err := ppdomain.NewSourceFileIdentity("SYN-"+bandedPlanID+"-"+version+".xlsx", strings.Repeat("cd", 32))
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactCommercialAuthorization, "SYN-AUTH-"+bandedPlanID, "v1")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := ppdomain.NewPriceCardRegistration(ppValue(t, ppdomain.NewTenantID, demoTenant), plan, source,
		authorization, "SYN-PRICING-GOVERNOR-01")
	if err != nil {
		t.Fatalf("价卡登记：%v", err)
	}
	return registration
}

// closeBandedCard 经生产的收尾用例与真交接口收尾 v1（截止或退役），同一笔事务里锁、判、写、交信封。
func closeBandedCard(t *testing.T, db *bentopg.DB, retire bool) ppapplication.PriceCardClosureOutcome {
	t.Helper()
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := ppapplication.NewClosePriceCardVersionHandler(ppapplication.ClosePriceCardVersionDeps{
		Registry: cards, Registered: pricingParameterHandoffForTest(t, db), Clock: systemClock{},
	})
	if err != nil {
		t.Fatal(err)
	}
	target := ppapplication.PriceCardClosureTarget{
		Tenant: ppValue(t, ppdomain.NewTenantID, demoTenant), PlanID: bandedPlanID, Version: "v1", Successor: "v2",
	}
	var result ppapplication.PriceCardClosureResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		var err error
		if retire {
			result, err = handler.RetireAtCoveringSuccessor(ctx, ppapplication.RetirePriceCardVersionCommand{
				PriceCardClosureTarget: target, Reference: "SYN-RETIRE/" + bandedPlanID,
			})
		} else {
			result, err = handler.Cut(ctx, ppapplication.CutPriceCardVersionCommand{
				PriceCardClosureTarget: target, Reference: "SYN-CUT/" + bandedPlanID,
			})
		}
		return err
	})
	if cause := result.Cause(); cause != nil {
		t.Fatalf("收尾 v1 不成立：%v", cause)
	}
	return result.Outcome()
}

// evaluationsUnder 数这一份请求名下按 bandedPlanID 的这一版形成的评价。
func evaluationsUnder(t *testing.T, fixture *synVerticalFixture, request, version string) int {
	t.Helper()
	return fixture.countSQL(t, `SELECT count(*) FROM parcel_pricing.evaluation
		WHERE tenant_id = '`+demoTenant+`' AND request_reference = '`+request+`'
		  AND snapshot->'planReference'->>'id' = '`+bandedPlanID+`' AND snapshot->'planReference'->>'version' = '`+version+`'`)
}

// Covers: AT-PP-012、AT-SA-195——缺重量段那一格只有新一版卡解得开：BUY 请求在 v1 下评价待判断、停在价表上没有这一段；租户
// 登 v2（同一方案、从 6 月起、补上那一段），后继登记那一封续办出第二代，两版在发生项的业务时点同时适用，第二代停在适用冲突；
// 用收尾口把 v1 截止在 v2 的起点（或同起点重发时让 v1 退役），收尾那一封续办出第三代，计价选到 v2 形成评价，结算以第三代
// 派生的版本标识形成预期成本。前两代原样；再来一次同一收尾不出第四代。
func TestARequestStalledOnAMissingWeightBandResumesOnceTheOldCardIsClosed(t *testing.T) {
	for name, closure := range map[string]struct {
		successorStarts time.Time
		retire          bool
	}{
		"截止在后继起点":    {time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), false},
		"同起点重发、旧版退役": {time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true},
	} {
		t.Run(name, func(t *testing.T) {
			fixture, beat, store := buyResumptionFixture(t)
			registerBuyTriggers(t, fixture.db, demoTenant)
			registerPriceCardForTest(t, fixture.db, bandedBuyCard(t, "v1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "1"))
			drainBeats(t, beat)
			insertChargeOccurrence(t, fixture.db, demoTenant, "SYN-OCC-BANDED", tfdomain.ActualFulfillmentOccurrence, trunkAgreementObject+"/v1")
			enqueueOccurrenceFormed(t, fixture.db, store, "occ-banded", "SYN-OCC-BANDED")
			drainBeats(t, beat)
			if stalled := buyRequestsOf(t, fixture.db, "SYN-OCC-BANDED"); len(stalled) != 1 || stalled[0].stopCause != "EVALUATION_RATE_NOT_FOUND" {
				t.Fatalf("v1 下的 BUY 请求册 = %+v，want 一份第一代、停在 EVALUATION_RATE_NOT_FOUND", stalled)
			}

			registerPriceCardForTest(t, fixture.db, bandedBuyCard(t, "v2", closure.successorStarts, "30"))
			drainBeats(t, beat)
			conflicted := buyRequestsOf(t, fixture.db, "SYN-OCC-BANDED")
			if len(conflicted) != 2 || conflicted[1].previous != conflicted[0].id ||
				!strings.HasPrefix(conflicted[1].basis, "PARAMREG/PRICE_CARD/") || conflicted[1].stopCause != "PRICE_CARD_APPLICABILITY_CONFLICT" {
				t.Fatalf("登了 v2 之后 = %+v，want 凭 v2 的登记续办出第二代、两版同时适用停在适用冲突", conflicted)
			}
			if versions := supplierCostVersions(t, fixture.db); len(versions) != 0 {
				t.Fatalf("收尾之前就形成了成本：%v", versions)
			}

			if outcome := closeBandedCard(t, fixture.db, closure.retire); outcome != ppapplication.PriceCardClosureRecorded {
				t.Fatalf("收尾 v1 = %s", outcome)
			}
			drainBeats(t, beat)
			lineage := buyRequestsOf(t, fixture.db, "SYN-OCC-BANDED")
			if len(lineage) != 3 {
				t.Fatalf("收尾之后的 BUY 请求册 = %+v，want 三代", lineage)
			}
			if lineage[0] != conflicted[0] || lineage[1] != conflicted[1] {
				t.Fatalf("前两代被改写：%+v", lineage[:2])
			}
			third := lineage[2]
			if third.previous != lineage[1].id || third.stopCause != "" || !strings.HasPrefix(third.basis, "PARAMREG/PRICE_CARD/") ||
				third.basis == lineage[1].basis {
				t.Fatalf("第三代 = %+v，want 回指第二代、凭收尾那一封续办、没有停因", third)
			}
			if versions := supplierCostVersions(t, fixture.db); len(versions) != 1 || versions[0] != "BUYSC-"+third.id {
				t.Fatalf("预期成本版本 = %v，want 恰一份 BUYSC-%s", versions, third.id)
			}
			if n := evaluationsUnder(t, fixture, third.id, "v2"); n != 1 {
				t.Fatalf("第三代名下按 v2 形成的评价 %d 份，want 1", n)
			}
			if n := evaluationsUnder(t, fixture, lineage[0].id, "v1"); n != 1 {
				t.Fatalf("第一代名下按 v1 形成的那一份评价剩 %d 份，want 原样 1 份", n)
			}

			if outcome := closeBandedCard(t, fixture.db, closure.retire); outcome != ppapplication.PriceCardClosureAlreadyOnRegister {
				t.Fatalf("重放收尾 v1 = %s", outcome)
			}
			drainBeats(t, beat)
			if again := buyRequestsOf(t, fixture.db, "SYN-OCC-BANDED"); len(again) != 3 {
				t.Fatalf("同一收尾重放又续办了：%+v", again)
			}
			assertNothingAbandoned(t, fixture.db,
				"settlement-accounting.evaluation-request.submitted", "parcel-pricing.evaluation-request.unformed",
				"parcel-pricing.pricing-parameter.registered", "parcel-pricing.evaluation.recorded")
		})
	}
}

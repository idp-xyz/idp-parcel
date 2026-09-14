package parcelpricing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件证 SA-c 缝的消费侧适配器（票 pricing-amount-precision/02 第 3 项；ADR-0107 Decision 五）：金额从十进制到
// 最小币单位只按评价取整留痕里的进位单位换写，未声明取整策略的评价拒而不补取整；方向不对拒；四种非完成结果
// 逐格译；票级评价的主体种类与成员清单如实带过来（ADR-0111 Decision 三）。夹具全为 SYN 卡（S 级）。

type evaluationSourceDouble struct {
	byID map[string]ppdomain.PricingEvaluation
}

func (double *evaluationSourceDouble) FindByID(_ context.Context, id ppdomain.EvaluationID) (ppdomain.PricingEvaluation, bool, error) {
	evaluation, found := double.byID[id.String()]
	return evaluation, found, nil
}

// must 是夹具的统一失败出口：`must[T](t)(f())` 让返回 (T, error) 的构造器直接喂进去。
func must[T any](t *testing.T) func(T, error) T {
	return func(value T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatalf("fixture: %v", err)
		}
		return value
	}
}

func decimal(t *testing.T, raw string) ppdomain.Decimal {
	return must[ppdomain.Decimal](t)(ppdomain.ParseDecimal(raw))
}

func kilograms(t *testing.T, raw string) ppdomain.Weight {
	return must[ppdomain.Weight](t)(ppdomain.NewWeight(decimal(t, raw), ppdomain.WeightUnitKilogram))
}

func scope(t *testing.T) ppdomain.PricingScopeID {
	return must[ppdomain.PricingScopeID](t)(ppdomain.NewPricingScopeID("scope-1"))
}

func ppTenant(t *testing.T, tenant string) ppdomain.TenantID {
	return must[ppdomain.TenantID](t)(ppdomain.NewTenantID(tenant))
}

func packageID(t *testing.T, id string) ppdomain.PackageID {
	return must[ppdomain.PackageID](t)(ppdomain.NewPackageID(id))
}

var basisAt = time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

// buyPlan 造一张 BUY·SUPPLIER_COST 的 SYN 卡：USD 价表 12.5，可选的金额取整策略（合计 HALF_UP 到 0.01）。
// 构造走 PP 的测试专属夹具 pptest，值全部在这里显式给出（票 sa-cc/17）。
func buyPlan(t *testing.T, direction ppdomain.PricingDirection, purpose ppdomain.PricingPurpose, rounded bool) ppdomain.PricingPlanVersion {
	t.Helper()
	var amountRounding *pptest.AmountRoundingSpec
	if rounded {
		amountRounding = &pptest.AmountRoundingSpec{
			Mode:      ppdomain.RoundingHalfUp,
			Increment: "0.01",
			Points:    []ppdomain.AmountRoundingPoint{ppdomain.AmountRoundingTotal},
		}
	}
	return pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-BUY-CARD", "v2"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, "table", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, "weight", "v1"),
		Scope:                 "scope-1",
		Direction:             direction,
		Purpose:               purpose,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "USD",
		Period: pptest.Period{
			StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndsAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		RateEntryID:         "entry",
		RateZone:            "Z1",
		MinimumKilograms:    "0",
		MaximumKilograms:    "10",
		RateAmount:          "12.5",
		WeightRounding:      ppdomain.RoundingNone,
		WeightStepKilograms: "1",
		AmountRounding:      amountRounding,
	})
}

func packageInput(t *testing.T, tenant, zone string) ppdomain.PricingInputSnapshot {
	t.Helper()
	return pptest.Input(t, pptest.InputSpec{
		Tenant:     tenant,
		Scope:      "scope-1",
		PackageID:  "pkg-1",
		Zone:       zone,
		Kilograms:  "5",
		BusinessAt: basisAt,
	})
}

func evaluate(t *testing.T, id string, plan ppdomain.PricingPlanVersion, input ppdomain.PricingInputSnapshot) ppdomain.PricingEvaluation {
	t.Helper()
	return pptest.Evaluate(t, id, plan, input)
}

func newAdapter(t *testing.T, evaluations ...ppdomain.PricingEvaluation) *adapter.BuyEvaluationAdapter {
	t.Helper()
	source := &evaluationSourceDouble{byID: map[string]ppdomain.PricingEvaluation{}}
	for _, evaluation := range evaluations {
		source.byID[evaluation.ID().String()] = evaluation
	}
	return must[*adapter.BuyEvaluationAdapter](t)(adapter.NewBuyEvaluationAdapter(source))
}

func load(t *testing.T, view saports.BuyEvaluationView, tenant, id string) (saports.BuyEvaluationAdoption, bool, error) {
	t.Helper()
	saTenant := must[sadomain.TenantID](t)(sadomain.NewTenantID(tenant))
	evaluationReference := must[sadomain.BuyEvaluationReference](t)(sadomain.NewBuyEvaluationReference(id))
	return view.LoadBuyEvaluation(context.Background(), saTenant, evaluationReference)
}

// Covers: ADR-0107 Decision 五「声明了策略的卡算出的合计 scale 就是进位单位的 scale，消费方直接采用」——12.5 USD
// 按合计 0.01 的留痕换写成 1250 分，不做任何算术。
func TestCompletedEvaluationIsAdoptedAtTheDeclaredIncrementScale(t *testing.T) {
	evaluation := evaluate(t, "eval-buy-1", buyPlan(t, ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost, true), packageInput(t, "tenant-1", "Z1"))
	if evaluation.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("fixture status = %s %#v", evaluation.Status(), evaluation.Issues())
	}
	adoption, found, err := load(t, newAdapter(t, evaluation), "tenant-1", "eval-buy-1")
	if err != nil || !found {
		t.Fatalf("load: found=%v err=%v", found, err)
	}
	if adoption.Outcome != saports.BuyEvaluationCompleted || adoption.SettlementMinor != 1250 || adoption.SettlementCurrency.String() != "USD" {
		t.Fatalf("adoption = %#v", adoption)
	}
	if adoption.OriginalMinor != 1250 || adoption.OriginalCurrency.String() != "USD" || adoption.Conversion != (sadomain.ConversionStepReference{}) {
		t.Fatalf("same-currency evaluation carries a conversion: %#v", adoption)
	}
	if adoption.RuleVersion.String() != "SYN-BUY-CARD@v2" || adoption.SubjectKind != "ACCEPTED_PACKAGE" || len(adoption.MemberPackages) != 0 {
		t.Fatalf("rule / subject = %#v", adoption)
	}
}

// Covers: ADR-0107 Decision 五「未声明的卡……不得自己补一次取整」——评价带 AMOUNT_PRECISION_UNDECLARED 时拒，不给
// 一个凑出来的分值。
func TestUndeclaredPrecisionIsRefusedNotRounded(t *testing.T) {
	evaluation := evaluate(t, "eval-buy-undeclared", buyPlan(t, ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost, false), packageInput(t, "tenant-1", "Z1"))
	_, _, err := load(t, newAdapter(t, evaluation), "tenant-1", "eval-buy-undeclared")
	if !errors.Is(err, adapter.ErrAmountPrecisionUndeclared) {
		t.Fatalf("err = %v, want ErrAmountPrecisionUndeclared", err)
	}
}

// Covers: 缝的裁定「方向 / 目的不是 BUY·SUPPLIER_COST 拒」；别的租户的评价对本租户不存在；四种非完成结果逐格译。
func TestBoundariesOfTheBuyEvaluationSeam(t *testing.T) {
	sell := evaluate(t, "eval-sell", buyPlan(t, ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, true), packageInput(t, "tenant-1", "Z1"))
	if _, _, err := load(t, newAdapter(t, sell), "tenant-1", "eval-sell"); !errors.Is(err, adapter.ErrNotABuyEvaluation) {
		t.Fatalf("sell evaluation accepted: %v", err)
	}

	buy := evaluate(t, "eval-buy-2", buyPlan(t, ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost, true), packageInput(t, "tenant-1", "Z1"))
	if _, found, err := load(t, newAdapter(t, buy), "tenant-2", "eval-buy-2"); err != nil || found {
		t.Fatalf("another tenant's evaluation was found: found=%v err=%v", found, err)
	}
	if _, found, err := load(t, newAdapter(t), "tenant-1", "eval-missing"); err != nil || found {
		t.Fatalf("missing evaluation: found=%v err=%v", found, err)
	}

	// 分区 Z9 不在价表里：评价待判断（RATE_NOT_FOUND），SA 那一格是待判断，不带金额。
	pending := evaluate(t, "eval-buy-pending", buyPlan(t, ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost, true), packageInput(t, "tenant-1", "Z9"))
	if pending.Status() != ppdomain.EvaluationPending {
		t.Fatalf("fixture status = %s", pending.Status())
	}
	adoption, found, err := load(t, newAdapter(t, pending), "tenant-1", "eval-buy-pending")
	if err != nil || !found || adoption.Outcome != saports.BuyEvaluationPending || adoption.SettlementMinor != 0 {
		t.Fatalf("pending adoption = %#v found=%v err=%v", adoption, found, err)
	}
}

// Covers: ADR-0111 Decision 三——票级评价交过来时主体种类与成员清单如实在场，金额不摊。
func TestShipmentLevelEvaluationCarriesItsMembersToSettlement(t *testing.T) {
	base := buyPlan(t, ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost, true)
	plan := must[ppdomain.PricingPlanVersion](t)(ppdomain.NewAggregatePricingPlanVersion(ppdomain.AggregationPerShipment,
		base.Reference(), base.Scope(), base.Direction(), base.Purpose(), base.BaseChargeCode(), base.EffectivePeriod(),
		base.RateTable(), base.WeightPolicy(), nil, base.Structures()))
	manifest := must[ppdomain.MemberManifest](t)(ppdomain.NewMemberManifest(
		[]ppdomain.PackageID{packageID(t, "pkg-1"), packageID(t, "pkg-2")}, kilograms(t, "7"), nil))
	subject := must[ppdomain.EvaluationSubject](t)(ppdomain.NewShipmentSubject("SR-1"))
	input := must[ppdomain.PricingInputSnapshot](t)(ppdomain.NewAggregatePricingInputSnapshot(ppTenant(t, "tenant-1"), scope(t), subject, "Z1", manifest, basisAt))
	evaluation := evaluate(t, "eval-buy-shipment", plan, input)
	if evaluation.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("fixture status = %s %#v", evaluation.Status(), evaluation.Issues())
	}
	adoption, found, err := load(t, newAdapter(t, evaluation), "tenant-1", "eval-buy-shipment")
	if err != nil || !found {
		t.Fatalf("load: found=%v err=%v", found, err)
	}
	if adoption.SubjectKind != "SHIPMENT" || len(adoption.MemberPackages) != 2 || adoption.MemberPackages[0] != "pkg-1" || adoption.SettlementCurrency.String() != "USD" {
		t.Fatalf("adoption = %#v", adoption)
	}
}

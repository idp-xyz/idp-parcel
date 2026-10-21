package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 本文件证价卡收尾编排（ADR-0234）交「参数已登记」的那一半：落库或重放认出之后，按旧版的计价范围与方向交一封价卡种类的
// 信封，带收尾的身份；不成立的收尾不交；内部方向的卡不交；交不出整笔失败；缺交接口构造即拒。落库与选卡的口径由
// adapters/postgres 的真库用例证。

// closureRegistryDouble 是只在内存里的收尾册：一版一个此刻的样子，写入只记下。
type closureRegistryDouble struct {
	standings map[string]domain.PriceCardStanding
	cuts      []domain.PriceCardCut
	locked    []string
}

func (double *closureRegistryDouble) LockPriceCardVersionForClosure(
	_ context.Context, _ domain.TenantID, plan domain.VersionReference,
) (bool, error) {
	double.locked = append(double.locked, plan.Version())
	_, found := double.standings[plan.Version()]
	return found, nil
}

func (double *closureRegistryDouble) LoadPriceCardStanding(
	_ context.Context, _ domain.TenantID, plan domain.VersionReference,
) (domain.PriceCardStanding, bool, error) {
	standing, found := double.standings[plan.Version()]
	return standing, found, nil
}

func (double *closureRegistryDouble) SavePriceCardCut(_ context.Context, cut domain.PriceCardCut) (ports.PriceCardClosureSaveOutcome, error) {
	double.cuts = append(double.cuts, cut)
	return ports.PriceCardClosureSaved, nil
}

func (double *closureRegistryDouble) SavePriceCardRetirement(
	_ context.Context, _ domain.PriceCardRetirement,
) (ports.PriceCardClosureSaveOutcome, error) {
	return ports.PriceCardClosureSaved, nil
}

func closureCardStanding(t *testing.T, version string, direction domain.PricingDirection, starts time.Time) domain.PriceCardStanding {
	t.Helper()
	purpose := domain.PricingPurposeSupplierCost
	if direction == domain.PricingDirectionInternal {
		purpose = domain.PricingPurposeInternalPrice
	}
	plan := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-PLAN-CLOSE", version),
		TableReference:        pptest.IdentityReference(t, domain.ArtifactRateTable, "SYN-TABLE-CLOSE", version),
		WeightPolicyReference: pptest.IdentityReference(t, domain.ArtifactWeightPolicy, "SYN-WEIGHT-CLOSE", version),
		Scope:                 "SYN-SCOPE-CLOSE",
		Direction:             direction,
		Purpose:               purpose,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
		Period:                pptest.Period{StartsAt: starts, EndsAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		RateEntryID:           "SYN-CLOSE-Z1-" + version,
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            "10",
		WeightRounding:        domain.RoundingNone,
		WeightStepKilograms:   "1",
	})
	standing, err := domain.NewPriceCardStanding(mustValue(t, domain.NewTenantID, "SYN-TENANT-01"), plan, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return standing
}

func closureHandler(t *testing.T, direction domain.PricingDirection, handoff *pricingParameterDouble) (*application.ClosePriceCardVersionHandler, *closureRegistryDouble) {
	t.Helper()
	registry := &closureRegistryDouble{standings: map[string]domain.PriceCardStanding{
		"v1": closureCardStanding(t, "v1", direction, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		"v2": closureCardStanding(t, "v2", direction, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)),
	}}
	handler, err := application.NewClosePriceCardVersionHandler(application.ClosePriceCardVersionDeps{
		Registry: registry, Registered: handoff, Clock: fixedClock{at: time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler, registry
}

func closureCommand(t *testing.T, successor, reference string) application.CutPriceCardVersionCommand {
	t.Helper()
	return application.CutPriceCardVersionCommand{
		PriceCardClosureTarget: application.PriceCardClosureTarget{
			Tenant: mustValue(t, domain.NewTenantID, "SYN-TENANT-01"), PlanID: "SYN-PLAN-CLOSE", Version: "v1", Successor: successor,
		},
		Reference: reference,
	}
}

// Covers: AT-PP-012 的计价半边——截止落库同笔交一封「参数已登记」：价卡种类、旧版那一版、旧版的计价范围与方向，带收尾的
// 种类、后继与依据；先锁的是旧版那一行。不成立的收尾不交。
func TestAPriceCardCutAnnouncesTheScopeItHandsToTheSuccessor(t *testing.T) {
	handoff := &pricingParameterDouble{}
	handler, registry := closureHandler(t, domain.PricingDirectionBuy, handoff)

	result, err := handler.Cut(t.Context(), closureCommand(t, "v2", "SYN-CUT-V2"))
	if err != nil || result.Outcome() != application.PriceCardClosureRecorded {
		t.Fatalf("截止 = %s（%v）err=%v", result.Outcome(), result.Cause(), err)
	}
	if len(registry.locked) != 1 || registry.locked[0] != "v1" || len(registry.cuts) != 1 {
		t.Fatalf("锁了 %v、写了 %d 条，want 先锁旧版、写一条", registry.locked, len(registry.cuts))
	}
	if len(handoff.intents) != 1 {
		t.Fatalf("交了 %d 封，want 1", len(handoff.intents))
	}
	intent := handoff.intents[0]
	if intent.Kind != ports.PricingParameterPriceCard || intent.Registration.Version() != "v1" ||
		intent.Scope.String() != "SYN-SCOPE-CLOSE" || intent.Direction != domain.PricingDirectionBuy ||
		intent.Closure != (ports.PriceCardClosureOccasion{Kind: ports.PriceCardCutClosure, Successor: "v2", Reference: "SYN-CUT-V2"}) {
		t.Fatalf("交出的信封 = %+v", intent)
	}

	refused, err := handler.Cut(t.Context(), closureCommand(t, "v9", "SYN-CUT-V9"))
	if err != nil || refused.Outcome() != application.PriceCardClosureNotAccepted ||
		!errors.Is(refused.Cause(), domain.ErrPriceCardClosureVersionNotRegistered) || len(handoff.intents) != 1 {
		t.Fatalf("后继不在册 = %s（%v）err=%v，交了 %d 封", refused.Outcome(), refused.Cause(), err, len(handoff.intents))
	}
}

// Covers: AT-PP-012 的计价半边——内部方向的卡收尾不交信封（结算的评价请求只有 BUY 与 SELL 两路）；交不出整笔失败，不答
// 已登记；缺交接口构造即拒。
func TestAPriceCardClosureAnnouncementFollowsTheRegistrationRules(t *testing.T) {
	internal := &pricingParameterDouble{}
	handler, _ := closureHandler(t, domain.PricingDirectionInternal, internal)
	if result, err := handler.Cut(t.Context(), closureCommand(t, "v2", "SYN-CUT-V2")); err != nil ||
		result.Outcome() != application.PriceCardClosureRecorded || len(internal.intents) != 0 {
		t.Fatalf("内部方向 = %s err=%v，交了 %d 封", result.Outcome(), err, len(internal.intents))
	}

	failing := &pricingParameterDouble{err: errors.New("outbox down")}
	handler, _ = closureHandler(t, domain.PricingDirectionBuy, failing)
	if _, err := handler.Cut(t.Context(), closureCommand(t, "v2", "SYN-CUT-V2")); err == nil {
		t.Fatal("交不出信封却答了已登记")
	}

	if _, err := application.NewClosePriceCardVersionHandler(application.ClosePriceCardVersionDeps{
		Registry: &closureRegistryDouble{}, Clock: fixedClock{},
	}); !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("缺交接口 err = %v，want ErrNilDependency", err)
	}
}

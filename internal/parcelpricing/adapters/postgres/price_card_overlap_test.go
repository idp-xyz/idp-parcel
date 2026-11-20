package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件对真实 PostgreSQL 证：重叠提示折截止与退役的读法与 ResolveInForce 同一套，提示里的版本集合与交集内的适用冲突候选一致。
// 夹具全是合成价卡，证据只记 S。

var (
	overlapJanuary = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	overlapJune    = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	overlapJuly    = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
)

func overlapTenant(t *testing.T) domain.TenantID {
	t.Helper()
	return evaluationValue(t, domain.NewTenantID, "tenant-a")
}

func registerOverlapCard(t *testing.T, catalog *adapter.PriceCards, transactor bentoapp.Transactor, plan domain.PricingPlanVersion) {
	t.Helper()
	if outcome := registerCard(t, catalog, transactor, t.Context(), cardRegistration(t, "tenant-a", plan)); outcome != ports.PriceCardRegistered {
		t.Fatalf("登记 %s/%s = %d", plan.Reference().ID(), plan.Reference().Version(), outcome)
	}
}

func findOverlaps(t *testing.T, catalog *adapter.PriceCards, plan domain.PricingPlanVersion) []domain.PriceCardOverlap {
	t.Helper()
	overlaps, err := catalog.FindOverlappingInForce(
		t.Context(), overlapTenant(t), plan.Reference(), plan.Scope(), plan.Direction(), plan.Purpose(),
		plan.EffectivePeriod(), plan.ServiceProducts(),
	)
	if err != nil {
		t.Fatalf("查重叠：%v", err)
	}
	if overlaps == nil {
		t.Fatal("没有重叠时应交回空切片，不是 nil")
	}
	return overlaps
}

func overlapByVersion(overlaps []domain.PriceCardOverlap, version string) (domain.PriceCardOverlap, bool) {
	for _, item := range overlaps {
		if item.Plan().Version() == version {
			return item, true
		}
	}
	return domain.PriceCardOverlap{}, false
}

func TestOverlappingInForceAgreesWithResolveInForceUntilThePredecessorIsCut(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	v1 := catalogPlanWithPeriod(t, "SYN-PLAN-OVERLAP", "v1", domain.PricingDirectionSell, "5.00", overlapJanuary, time.Time{})
	v2 := catalogPlanWithPeriod(t, "SYN-PLAN-OVERLAP", "v2", domain.PricingDirectionSell, "6.00", overlapJune, time.Time{})
	registerOverlapCard(t, catalog, transactor, v1)

	overlaps := findOverlaps(t, catalog, v2)
	item, ok := overlapByVersion(overlaps, "v1")
	if !ok || len(overlaps) != 1 || !item.SamePlan() || item.Resolution() != domain.PriceCardOverlapSuccessorCut ||
		!item.Overlap().StartsAt().Equal(overlapJune) || !item.Overlap().EndsAt().IsZero() ||
		!item.Plan().SameIdentity(v1.Reference()) {
		t.Fatalf("v2 预览应只提示 v1 后继截止，得到 %#v", overlaps)
	}

	registerOverlapCard(t, catalog, transactor, v2)
	resolution := resolveInForce(t, catalog, "tenant-a", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, overlapJuly)
	if resolution.Outcome != ports.PriceCardApplicabilityConflict || len(resolution.Candidates) != 2 ||
		!resolution.Candidates[0].SameIdentity(v1.Reference()) || !resolution.Candidates[1].SameIdentity(v2.Reference()) {
		t.Fatalf("交集内应恰好是 {v1, v2}：%s %v", resolution.Outcome, resolution.Candidates)
	}

	cutPredecessor(t, catalog, transactor, v1, v2)
	if _, still := overlapByVersion(findOverlaps(t, catalog, v2), "v1"); still {
		t.Fatal("v1 已截止到 v2 起点，不该再出现在重叠里")
	}
	v3 := catalogPlanWithPeriod(t, "SYN-PLAN-OVERLAP", "v3", domain.PricingDirectionSell, "7.00", overlapJune, time.Time{})
	if _, still := overlapByVersion(findOverlaps(t, catalog, v3), "v1"); still {
		t.Fatal("同起点的新卡也不该再看见已截止的 v1")
	}
}

func TestRetiredPredecessorIsAbsentFromOverlaps(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	predecessor := catalogPlanWithPeriod(t, "SYN-PLAN-RETIRE", "v1", domain.PricingDirectionSell, "5.00", overlapJune, time.Time{})
	successor := catalogPlanWithPeriod(t, "SYN-PLAN-RETIRE", "v2", domain.PricingDirectionSell, "6.00", overlapJanuary, time.Time{})
	registerOverlapCard(t, catalog, transactor, predecessor)
	registerOverlapCard(t, catalog, transactor, successor)
	retirePredecessor(t, catalog, transactor, predecessor, successor)

	candidate := catalogPlanWithPeriod(t, "SYN-PLAN-RETIRE", "v3", domain.PricingDirectionSell, "7.00", overlapJune, time.Time{})
	if _, still := overlapByVersion(findOverlaps(t, catalog, candidate), "v1"); still {
		t.Fatal("已退役的 v1 不该出现在重叠里")
	}
}

func TestNamedAndUnnamedPriceCardsDoNotOverlapEachOther(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	fba := named(t, catalogPlanWithPeriod(t, "plan-fba", "v1", domain.PricingDirectionSell, "10", overlapJanuary, time.Time{}), productFBA)
	express := catalogPlanWithPeriod(t, "plan-express", "v1", domain.PricingDirectionSell, "11", overlapJanuary, time.Time{})
	registerOverlapCard(t, catalog, transactor, fba)
	registerOverlapCard(t, catalog, transactor, express)

	nextFBA := named(t, catalogPlanWithPeriod(t, "plan-fba-next", "v1", domain.PricingDirectionSell, "12", overlapJune, time.Time{}), productFBA)
	fbaOverlaps := findOverlaps(t, catalog, nextFBA)
	if _, seen := overlapByVersion(fbaOverlaps, "v1"); len(fbaOverlaps) != 1 || !seen || fbaOverlaps[0].Plan().ID() != "plan-fba" {
		t.Fatalf("指名 FBA 的卡不该报出不声明产品的快递卡：%v", planIDs(fbaOverlaps))
	}
	nextExpress := catalogPlanWithPeriod(t, "plan-express-next", "v1", domain.PricingDirectionSell, "13", overlapJune, time.Time{})
	expressOverlaps := findOverlaps(t, catalog, nextExpress)
	if _, seen := overlapByVersion(expressOverlaps, "v1"); len(expressOverlaps) != 1 || !seen || expressOverlaps[0].Plan().ID() != "plan-express" {
		t.Fatalf("不声明产品的卡不该报出 FBA 卡：%v", planIDs(expressOverlaps))
	}

	tenant := overlapTenant(t)
	scope := fba.Scope()
	fbaHit, err := catalog.ResolveInForce(t.Context(), tenant, scope, domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, overlapJuly, productOf(t, productFBA))
	if err != nil {
		t.Fatal(err)
	}
	if fbaHit.Outcome != ports.PriceCardVersionInForce || !fbaHit.Plan.Reference().SameIdentity(fba.Reference()) {
		t.Fatalf("申报 FBA 应唯一命中 FBA 卡：%s %s", fbaHit.Outcome, fbaHit.Plan.Reference().ID())
	}
	expressHit, err := catalog.ResolveInForce(t.Context(), tenant, scope, domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, overlapJuly, domain.ServiceProductReference{})
	if err != nil {
		t.Fatal(err)
	}
	if expressHit.Outcome != ports.PriceCardVersionInForce || !expressHit.Plan.Reference().SameIdentity(express.Reference()) {
		t.Fatalf("不申报产品应唯一命中快递卡：%s %s", expressHit.Outcome, expressHit.Plan.Reference().ID())
	}
}

func planIDs(overlaps []domain.PriceCardOverlap) []string {
	ids := make([]string, 0, len(overlaps))
	for _, item := range overlaps {
		ids = append(ids, item.Plan().ID()+"@"+item.Plan().Version())
	}
	return ids
}

func cutPredecessor(t *testing.T, catalog *adapter.PriceCards, transactor bentoapp.Transactor, predecessor, successor domain.PricingPlanVersion) {
	t.Helper()
	closeForSuccessor(t, catalog, transactor, predecessor, successor, true)
}

func retirePredecessor(t *testing.T, catalog *adapter.PriceCards, transactor bentoapp.Transactor, predecessor, successor domain.PricingPlanVersion) {
	t.Helper()
	closeForSuccessor(t, catalog, transactor, predecessor, successor, false)
}

func closeForSuccessor(t *testing.T, catalog *adapter.PriceCards, transactor bentoapp.Transactor, predecessor, successor domain.PricingPlanVersion, cut bool) {
	t.Helper()
	tenant := overlapTenant(t)
	old, oldOK, err := catalog.LoadPriceCardStanding(t.Context(), tenant, predecessor.Reference())
	if err != nil || !oldOK {
		t.Fatalf("读旧版：ok=%v err=%v", oldOK, err)
	}
	next, nextOK, err := catalog.LoadPriceCardStanding(t.Context(), tenant, successor.Reference())
	if err != nil || !nextOK {
		t.Fatalf("读后继：ok=%v err=%v", nextOK, err)
	}
	reference, err := domain.NewPriceCardClosureReference("SYN-OVERLAP-CLOSE")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	inCatalogTx(t, transactor, t.Context(), func(ctx context.Context) error {
		if cut {
			decision, replay, cutErr := old.CutForSuccessor(next, reference, at)
			if cutErr != nil || replay {
				return fmt.Errorf("截止：replay=%v err=%w", replay, cutErr)
			}
			outcome, saveErr := catalog.SavePriceCardCut(ctx, decision)
			if saveErr != nil || outcome != ports.PriceCardClosureSaved {
				return fmt.Errorf("落下截止：%d %w", outcome, saveErr)
			}
			return nil
		}
		decision, replay, retireErr := old.RetireForCoveringSuccessor(next, reference, at)
		if retireErr != nil || replay {
			return fmt.Errorf("退役：replay=%v err=%w", replay, retireErr)
		}
		outcome, saveErr := catalog.SavePriceCardRetirement(ctx, decision)
		if saveErr != nil || outcome != ports.PriceCardClosureSaved {
			return fmt.Errorf("落下退役：%d %w", outcome, saveErr)
		}
		return nil
	})
}

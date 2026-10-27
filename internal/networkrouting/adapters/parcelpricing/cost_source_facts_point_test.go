package parcelpricing_test

import (
	"context"
	"errors"
	"testing"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelpricing"
)

// 本文件钉候选出价所处的判断点怎样改变出价（CONTEXT「预路由使用客户声明快照；首次收寄复核使用当时已经取得的物理事实，到站
// 复核使用当前有效物理实测」）：同一件包裹、同两张价卡，声明与实测不符时，候选的成本事实随判断点所取的事实而变，成本出处
// 标明依据；定不下用哪条实测时各段待判断，不退回声明。

// tieredBuyPlan 立一张分档采购价卡：每个 [min, max] 重量档一个 Z1 价。
func tieredBuyPlan(t *testing.T, id string, tiers ...[3]string) ppdomain.PricingPlanVersion {
	t.Helper()
	plain := syntheticBuyPlan(t, id, "v1", "1")
	entries := make([]ppdomain.RateEntry, 0, len(tiers))
	for index, tier := range tiers {
		money, err := ppdomain.NewMoneyFromString(tier[2], value(t, ppdomain.NewCurrency, "USD"))
		if err != nil {
			t.Fatalf("amount: %v", err)
		}
		entry, err := ppdomain.NewRateEntry(
			value(t, ppdomain.NewRateEntryID, "entry-"+id+"-"+string(rune('a'+index))), "Z1", weightOf(t, tier[0]), weightOf(t, tier[1]), money)
		if err != nil {
			t.Fatalf("rate entry: %v", err)
		}
		entries = append(entries, entry)
	}
	table, err := ppdomain.NewRateTableVersion(
		mustReference(t, ppdomain.ArtifactRateTable, "table-"+id, "v1"),
		ppdomain.RateTableFamilyWeightZone, value(t, ppdomain.NewCurrency, "USD"), ppdomain.WeightUnitKilogram,
		plain.EffectivePeriod(), entries)
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	return rateTablePlanWith(t, id, "v1", ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost,
		table, plain.WeightPolicy(), plain.EffectivePeriod(), ppdomain.PricingPlanStructures{})
}

// twoCandidatesCheapUntilHeavy 立两张候选的价卡：candidate "light" 在 10kg 内便宜、超过 10kg 贵得多；"flat" 全档同价居中。
// 声明 1kg 时 light 最便宜；实测 12kg 时 flat 最便宜——选中的候选随判断点所取的事实而变。
func twoCandidatesCheapUntilHeavy(t *testing.T) *testSource {
	t.Helper()
	return newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-light": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-light/v1"}},
			"line-flat":  {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-flat/v1"}},
		},
		map[string]ppdomain.PricingPlanVersion{
			"plan-light/v1": tieredBuyPlan(t, "plan-light", [3]string{"0", "10", "1"}, [3]string{"10", "30", "50"}),
			"plan-flat/v1":  tieredBuyPlan(t, "plan-flat", [3]string{"0", "30", "3"}),
		},
	)
}

func cheapest(t *testing.T, costs nrports.RouteCandidateCosts) string {
	t.Helper()
	var name string
	var least int64 = -1
	for _, line := range []string{"line-light", "line-flat"} {
		fact := factOf(t, costs.Facts, line)
		if fact.State() != nrdomain.CandidateCostPriced {
			t.Fatalf("%s = %s，want 已计价", line, fact.State())
		}
		if least < 0 || fact.AmountMinor() < least {
			name, least = line, fact.AmountMinor()
		}
	}
	return name
}

func loadAt(t *testing.T, source *testSource, point nrdomain.ParcelFactsPoint) nrports.RouteCandidateCosts {
	t.Helper()
	source.askedPoint = point
	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
		pathOf(t, "line-light", 1), pathOf(t, "line-flat", 1)))
	if err != nil {
		t.Fatalf("取候选成本：%v", err)
	}
	return costs
}

// Covers: 判断点改变选中的候选（判据 4）——声明 1kg、实测 12kg：预路由按声明，light 最便宜；首次收寄复核与到站复核按实测，
// light 落进 10kg 以上的贵档，flat 最便宜。成本出处与成本族都标明依据是物理事实。
func TestTheJudgmentPointChangesWhichCandidateIsCheapest(t *testing.T) {
	source := twoCandidatesCheapUntilHeavy(t)
	source.pointed = pointedFacts(t, declaredFacts(t, "1", false), &physicalDouble{answer: obtained(t, "12", nil)})

	preRoute := loadAt(t, source, nrdomain.NewPreRouteFactsPoint())
	if got := cheapest(t, preRoute); got != "line-light" || preRoute.FactsBasis != nrdomain.ParcelFactsDeclaration {
		t.Fatalf("预路由：最便宜 %s、依据 %s，want 按声明的 line-light", got, preRoute.FactsBasis)
	}
	for name, point := range map[string]nrdomain.ParcelFactsPoint{
		"首次收寄": firstIntake(t),
		"到站":   nrdomain.NewArrivalFactsPoint(),
		"授权决定": nrdomain.NewDecisionFactsPoint(),
	} {
		costs := loadAt(t, source, point)
		if got := cheapest(t, costs); got != "line-flat" {
			t.Fatalf("%s：最便宜 %s，want 按实测 12kg 的 line-flat", name, got)
		}
		if costs.FactsBasis != nrdomain.ParcelFactsPhysical {
			t.Fatalf("%s：依据 %s，want PHYSICAL", name, costs.FactsBasis)
		}
		if len(costs.Citations) != 2 {
			t.Fatalf("%s：出处 %d 条，want 两候选各一段", name, len(costs.Citations))
		}
		for _, citation := range costs.Citations {
			if citation.FactsBasis() != nrdomain.ParcelFactsPhysical {
				t.Fatalf("%s：出处依据 %s，want PHYSICAL", name, citation.FactsBasis())
			}
		}
	}
}

// Covers: 物理事实没有时退回声明出价、排序照常——与预路由同价，成本族与出处如实标退回。
func TestAFallbackPricesLikeTheDeclarationAndSaysSo(t *testing.T) {
	source := twoCandidatesCheapUntilHeavy(t)
	source.pointed = pointedFacts(t, declaredFacts(t, "1", false), &physicalDouble{answer: adapter.NoPhysicalFactsObtained()})

	preRoute := loadAt(t, source, nrdomain.NewPreRouteFactsPoint())
	arrival := loadAt(t, source, nrdomain.NewArrivalFactsPoint())
	for _, line := range []string{"line-light", "line-flat"} {
		if factOf(t, preRoute.Facts, line).AmountMinor() != factOf(t, arrival.Facts, line).AmountMinor() {
			t.Fatalf("%s：退回声明的价与预路由不同", line)
		}
	}
	if arrival.FactsBasis != nrdomain.ParcelFactsDeclarationFallback {
		t.Fatalf("成本族依据 %s，want DECLARATION_FALLBACK", arrival.FactsBasis)
	}
	for _, citation := range arrival.Citations {
		if citation.FactsBasis() != nrdomain.ParcelFactsDeclarationFallback {
			t.Fatalf("出处依据 %s，want DECLARATION_FALLBACK", citation.FactsBasis())
		}
	}
}

// Covers: 物理事实在而定不下用哪条：候选一律待判断、没有出处，成本族依据说明是定不下，声明不顶替实测。
func TestUnresolvedPhysicalFactsLeaveEveryCandidatePendingWithoutCitations(t *testing.T) {
	source := twoCandidatesCheapUntilHeavy(t)
	source.pointed = pointedFacts(t, declaredFacts(t, "1", false), &physicalDouble{answer: adapter.UnresolvedPhysicalFacts()})

	costs := loadAt(t, source, nrdomain.NewArrivalFactsPoint())
	for _, line := range []string{"line-light", "line-flat"} {
		if fact := factOf(t, costs.Facts, line); fact.State() != nrdomain.CandidateCostPending {
			t.Fatalf("%s = %s，want 待判断", line, fact.State())
		}
	}
	if costs.FactsBasis != nrdomain.ParcelFactsPhysicalUnresolved || len(costs.Citations) != 0 {
		t.Fatalf("依据 %s、出处 %d 条，want 定不下、没有出处", costs.FactsBasis, len(costs.Citations))
	}
}

// Covers: 事实依据是取数侧的约定——有事实却没说依据、没事实却说成出价的依据，都是装配写坏了，挂在 ErrRouteCostAnswerUnusable
// 名下响亮报错，不静默出价。
func TestAFactsSourceThatMisreportsItsBasisIsUnusable(t *testing.T) {
	for name, test := range map[string]struct {
		hasFacts bool
		basis    nrdomain.ParcelFactsBasis
	}{
		"有事实没依据":      {true, nrdomain.ParcelFactsBasisInvalid},
		"有事实却说没有":     {true, nrdomain.ParcelFactsNone},
		"没事实却说是声明":    {false, nrdomain.ParcelFactsDeclaration},
		"没事实且没说为什么没有": {false, nrdomain.ParcelFactsBasisInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			source := twoCandidatesCheapUntilHeavy(t)
			source.hasFacts, source.facts.Basis = test.hasFacts, test.basis
			_, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
				pathOf(t, "line-light", 1)))
			if !errors.Is(err, nrports.ErrRouteCostAnswerUnusable) {
				t.Fatalf("err = %v, want ErrRouteCostAnswerUnusable", err)
			}
		})
	}
}

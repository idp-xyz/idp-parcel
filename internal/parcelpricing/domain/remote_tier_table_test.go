package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件钉 ADR-0348 决定三：附加费价表可以声明按偏远档位查档（与重量并列），一条规则一个费用代码，各档金额在同一张
// 价表里；档位由评价从偏远档位目录解出，解不出答现有的待判断，不套默认档。取值全是 SYN（只记 S）。

const (
	tierZoneCatalogue = "SYN-CAT-ZONE"
	tierTierCatalogue = "SYN-CAT-TIER"
)

// tierSurchargeTable 造一张按档位给金额的附加费价表：每档一条从 0 起的开口重量段，金额与重量无关，只由档位定。
func tierSurchargeTable(t testing.TB, amounts map[string]string) domain.RateTableVersion {
	t.Helper()
	usd := mustValue(t, domain.NewCurrency, "USD")
	entries := make([]domain.RateEntry, 0, len(amounts))
	for tier, amount := range amounts {
		entry, err := domain.NewOpenEndedRateEntry(
			mustValue(t, domain.NewRateEntryID, "SYN-TIER-ENTRY-"+tier), tier, weight(t, "0", domain.WeightUnitKilogram), money(t, amount, usd))
		if err != nil {
			t.Fatalf("档位条目 %s：%v", tier, err)
		}
		entries = append(entries, entry)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "SYN-TIER-TABLE", "v1"),
		domain.RateTableFamilyWeightZone, usd, domain.WeightUnitKilogram, effectivePeriod(t), entries)
	if err != nil {
		t.Fatalf("档位价表：%v", err)
	}
	return table
}

// tierRule 造一条按档位查表的附加费规则，触发条件是「档位属于 triggeredTiers 之一」——哪些档位收费是触发条件的事，
// 各档收多少是价表的事。
func tierRule(t testing.TB, table domain.RateTableVersion, triggeredTiers ...string) domain.SurchargeRule {
	t.Helper()
	calculation, err := domain.NewRemoteTierTableLookupSurcharge(table)
	if err != nil {
		t.Fatalf("按偏远档位查表的计算：%v", err)
	}
	return tierRuleWith(t, "SYN-RULE-REMOTE", "REMOTE_AREA_SURCHARGE", calculation, triggeredTiers...)
}

func tierRuleWith(t testing.TB, id, code string, calculation domain.SurchargeCalculation, triggeredTiers ...string) domain.SurchargeRule {
	t.Helper()
	operands := make([]domain.TriggerCondition, 0, len(triggeredTiers))
	for _, tier := range triggeredTiers {
		condition, err := domain.NewCategoryFeatureCondition(domain.FeatureAddressType, domain.CategoryValue(tier))
		if err != nil {
			t.Fatalf("档位条件 %s：%v", tier, err)
		}
		operands = append(operands, leafTrigger(t, condition))
	}
	trigger, err := domain.NewAnyOfTrigger(operands...)
	if err != nil {
		t.Fatalf("触发条件：%v", err)
	}
	rule, err := domain.NewSurchargeRule(id, mustValue(t, domain.NewChargeCode, code), id, domain.ChargeEffectAdd, trigger, calculation)
	if err != nil {
		t.Fatalf("附加费规则：%v", err)
	}
	standalone, err := rule.Standalone()
	if err != nil {
		t.Fatalf("并列计收：%v", err)
	}
	return standalone
}

func tierStructures(t testing.TB, rule domain.SurchargeRule, bindTier bool) domain.PricingPlanStructures {
	t.Helper()
	base, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{rule}, nil, nil)
	if err != nil {
		t.Fatalf("结构：%v", err)
	}
	links := []domain.ReferenceCatalogueLink{catalogueLink(t, domain.CatalogueKindZone, tierZoneCatalogue)}
	if bindTier {
		links = append(links, catalogueLink(t, domain.CatalogueKindRemoteTier, tierTierCatalogue))
	}
	bound, err := base.WithReferenceCatalogues(links...)
	if err != nil {
		t.Fatalf("绑目录：%v", err)
	}
	return bound
}

// tierInput 是带尺寸与目录读数的邮编输入：绑了目录的卡走的正是这条路，附加费规则要读尺寸侧特征所以必须带尺寸。
func tierInput(t testing.TB, destination, tier string) domain.PricingInputSnapshot {
	t.Helper()
	sides := dimensions(t, "10", "10", "10", domain.LengthUnitCentimeter)
	input, err := domain.NewPostalPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewPricingScopeID, "scope-1"),
		packageSubject(t, "package-1"), postalRoute(t, "", destination),
		weight(t, "5", domain.WeightUnitKilogram), &sides, time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("邮编输入：%v", err)
	}
	readings := []domain.ResolvedCatalogueValue{catalogueReading(t, domain.CatalogueKindZone, tierZoneCatalogue, "v1", "Z1")}
	if tier != "" {
		readings = append(readings, catalogueReading(t, domain.CatalogueKindRemoteTier, tierTierCatalogue, "v1", tier))
	}
	withReadings, err := input.WithReferenceCatalogues(readings...)
	if err != nil {
		t.Fatalf("补目录读数：%v", err)
	}
	return withReadings
}

func tierPlan(t *testing.T, amounts map[string]string, triggered ...string) domain.PricingPlanVersion {
	t.Helper()
	return planWithStructures(t, tierStructures(t, tierRule(t, tierSurchargeTable(t, amounts), triggered...), true))
}

// Covers: AT-PP-036
func TestATPP036OneRuleOneChargeCodeChargesEachRemoteTierItsOwnAmount(t *testing.T) {
	plan := tierPlan(t, map[string]string{"T1": "12", "T2": "28"}, "T1", "T2")

	for _, tc := range []struct {
		tier   string
		total  string
		charge string
	}{
		{"T1", "22", "12"},
		{"T2", "38", "28"},
		{"T0", "10", ""},
	} {
		t.Run(tc.tier, func(t *testing.T) {
			evaluation := evaluate(t, "eval-tier-"+tc.tier, plan, tierInput(t, "098123", tc.tier))
			if evaluation.Status() != domain.EvaluationCompleted {
				t.Fatalf("状态 %s，问题项 %v", evaluation.Status(), evaluation.Issues())
			}
			if got := totalOf(t, evaluation); got != tc.total {
				t.Fatalf("合计 %s，手算 %s（基础 10 加 %q）", got, tc.total, tc.charge)
			}
			got := lineAmounts(evaluation)["REMOTE_AREA_SURCHARGE"]
			if got != tc.charge {
				t.Fatalf("偏远费用行 %q，手算 %q；全部行 %v", got, tc.charge, lineAmounts(evaluation))
			}
			if tc.charge != "" && !explanationMentions(evaluation, "looked up by remote tier "+tc.tier) {
				t.Fatalf("解释没有写明按哪一档查：%v", evaluation.Explanation())
			}
			if tc.charge == "" && !explanationMentions(evaluation, "SYN-RULE-REMOTE did not apply") {
				t.Fatalf("T0 不收也要留痕未命中：%v", evaluation.Explanation())
			}
		})
	}
	if len(plan.Structures().SurchargeRules()) != 1 {
		t.Fatal("一条规则就够了：不再为每一档各写一条、各占一个费用代码")
	}
}

func TestATPP036ATierKeyedTableStillBracketsByWeightWithinATier(t *testing.T) {
	usd := mustValue(t, domain.NewCurrency, "USD")
	var entries []domain.RateEntry
	for _, row := range []struct{ id, tier, from, to, amount string }{
		{"E-T1-LOW", "T1", "0", "10", "12"},
		{"E-T1-HIGH", "T1", "10", "", "20"},
	} {
		var entry domain.RateEntry
		var err error
		if row.to == "" {
			entry, err = domain.NewOpenEndedRateEntry(mustValue(t, domain.NewRateEntryID, row.id), row.tier, weight(t, row.from, domain.WeightUnitKilogram), money(t, row.amount, usd))
		} else {
			entry, err = domain.NewRateEntry(mustValue(t, domain.NewRateEntryID, row.id), row.tier,
				weight(t, row.from, domain.WeightUnitKilogram), weight(t, row.to, domain.WeightUnitKilogram), money(t, row.amount, usd))
		}
		if err != nil {
			t.Fatalf("条目 %s：%v", row.id, err)
		}
		entries = append(entries, entry)
	}
	table, err := domain.NewRateTableVersion(versionReference(t, domain.ArtifactRateTable, "SYN-TIER-BANDED", "v1"),
		domain.RateTableFamilyWeightZone, usd, domain.WeightUnitKilogram, effectivePeriod(t), entries)
	if err != nil {
		t.Fatalf("价表：%v", err)
	}
	plan := planWithStructures(t, tierStructures(t, tierRule(t, table, "T1"), true))
	// 5 kg 落在 [0,10) 段：基础 10 + 12。
	if got := totalOf(t, evaluate(t, "eval-tier-band", plan, tierInput(t, "098123", "T1"))); got != "22" {
		t.Fatalf("合计 %s，手算 22", got)
	}
}

// Covers: AT-PP-037
func TestATPP037AnUnresolvedTierIsPendingNotDefaultedAndATierTheTableLacksIsAGapNotAZero(t *testing.T) {
	plan := tierPlan(t, map[string]string{"T1": "12", "T2": "28"}, "T1", "T2", "T3")

	// 目录没给读数（没查过）。
	noReading := evaluate(t, "eval-tier-no-reading", plan, tierInput(t, "098123", ""))
	if noReading.Status() != domain.EvaluationPending || issueCodes(noReading)[0] != "REMOTE_TIER_UNRESOLVED" {
		t.Fatalf("没有档位读数：%s %v，应待判断 REMOTE_TIER_UNRESOLVED", noReading.Status(), issueCodes(noReading))
	}
	if _, ok := noReading.Total(); ok || len(noReading.ChargeLines()) != 0 && hasChargeCode(noReading, "REMOTE_AREA_SURCHARGE") {
		t.Fatal("解不出档位的评价不得交出合计或偏远费用行")
	}

	// 查过目录、目的邮编没有条目。
	consulted, err := domain.NewUnresolvedCatalogueReading(domain.CatalogueKindRemoteTier, versionReference(t, domain.ArtifactReferenceCatalogue, tierTierCatalogue, "v1"))
	if err != nil {
		t.Fatalf("未解出的读数：%v", err)
	}
	input := tierInput(t, "099999", "")
	input, err = input.WithReferenceCatalogues(catalogueReading(t, domain.CatalogueKindZone, tierZoneCatalogue, "v1", "Z1"), consulted)
	if err != nil {
		t.Fatalf("补读数：%v", err)
	}
	notFound := evaluate(t, "eval-tier-not-found", plan, input)
	if notFound.Status() != domain.EvaluationPending || issueCodes(notFound)[0] != "REMOTE_TIER_UNRESOLVED" {
		t.Fatalf("目录查无此邮编：%s %v，应待判断 REMOTE_TIER_UNRESOLVED", notFound.Status(), issueCodes(notFound))
	}

	// 档位解出来了、触发条件也成立，价表却没有这一档：那是卡上的空档，不是规则没命中，更不是零。
	gap := evaluate(t, "eval-tier-table-gap", plan, tierInput(t, "098123", "T3"))
	if gap.Status() != domain.EvaluationPending || issueCodes(gap)[0] != "RATE_NOT_FOUND" {
		t.Fatalf("价表缺这一档：%s %v，应待判断 RATE_NOT_FOUND", gap.Status(), issueCodes(gap))
	}
	if !strings.Contains(gap.Issues()[0].Message(), "remote tier T3") {
		t.Fatalf("问题项没有点名档位：%s", gap.Issues()[0].Message())
	}
}

// Covers: AT-PP-038
func TestATPP038ATierKeyedTableNeedsTheCardToBindARemoteTierCatalogueAndAZoneKeyedTableIsUnchanged(t *testing.T) {
	rule := tierRule(t, tierSurchargeTable(t, map[string]string{"T1": "12"}), "T1")
	if _, err := newPlanWithStructures(t, tierStructures(t, rule, false)); !errors.Is(err, domain.ErrInvalidPricingPlan) {
		t.Fatalf("没绑偏远档位目录的卡声明了按档位查表：%v，应拒为 ErrInvalidPricingPlan", err)
	}

	// 既有的按分区查表：同一张价表按分区键，行为、解释、规范化版本都与此前相同。
	zoneTable := tableSurchargeTable(t, "Z1", "18")
	calculation, err := domain.NewTableLookupSurcharge(zoneTable)
	if err != nil {
		t.Fatalf("按分区查表：%v", err)
	}
	if calculation.TableKey() != domain.TableKeyZone {
		t.Fatalf("缺省的查档维度 %q，应是分区", calculation.TableKey())
	}
	zoneRule := tierRuleWith(t, "SYN-RULE-ZONE", "ZONE_SURCHARGE", calculation, "T1")
	plan := planWithStructures(t, tierStructures(t, zoneRule, true))
	if plan.CanonicalizationVersion() != "PPC-5" {
		t.Fatalf("按分区查表的方案写 %s，应仍是 PPC-5", plan.CanonicalizationVersion())
	}
	evaluation := evaluate(t, "eval-zone-keyed", plan, tierInput(t, "098123", "T1"))
	if got := totalOf(t, evaluation); got != "28" {
		t.Fatalf("合计 %s，手算 28 = 10 + 18", got)
	}
	if explanationMentions(evaluation, "looked up by remote tier") {
		t.Fatalf("按分区查表的解释不得出现档位字样：%v", evaluation.Explanation())
	}
}

// tableSurchargeTable 是按分区给金额的开口单段价表，档位测试里用来做「既有读法不变」的对照。
func tableSurchargeTable(t testing.TB, zone, amount string) domain.RateTableVersion {
	t.Helper()
	usd := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewOpenEndedRateEntry(mustValue(t, domain.NewRateEntryID, "SYN-ZONE-ENTRY-"+zone), zone, weight(t, "0", domain.WeightUnitKilogram), money(t, amount, usd))
	if err != nil {
		t.Fatalf("条目：%v", err)
	}
	table, err := domain.NewRateTableVersion(versionReference(t, domain.ArtifactRateTable, "SYN-ZONE-TABLE", "v1"),
		domain.RateTableFamilyWeightZone, usd, domain.WeightUnitKilogram, effectivePeriod(t), []domain.RateEntry{entry})
	if err != nil {
		t.Fatalf("价表：%v", err)
	}
	return table
}

func TestATPP038ATierKeyedOperandInsideAGreaterOfIsKeyedByTierToo(t *testing.T) {
	usd := mustValue(t, domain.NewCurrency, "USD")
	byTier, err := domain.NewRemoteTierTableLookupSurcharge(tierSurchargeTable(t, map[string]string{"T1": "12"}))
	if err != nil {
		t.Fatalf("档位操作数：%v", err)
	}
	floor, err := domain.NewFixedAmountSurcharge(money(t, "15", usd))
	if err != nil {
		t.Fatalf("定额操作数：%v", err)
	}
	greater, err := domain.NewGreaterOfSurcharge(byTier, floor)
	if err != nil {
		t.Fatalf("取较大值：%v", err)
	}
	rule := tierRuleWith(t, "SYN-RULE-GREATER", "REMOTE_AREA_SURCHARGE", greater, "T1")
	plan := planWithStructures(t, tierStructures(t, rule, true))
	if plan.CanonicalizationVersion() != "PPC-6" {
		t.Fatalf("操作数里按档位查表的方案写 %s，应是 PPC-6", plan.CanonicalizationVersion())
	}
	// 档位表 12 与定额 15 取较大者 15。
	if got := totalOf(t, evaluate(t, "eval-tier-greater", plan, tierInput(t, "098123", "T1"))); got != "25" {
		t.Fatalf("合计 %s，手算 25 = 10 + max(12, 15)", got)
	}
	if _, err := newPlanWithStructures(t, tierStructures(t, rule, false)); !errors.Is(err, domain.ErrInvalidPricingPlan) {
		t.Fatalf("操作数里按档位查表而没绑档位目录：%v，应拒为 ErrInvalidPricingPlan", err)
	}
}

func TestTableKeyOnANonLookupCalculationIsRefused(t *testing.T) {
	// 查档维度只对查表有意义：没有公开的构造路径能造出非查表却带档位键的计算，读回的坏数据由方案重验拒绝。
	plan := tierPlan(t, map[string]string{"T1": "12"}, "T1")
	raw, _ := planSnapshotRoundTrip(t, plan)
	if _, err := domain.RehydratePriceCardRegistration(tamperedDocument(t, raw, func(document map[string]any) {
		rules := document["plan"].(map[string]any)["surchargeRules"].([]any)
		calculation := rules[0].(map[string]any)["calculation"].(map[string]any)
		calculation["method"] = "FIXED_AMOUNT"
		delete(calculation, "table")
	})); err == nil {
		t.Fatal("非查表的计算带着档位键，应被方案重验拒绝")
	}
}

// Covers: AT-PP-039
func TestATPP039PlansWithTheNewStructuresAreWrittenAsPPC6AndOthersStayPPC5(t *testing.T) {
	tier := tierPlan(t, map[string]string{"T1": "12", "T2": "28"}, "T1", "T2")
	bounded := boundedPlan(t, "25", "")
	plain := planWithStructures(t, domain.PricingPlanStructures{})
	if tier.CanonicalizationVersion() != "PPC-6" || bounded.CanonicalizationVersion() != "PPC-6" || plain.CanonicalizationVersion() != "PPC-5" {
		t.Fatalf("规范化版本 %s / %s / %s，应为 PPC-6 / PPC-6 / PPC-5",
			tier.CanonicalizationVersion(), bounded.CanonicalizationVersion(), plain.CanonicalizationVersion())
	}
	if domain.CurrentCanonicalizationVersion() != "PPC-5" {
		t.Fatalf("基础版本 %s，应仍是 PPC-5", domain.CurrentCanonicalizationVersion())
	}

	for name, plan := range map[string]domain.PricingPlanVersion{"档位查表": tier, "总额下限": bounded} {
		raw, rebuilt := planSnapshotRoundTrip(t, plan)
		if rebuilt.ContentDigest() != plan.ContentDigest() || rebuilt.CanonicalizationVersion() != "PPC-6" {
			t.Fatalf("%s：快照往返后摘要或版本变了", name)
		}
		// 记着 PPC-5 却带了 PPC-6 的内容：版本与内容对不上，读回门拒。
		if _, err := domain.RehydratePriceCardRegistration(tamperedDocument(t, raw, func(document map[string]any) {
			document["plan"].(map[string]any)["canonicalization"] = "PPC-5"
		})); err == nil {
			t.Fatalf("%s：记着 PPC-5 却带了 PPC-6 的内容，应被读回门拒绝", name)
		}
		// 本构建不认的版本：结构上算不出来，不是内容冲突。
		if _, err := domain.RehydratePriceCardRegistration(tamperedDocument(t, raw, func(document map[string]any) {
			document["plan"].(map[string]any)["canonicalization"] = "PPC-7"
		})); !errors.Is(err, domain.ErrCanonicalizationVersionUnsupported) {
			t.Fatalf("%s：PPC-7：%v，应拒为 ErrCanonicalizationVersionUnsupported", name, err)
		}
	}
	// 反过来：不带新内容却记着 PPC-6。
	plainRaw, _ := planSnapshotRoundTrip(t, plain)
	if _, err := domain.RehydratePriceCardRegistration(tamperedDocument(t, plainRaw, func(document map[string]any) {
		document["plan"].(map[string]any)["canonicalization"] = "PPC-6"
	})); err == nil {
		t.Fatal("不带新内容却记着 PPC-6，应被读回门拒绝")
	}
	// 内容被改（下限金额）：摘要自校暴露。
	boundedRaw, _ := planSnapshotRoundTrip(t, bounded)
	if _, err := domain.RehydratePriceCardRegistration(tamperedDocument(t, boundedRaw, func(document map[string]any) {
		bounds := document["plan"].(map[string]any)["totalChargeBounds"].(map[string]any)
		bounds["minimum"].(map[string]any)["amount"] = "99"
	})); err == nil {
		t.Fatal("下限被改而摘要没变，应被摘要自校拒绝")
	}
}

func TestATPP039EvaluationsOfAPPC6PlanAreWrittenUnderPPC6AndReplayToTheSameDigest(t *testing.T) {
	plan := tierPlan(t, map[string]string{"T1": "12", "T2": "28"}, "T1", "T2")
	original := evaluate(t, "eval-ppc6-source", plan, tierInput(t, "098123", "T2"))
	if original.PlanCanonicalizationVersion() != "PPC-6" || original.Status() != domain.EvaluationCompleted {
		t.Fatalf("状态 %s 版本 %s", original.Status(), original.PlanCanonicalizationVersion())
	}
	raw, err := domain.MarshalEvaluationSnapshot(original)
	if err != nil {
		t.Fatalf("折装评价快照：%v", err)
	}
	restored, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("重建评价：%v", err)
	}
	if restored.SemanticDigest() != original.SemanticDigest() {
		t.Fatal("快照往返后语义摘要变了")
	}
	replayed, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-ppc6-replay"), restored, plan, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if replayed.Status() != domain.EvaluationCompleted || replayed.SemanticDigest() != original.SemanticDigest() {
		t.Fatalf("重放：%s，摘要 %s 对 %s", replayed.Status(), replayed.SemanticDigest(), original.SemanticDigest())
	}

	// PPC-6 评价的语义摘要里写的规范化版本是 PPC-6，所以它与同样结果的 PPC-5 评价摘要不同：换号的意义正在于此。
	if !strings.Contains(string(raw), `"planCanonicalization":"PPC-6"`) {
		t.Fatal("评价快照没有记下方案的规范化版本 PPC-6")
	}
}

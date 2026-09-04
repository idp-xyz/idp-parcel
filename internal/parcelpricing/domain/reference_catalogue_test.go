package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

func catalogueLink(t testing.TB, kind domain.CatalogueKind, id string) domain.ReferenceCatalogueLink {
	t.Helper()
	link, err := domain.NewReferenceCatalogueLink(kind, id)
	if err != nil {
		t.Fatalf("catalogue link %s/%s: %v", kind, id, err)
	}
	return link
}

func structuresWithCatalogues(t testing.TB, links ...domain.ReferenceCatalogueLink) domain.PricingPlanStructures {
	t.Helper()
	structures, err := (domain.PricingPlanStructures{}).WithReferenceCatalogues(links...)
	if err != nil {
		t.Fatalf("structures with catalogues: %v", err)
	}
	return structures
}

// Covers: ADR-0109 Decision 三「价卡以目录绑定声明分区与档位从哪个目录来，绑标识不绑版本」——
// 一张卡对每一种目录至多声明一个来源；同一种声明两遍，评价就说不出该查哪一本。
func TestPlanDeclaresWhichCatalogueSuppliesZoneAndTier(t *testing.T) {
	zone := catalogueLink(t, domain.CatalogueKindZone, "carrier-zone-chart")
	tier := catalogueLink(t, domain.CatalogueKindRemoteTier, "carrier-das-table")

	structures := structuresWithCatalogues(t, zone, tier)
	links := structures.ReferenceCatalogues()
	if len(links) != 2 {
		t.Fatalf("declared %d catalogue links, want 2", len(links))
	}
	// 声明顺序不进内容：两个轴按种类排定，读的人与摘要看到的是同一份。
	if links[0].Kind() != domain.CatalogueKindRemoteTier || links[1].Kind() != domain.CatalogueKindZone {
		t.Fatalf("links are not ordered by kind: %s, %s", links[0].Kind(), links[1].Kind())
	}
	if links[1].CatalogueID() != "carrier-zone-chart" {
		t.Fatalf("zone link points at %q", links[1].CatalogueID())
	}
	if !structures.Declared() {
		t.Fatal("a card that binds a catalogue has declared a structure the evaluator must execute")
	}

	if _, err := domain.NewReferenceCatalogueLink(domain.CatalogueKindZone, " "); !errors.Is(err, domain.ErrInvalidReferenceCatalogue) {
		t.Fatalf("blank catalogue id accepted: %v", err)
	}
	if _, err := domain.NewReferenceCatalogueLink(domain.CatalogueKind("POSTAL_LIBRARY"), "x"); !errors.Is(err, domain.ErrInvalidReferenceCatalogue) {
		t.Fatalf("catalogue kind outside the closed set accepted: %v", err)
	}
	other := catalogueLink(t, domain.CatalogueKindZone, "another-zone-chart")
	if _, err := (domain.PricingPlanStructures{}).WithReferenceCatalogues(zone, other); !errors.Is(err, domain.ErrInvalidReferenceCatalogue) {
		t.Fatalf("two zone catalogues on one card accepted: %v", err)
	}
}

func postalRoute(t testing.TB, origin, destination string) domain.PostalRoute {
	t.Helper()
	route, err := domain.NewPostalRoute(origin, destination)
	if err != nil {
		t.Fatalf("postal route %s→%s: %v", origin, destination, err)
	}
	return route
}

// postalInput 是只带邮编、不带调用方分区的输入：绑了目录的卡走的正是这条路。
func postalInput(t testing.TB, destination string, readings ...domain.ResolvedCatalogueValue) domain.PricingInputSnapshot {
	t.Helper()
	input, err := domain.NewPostalPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		packageSubject(t, "package-1"),
		postalRoute(t, "", destination),
		weight(t, "5", domain.WeightUnitKilogram),
		nil,
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("postal input: %v", err)
	}
	if len(readings) == 0 {
		return input
	}
	withReadings, err := input.WithReferenceCatalogues(readings...)
	if err != nil {
		t.Fatalf("attach catalogue readings: %v", err)
	}
	return withReadings
}

func catalogueReading(t testing.TB, kind domain.CatalogueKind, id, version, value string) domain.ResolvedCatalogueValue {
	t.Helper()
	reading, err := domain.NewResolvedCatalogueValue(kind, versionReference(t, domain.ArtifactReferenceCatalogue, id, version), domain.CategoryValue(value))
	if err != nil {
		t.Fatalf("catalogue reading %s: %v", id, err)
	}
	return reading
}

func issueCodes(evaluation domain.PricingEvaluation) []string {
	codes := make([]string, 0, len(evaluation.Issues()))
	for _, issue := range evaluation.Issues() {
		codes = append(codes, issue.Code())
	}
	return codes
}

// Covers: ADR-0109 Decision 四「绑定了目录的卡，zone 由评价从目录解析，不再由调用方给」与 Decision 三
// 「目录版本引用冻进评价清单」——查表用的分区来自读数，采用的目录版本进清单，解释里写明从哪一版
// 解出来的。
func TestBoundCardResolvesZoneFromTheCatalogueReadingAndFreezesItsVersion(t *testing.T) {
	plan := planWithStructures(t, structuresWithCatalogues(t, catalogueLink(t, domain.CatalogueKindZone, "carrier-zone-chart")))
	input := postalInput(t, "94016", catalogueReading(t, domain.CatalogueKindZone, "carrier-zone-chart", "v3", "Z1"))

	evaluation := evaluate(t, "eval-catalogue-zone", plan, input)
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s %v", evaluation.Status(), issueCodes(evaluation))
	}
	matched, _ := evaluation.MatchedRate()
	if matched.Zone() != "Z1" {
		t.Fatalf("matched zone = %q, want the catalogue's Z1", matched.Zone())
	}
	frozen := false
	for _, reference := range evaluation.Manifest().References() {
		if reference.Kind() == domain.ArtifactReferenceCatalogue && reference.ID() == "carrier-zone-chart" && reference.Version() == "v3" {
			frozen = true
		}
	}
	if !frozen {
		t.Fatalf("manifest does not freeze the catalogue version: %#v", evaluation.Manifest().References())
	}
	if !explanationMentions(evaluation, "carrier-zone-chart@v3") {
		t.Fatalf("explanation = %#v, want the catalogue version the zone came from", evaluation.Explanation())
	}
}

// Covers: ADR-0109 Decision 四「查不到即待判断，不给默认」——没有读数、或在用版本里查不到该邮编，
// 评价都停在待判断，原因码与 REFERENCE_SERIES_UNRESOLVED 同形而分区与档位两格分开。
func TestBoundCardWithoutAResolvableZoneIsPendingNotDefaulted(t *testing.T) {
	plan := planWithStructures(t, structuresWithCatalogues(t, catalogueLink(t, domain.CatalogueKindZone, "carrier-zone-chart")))

	noReading := evaluate(t, "eval-catalogue-no-reading", plan, postalInput(t, "94016"))
	if noReading.Status() != domain.EvaluationPending || issueCodes(noReading)[0] != "ZONE_UNRESOLVED" {
		t.Fatalf("without a reading: %s %v", noReading.Status(), issueCodes(noReading))
	}
	if _, ok := noReading.Total(); ok {
		t.Fatal("a pending evaluation handed out a total")
	}

	consulted, err := domain.NewUnresolvedCatalogueReading(domain.CatalogueKindZone, versionReference(t, domain.ArtifactReferenceCatalogue, "carrier-zone-chart", "v3"))
	if err != nil {
		t.Fatalf("unresolved reading: %v", err)
	}
	notFound := evaluate(t, "eval-catalogue-not-found", plan, postalInput(t, "99999", consulted))
	if notFound.Status() != domain.EvaluationPending || issueCodes(notFound)[0] != "ZONE_UNRESOLVED" {
		t.Fatalf("with an unresolved reading: %s %v", notFound.Status(), issueCodes(notFound))
	}
	if !explanationMentions(notFound, "carrier-zone-chart@v3") && !strings.Contains(notFound.Issues()[0].Message(), "v3") {
		t.Fatalf("the consulted version is not recorded: %#v / %#v", notFound.Explanation(), notFound.Issues())
	}
}

// Covers: ADR-0109 Decision 三「绑标识不绑版本」的另一半——读数来自另一本目录不是缺口而是分歧：按卡
// 从未声明过的目录解出的分区去查表，会静默用错分区计价。
func TestReadingFromAnotherCatalogueIsAConflict(t *testing.T) {
	plan := planWithStructures(t, structuresWithCatalogues(t, catalogueLink(t, domain.CatalogueKindZone, "carrier-zone-chart")))
	input := postalInput(t, "94016", catalogueReading(t, domain.CatalogueKindZone, "someone-elses-chart", "v1", "Z1"))

	evaluation := evaluate(t, "eval-catalogue-mismatch", plan, input)
	if evaluation.Status() != domain.EvaluationConflict || issueCodes(evaluation)[0] != "REFERENCE_CATALOGUE_MISMATCH" {
		t.Fatalf("status = %s %v", evaluation.Status(), issueCodes(evaluation))
	}
}

// Covers: ADR-0109 Decision 四「没有绑定目录的卡保留今天的调用方给值路径——这不是默认值，是该卡显式
// 没有声明来源」；而没绑目录又没给分区的输入，没有任何一方能产出分区，评价待判断而不是编一个。
func TestUnboundCardKeepsTheCallerGivenZoneAndPendsWithoutOne(t *testing.T) {
	plan := planWithStructures(t, domain.PricingPlanStructures{})

	given := evaluate(t, "eval-caller-zone", plan, syntheticInput(t, "5", "Z1"))
	if given.Status() != domain.EvaluationCompleted {
		t.Fatalf("caller-given zone: %s %v", given.Status(), issueCodes(given))
	}
	postalOnly := evaluate(t, "eval-postal-only", plan, postalInput(t, "94016"))
	if postalOnly.Status() != domain.EvaluationPending || issueCodes(postalOnly)[0] != "ZONE_UNRESOLVED" {
		t.Fatalf("postal-only input on an unbound card: %s %v", postalOnly.Status(), issueCodes(postalOnly))
	}
}

// Covers: CONTEXT「地址分类：由计价参考目录的偏远档位表解析得出的偏远档位」与 ADR-0109 Context 把
// 档位对到 FeatureAddressType——绑了档位目录的卡，附加费规则的地址类型条件读到的是目录解出的档位。
func TestRemoteTierReadingFeedsTheAddressTypeCondition(t *testing.T) {
	rule := standaloneRule(t, surchargeRuleWithCategory(t, "das-extended", "DAS_EXTENDED", domain.FeatureAddressType, "DAS_EXTENDED", "3.5"))
	base, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{rule}, nil, nil)
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	structures, err := base.WithReferenceCatalogues(catalogueLink(t, domain.CatalogueKindRemoteTier, "carrier-das-table"))
	if err != nil {
		t.Fatalf("with tier catalogue: %v", err)
	}
	plan := planWithStructures(t, structures)
	sides := dimensions(t, "10", "10", "10", domain.LengthUnitInch)
	caller := syntheticInputWithDimensions(t, "5", "Z1", sides)

	inTier, err := caller.WithReferenceCatalogues(catalogueReading(t, domain.CatalogueKindRemoteTier, "carrier-das-table", "v2", "DAS_EXTENDED"))
	if err != nil {
		t.Fatalf("attach tier: %v", err)
	}
	charged := evaluate(t, "eval-tier-hit", plan, inTier)
	if charged.Status() != domain.EvaluationCompleted || len(charged.ChargeLines()) != 2 {
		t.Fatalf("tier hit: %s lines=%d %v", charged.Status(), len(charged.ChargeLines()), issueCodes(charged))
	}

	outsideTier, err := caller.WithReferenceCatalogues(catalogueReading(t, domain.CatalogueKindRemoteTier, "carrier-das-table", "v2", "DAS"))
	if err != nil {
		t.Fatalf("attach tier: %v", err)
	}
	notCharged := evaluate(t, "eval-tier-miss", plan, outsideTier)
	if notCharged.Status() != domain.EvaluationCompleted || len(notCharged.ChargeLines()) != 1 {
		t.Fatalf("tier miss: %s lines=%d %v", notCharged.Status(), len(notCharged.ChargeLines()), issueCodes(notCharged))
	}

	noTier := evaluate(t, "eval-tier-unresolved", plan, caller)
	if noTier.Status() != domain.EvaluationPending || issueCodes(noTier)[0] != "REMOTE_TIER_UNRESOLVED" {
		t.Fatalf("no tier reading: %s %v", noTier.Status(), issueCodes(noTier))
	}
}

// Covers: CONTEXT「特征……分区」——分区是判定条件可读的类别量；此前评价从未把它填进特征，一条分区
// 条件永远报特征不可用。调用方给的分区与目录解出的分区都要落进同一格。
func TestZoneConditionReadsTheResolvedZone(t *testing.T) {
	rule := standaloneRule(t, surchargeRuleWithCategory(t, "zone-1-handling", "ZONE_1_HANDLING", domain.FeatureZone, "Z1", "2"))
	structures, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{rule}, nil, nil)
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	plan := planWithStructures(t, structures)
	evaluation := evaluate(t, "eval-zone-condition", plan, syntheticInputWithDimensions(t, "5", "Z1", dimensions(t, "10", "10", "10", domain.LengthUnitInch)))
	if evaluation.Status() != domain.EvaluationCompleted || len(evaluation.ChargeLines()) != 2 {
		t.Fatalf("zone condition: %s lines=%d %v", evaluation.Status(), len(evaluation.ChargeLines()), issueCodes(evaluation))
	}
}

// Covers: 评价快照往返——邮编路线与目录读数都是评价输入的一部分，重建后语义摘要自校仍过。
func TestCatalogueReadingsSurviveTheEvaluationSnapshot(t *testing.T) {
	plan := planWithStructures(t, structuresWithCatalogues(t, catalogueLink(t, domain.CatalogueKindZone, "carrier-zone-chart")))
	input := postalInput(t, "94016", catalogueReading(t, domain.CatalogueKindZone, "carrier-zone-chart", "v3", "Z1"))
	evaluation := evaluate(t, "eval-catalogue-snapshot", plan, input)

	raw, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	restored, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	route, declared := restored.Input().PostalRoute()
	if !declared || route.Destination() != "94016" {
		t.Fatalf("postal route lost: %#v %v", route, declared)
	}
	readings := restored.Input().CatalogueReadings()
	if len(readings) != 1 || readings[0].Reference().Version() != "v3" {
		t.Fatalf("catalogue readings lost: %#v", readings)
	}
	if restored.SemanticDigest() != evaluation.SemanticDigest() {
		t.Fatal("semantic digest changed across the snapshot")
	}
}

func surchargeRuleWithCategory(t testing.TB, id, code string, source domain.FeatureSource, expected, amount string) domain.SurchargeRule {
	t.Helper()
	condition, err := domain.NewCategoryFeatureCondition(source, domain.CategoryValue(expected))
	if err != nil {
		t.Fatalf("category condition: %v", err)
	}
	calculation, err := domain.NewFixedAmountSurcharge(money(t, amount, mustValue(t, domain.NewCurrency, "USD")))
	if err != nil {
		t.Fatalf("fixed surcharge: %v", err)
	}
	rule, err := domain.NewSurchargeRule(id, mustValue(t, domain.NewChargeCode, code), id, domain.ChargeEffectAdd, leafTrigger(t, condition), calculation)
	if err != nil {
		t.Fatalf("surcharge rule %s: %v", id, err)
	}
	return rule
}

// Covers: ADR-0109 Decision 四「两条路径在卡的内容摘要里分得开」与 Consequences「规范化形状因绑定
// 进卡内容而换号……记在 PPC-5 下」——绑了目录的卡与没绑的卡摘要不同，而两张都没绑的卡不因这一格
// 多出差异；快照往返之后绑定与摘要都还在。
func TestCatalogueLinkEntersTheContentDigestAndSurvivesTheSnapshot(t *testing.T) {
	unbound := planWithStructures(t, domain.PricingPlanStructures{})
	unboundAgain := planWithStructures(t, domain.PricingPlanStructures{})
	bound := planWithStructures(t, structuresWithCatalogues(t, catalogueLink(t, domain.CatalogueKindZone, "carrier-zone-chart")))

	if unbound.ContentDigest() != unboundAgain.ContentDigest() {
		t.Fatal("two cards without catalogue links disagree on their digest")
	}
	if unbound.ContentDigest() == bound.ContentDigest() {
		t.Fatal("binding a catalogue left the content digest unchanged")
	}
	if bound.CanonicalizationVersion() != domain.CurrentCanonicalizationVersion() {
		t.Fatalf("canonicalization = %q, want the current version (PPC-5 already absorbed this widening)", bound.CanonicalizationVersion())
	}

	raw, err := domain.MarshalPricingPlanSnapshot(bound)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	restored, err := domain.RehydratePricingPlanSnapshot(raw)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	links := restored.Structures().ReferenceCatalogues()
	if len(links) != 1 || links[0].Kind() != domain.CatalogueKindZone || links[0].CatalogueID() != "carrier-zone-chart" {
		t.Fatalf("catalogue link did not survive the snapshot: %#v", links)
	}
	if restored.ContentDigest() != bound.ContentDigest() {
		t.Fatal("rehydrated card recomputes a different digest")
	}
}

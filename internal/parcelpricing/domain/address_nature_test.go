package domain_test

import (
	"bytes"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证收件地址性质（商业 / 住宅）作为评价输入与判定特征（ADR-0347 决定四）：它是与偏远档位并列的另一个槽，
// 由发起方声明，缺席时读它的规则答待判断，从不套默认。

func natureInput(t testing.TB, nature string) domain.PricingInputSnapshot {
	t.Helper()
	input := syntheticInputWithDimensions(t, "5", "Z1", dimensions(t, "10", "10", "10", domain.LengthUnitCentimeter))
	if nature == "" {
		return input
	}
	declared, err := domain.NewAddressNature(nature)
	if err != nil {
		t.Fatalf("address nature %q: %v", nature, err)
	}
	withNature, err := input.WithAddressNature(declared)
	if err != nil {
		t.Fatalf("with address nature: %v", err)
	}
	return withNature
}

func natureRule(t testing.TB, id, code, expected, amount string) domain.SurchargeRule {
	t.Helper()
	return standaloneRule(t, surchargeRuleWithCategory(t, id, code, domain.FeatureAddressNature, expected, amount))
}

// Covers: ADR-0347 决定四——地址性质的词表封闭为商业与住宅，原词、大小写与空白都不宽容；卡上期望取值不在表内一律拒
// （一个拼错的 RESIDENTAL 会成为永远不命中的条件，少收而无人知道）；其余类别特征的取值依旧只要求非空。
func TestAddressNatureVocabularyIsClosed(t *testing.T) {
	for _, accepted := range []string{"COMMERCIAL", "RESIDENTIAL"} {
		if nature, err := domain.NewAddressNature(accepted); err != nil || nature.String() != accepted {
			t.Fatalf("NewAddressNature(%q) = %q, %v", accepted, nature, err)
		}
	}
	for _, refused := range []string{"", "residential", " RESIDENTIAL", "RESIDENTIAL ", "MIXED", "RESIDENTAL"} {
		if _, err := domain.NewAddressNature(refused); !errors.Is(err, domain.ErrInvalidAddressNature) {
			t.Fatalf("NewAddressNature(%q) err = %v, want the closed-vocabulary refusal", refused, err)
		}
	}

	if _, err := domain.NewCategoryFeatureCondition(domain.FeatureAddressNature, "RESIDENTAL"); !errors.Is(err, domain.ErrInvalidFeatureCondition) {
		t.Fatalf("err = %v, want a misspelled expected nature refused at construction", err)
	}
	if _, err := domain.NewCategoryFeatureCondition(domain.FeatureAddressNature, "RESIDENTIAL"); err != nil {
		t.Fatalf("a valid expected nature refused: %v", err)
	}
	if _, err := domain.NewCategoryFeatureCondition(domain.FeatureAddressType, "T-ANYTHING"); err != nil {
		t.Fatalf("the address-type slot carries remote tiers, whose codes are tenant values and stay open: %v", err)
	}
	if _, err := natureInput(t, "").WithAddressNature(domain.AddressNature("MIXED")); !errors.Is(err, domain.ErrPricingInputInvalid) {
		t.Fatalf("err = %v, want an out-of-vocabulary nature refused on the input", err)
	}
}

// Covers: ADR-0347 决定四——住宅派送费按声明的地址性质收：住宅收、商业不收且解释里留痕；没声明不是商业也不是住宅，
// 评价待判断（ADDRESS_NATURE_REQUIRED），没有合计、没有费用行，补上声明就能得出价格。
func TestResidentialSurchargeFollowsTheDeclaredAddressNature(t *testing.T) {
	structures := structuresOf(t, natureRule(t, "residential-delivery", "RESIDENTIAL_DELIVERY_SURCHARGE", "RESIDENTIAL", "6"))
	plan := planWithStructures(t, structures)

	residential := evaluate(t, "eval-nature-residential", plan, natureInput(t, "RESIDENTIAL"))
	if residential.Status() != domain.EvaluationCompleted || !hasChargeCode(residential, "RESIDENTIAL_DELIVERY_SURCHARGE") {
		t.Fatalf("residential: status=%s issues=%v lines=%d", residential.Status(), issueCodes(residential), len(residential.ChargeLines()))
	}
	if got := ownWeightTotal(t, residential); got != "16" {
		t.Fatalf("residential total = %s, want 16 = 10 base + 6", got)
	}

	commercial := evaluate(t, "eval-nature-commercial", plan, natureInput(t, "COMMERCIAL"))
	if commercial.Status() != domain.EvaluationCompleted || hasChargeCode(commercial, "RESIDENTIAL_DELIVERY_SURCHARGE") {
		t.Fatalf("commercial: status=%s issues=%v", commercial.Status(), issueCodes(commercial))
	}
	if !explanationMentions(commercial, "ADDRESS_NATURE EQ RESIDENTIAL not met") {
		t.Fatalf("commercial explanation = %v, want the missed rule recorded", commercial.Explanation())
	}

	undeclared := evaluate(t, "eval-nature-undeclared", plan, natureInput(t, ""))
	if undeclared.Status() != domain.EvaluationPending || len(issueCodes(undeclared)) != 1 || issueCodes(undeclared)[0] != "ADDRESS_NATURE_REQUIRED" {
		t.Fatalf("undeclared: status=%s issues=%v, want PENDING ADDRESS_NATURE_REQUIRED", undeclared.Status(), issueCodes(undeclared))
	}
	if _, ok := undeclared.Total(); ok {
		t.Fatal("an evaluation waiting for the address nature carries a total — it must not default to commercial")
	}
}

// Covers: ADR-0347 决定四 + CONTEXT「地址分类」——地址性质与偏远档位是两个独立维度：同一票既命中住宅派送又落在某一偏远档位，
// 二者同时计收；地址类型那一槽仍只承载偏远档位，声明的地址性质不会满足一条 ADDRESS_TYPE 条件。
func TestResidentialDeliveryAndRemoteTierCoexistOnOneCard(t *testing.T) {
	tierRule := standaloneRule(t, surchargeRuleWithCategory(t, "das-extended", "DAS_EXTENDED", domain.FeatureAddressType, "T1", "3.5"))
	natureRuleResidential := natureRule(t, "residential-delivery", "RESIDENTIAL_DELIVERY_SURCHARGE", "RESIDENTIAL", "6")
	sneaky := standaloneRule(t, surchargeRuleWithCategory(t, "type-says-residential", "TYPE_RESIDENTIAL", domain.FeatureAddressType, "RESIDENTIAL", "9"))
	base, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{tierRule, natureRuleResidential, sneaky}, nil, nil)
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	structures, err := base.WithReferenceCatalogues(catalogueLink(t, domain.CatalogueKindRemoteTier, "carrier-das-table"))
	if err != nil {
		t.Fatalf("with tier catalogue: %v", err)
	}
	plan := planWithStructures(t, structures)
	tier := catalogueReading(t, domain.CatalogueKindRemoteTier, "carrier-das-table", "v2", "T1")

	both, err := natureInput(t, "RESIDENTIAL").WithReferenceCatalogues(tier)
	if err != nil {
		t.Fatalf("attach tier: %v", err)
	}
	evaluation := evaluate(t, "eval-nature-and-tier", plan, both)
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status=%s issues=%v", evaluation.Status(), issueCodes(evaluation))
	}
	if !hasChargeCode(evaluation, "DAS_EXTENDED") || !hasChargeCode(evaluation, "RESIDENTIAL_DELIVERY_SURCHARGE") {
		t.Fatalf("lines=%v, want both the remote tier fee and the residential delivery fee", evaluation.ChargeLines())
	}
	if hasChargeCode(evaluation, "TYPE_RESIDENTIAL") {
		t.Fatal("the address-type slot read the address nature; it carries the remote tier only")
	}
	if got := ownWeightTotal(t, evaluation); got != "19.5" {
		t.Fatalf("total = %s, want 19.5 = 10 base + 3.5 tier + 6 residential", got)
	}

	commercialInTier, err := natureInput(t, "COMMERCIAL").WithReferenceCatalogues(tier)
	if err != nil {
		t.Fatalf("attach tier: %v", err)
	}
	onlyTier := evaluate(t, "eval-commercial-and-tier", plan, commercialInTier)
	if hasChargeCode(onlyTier, "RESIDENTIAL_DELIVERY_SURCHARGE") || !hasChargeCode(onlyTier, "DAS_EXTENDED") {
		t.Fatalf("commercial in a remote tier: lines=%v", onlyTier.ChargeLines())
	}
}

// Covers: ADR-0347 决定四——地址性质随评价快照往返（可缺键，未声明的评价不写这一键，旧快照照常读回）；重放用快照里的声明得同一个摘要。
func TestAddressNatureSurvivesTheSnapshotAndTheReplayUsesIt(t *testing.T) {
	plan := planWithStructures(t, structuresOf(t, natureRule(t, "residential-delivery", "RESIDENTIAL_DELIVERY_SURCHARGE", "RESIDENTIAL", "6")))
	evaluation := evaluate(t, "eval-nature-snapshot", plan, natureInput(t, "RESIDENTIAL"))

	raw, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"addressNature":"RESIDENTIAL"`)) {
		t.Fatalf("snapshot = %s, want the declared nature written", raw)
	}
	restored, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if nature, declared := restored.Input().AddressNature(); !declared || nature != domain.AddressNatureResidential {
		t.Fatalf("restored nature = %q %v", nature, declared)
	}
	if restored.SemanticDigest() != evaluation.SemanticDigest() {
		t.Fatal("semantic digest changed across the snapshot")
	}
	replay, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-nature-snapshot-2"), restored, plan, domain.EvidenceSynthetic)
	if err != nil || replay.SemanticDigest() != evaluation.SemanticDigest() || !hasChargeCode(replay, "RESIDENTIAL_DELIVERY_SURCHARGE") {
		t.Fatalf("replay: err=%v digest=%s want %s", err, replay.SemanticDigest(), evaluation.SemanticDigest())
	}

	undeclared := evaluate(t, "eval-nature-snapshot-undeclared", planWithStructures(t, domain.PricingPlanStructures{}), natureInput(t, ""))
	rawUndeclared, err := domain.MarshalEvaluationSnapshot(undeclared)
	if err != nil {
		t.Fatalf("marshal undeclared: %v", err)
	}
	if bytes.Contains(rawUndeclared, []byte("addressNature")) {
		t.Fatalf("snapshot = %s, an evaluation without a declared nature must not carry the key", rawUndeclared)
	}
}

// Covers: ADR-0347 决定四、六——地址性质不进语义摘要的规范化文档（与结算币种同一种放法，不加键、不换规范化版本）：
// 方案不读它时，声明与否不改变摘要，已算对的评价逐字不变；方案读它时，影响结果的方式是命中与否，结果不同摘要自然不同。
func TestAddressNatureIsOutsideTheCanonicalInputAndStillMovesTheDigestThroughTheResult(t *testing.T) {
	plain := planWithStructures(t, domain.PricingPlanStructures{})
	without := evaluate(t, "eval-digest-plain", plain, natureInput(t, ""))
	with := evaluate(t, "eval-digest-plain", plain, natureInput(t, "RESIDENTIAL"))
	if without.SemanticDigest() != with.SemanticDigest() {
		t.Fatal("a declared nature changed the digest of an evaluation whose card never reads it")
	}

	reading := planWithStructures(t, structuresOf(t, natureRule(t, "residential-delivery", "RESIDENTIAL_DELIVERY_SURCHARGE", "RESIDENTIAL", "6")))
	residential := evaluate(t, "eval-digest-reading", reading, natureInput(t, "RESIDENTIAL"))
	commercial := evaluate(t, "eval-digest-reading", reading, natureInput(t, "COMMERCIAL"))
	if residential.SemanticDigest() == commercial.SemanticDigest() {
		t.Fatal("residential and commercial evaluations of a card that reads the nature share a digest")
	}
}

// Covers: ADR-0347 决定四、六——读地址性质的卡经价卡登记快照往返后仍是这张卡（ADDRESS_NATURE 条件与内容摘要都在）；
// ReadsFeature 让编排在发起评价之前就知道这张卡要发起方声明地址性质；不读它的卡答否。
func TestACardReadingTheAddressNatureSurvivesTheRegistrationSnapshot(t *testing.T) {
	reading := planWithStructures(t, structuresOf(t, natureRule(t, "residential-delivery", "RESIDENTIAL_DELIVERY_SURCHARGE", "RESIDENTIAL", "6")))
	if !reading.Structures().ReadsFeature(domain.FeatureAddressNature) {
		t.Fatal("a card with an ADDRESS_NATURE rule does not report reading it")
	}
	if planWithStructures(t, domain.PricingPlanStructures{}).Structures().ReadsFeature(domain.FeatureAddressNature) {
		t.Fatal("a card without rules reports reading the address nature")
	}
	_, restored := planSnapshotRoundTrip(t, reading)
	if restored.ContentDigest() != reading.ContentDigest() || !restored.Structures().ReadsFeature(domain.FeatureAddressNature) {
		t.Fatal("the rehydrated card lost its ADDRESS_NATURE rule or recomputes another digest")
	}
	rules := restored.Structures().SurchargeRules()
	expected, categorical := rules[0].Condition().Predicate()
	if !categorical {
		t.Fatal("restored trigger is not a single predicate")
	}
	if value, isCategory := expected.ExpectedCategory(); !isCategory || value != "RESIDENTIAL" {
		t.Fatalf("restored expected value = %q", value)
	}
}

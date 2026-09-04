package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证票级 / 主单级评价主体与聚合方式（ADR-0111）：委托与承运总单是评价对象，输入快照带成员清单与合计量；
// 按 KG 的行用合计计价重量查表、按件的行按件数乘定额、按票 / 按主单的行取定额一次；费用行标聚合单位；聚合
// 方式与主体种类不合是冲突。主单级只证形状——承运总单身份今天没有生产来源（TF 未立册），夹具里的引用是 SYN。

func members(t testing.TB, totalActual string, totalVolumetric *string, ids ...string) domain.MemberManifest {
	t.Helper()
	packages := make([]domain.PackageID, 0, len(ids))
	for _, id := range ids {
		packages = append(packages, mustValue(t, domain.NewPackageID, id))
	}
	var volumetric *domain.Weight
	if totalVolumetric != nil {
		declared := weight(t, *totalVolumetric, domain.WeightUnitKilogram)
		volumetric = &declared
	}
	manifest, err := domain.NewMemberManifest(packages, weight(t, totalActual, domain.WeightUnitKilogram), volumetric)
	if err != nil {
		t.Fatalf("member manifest: %v", err)
	}
	return manifest
}

func shipmentSubject(t testing.TB, id string) domain.EvaluationSubject {
	t.Helper()
	subject, err := domain.NewShipmentSubject(id)
	if err != nil {
		t.Fatalf("shipment subject: %v", err)
	}
	return subject
}

func aggregateInput(t testing.TB, subject domain.EvaluationSubject, manifest domain.MemberManifest) domain.PricingInputSnapshot {
	t.Helper()
	input, err := domain.NewAggregatePricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		subject, "Z1", manifest,
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("aggregate input: %v", err)
	}
	return input
}

// aggregatePlan 是 newPlanWithStructures 的聚合版：同一张价表与重量策略，聚合方式由调用方给。
func aggregatePlan(t testing.TB, aggregation domain.AggregationMode, method domain.PricingWeightMethod, rules ...domain.FixedChargeRule) (domain.PricingPlanVersion, error) {
	t.Helper()
	currency := usd(t)
	entry, err := domain.NewRateEntry(mustValue(t, domain.NewRateEntryID, "entry-aggregate"), "Z1",
		weight(t, "0", domain.WeightUnitKilogram), weight(t, "10", domain.WeightUnitKilogram), money(t, "10", currency))
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(versionReference(t, domain.ArtifactRateTable, "table-aggregate", "v1"),
		domain.RateTableFamilyWeightZone, currency, domain.WeightUnitKilogram, effectivePeriod(t), []domain.RateEntry{entry})
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingNone, weight(t, "1", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	var factor *domain.VolumetricFactor
	if method == domain.PricingWeightMax {
		factorRounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "0.1", domain.WeightUnitKilogram))
		if err != nil {
			t.Fatalf("factor rounding: %v", err)
		}
		declared, err := domain.NewVolumetricFactor(decimal(t, "5000"), domain.LengthUnitCentimeter, factorRounding)
		if err != nil {
			t.Fatalf("factor: %v", err)
		}
		factor = &declared
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(versionReference(t, domain.ArtifactWeightPolicy, "weight-aggregate", "v1"), method, rounding, factor)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	return domain.NewAggregatePricingPlanVersion(
		aggregation,
		versionReference(t, domain.ArtifactPricingPlan, "plan-aggregate", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t), table, weightPolicy, rules, domain.PricingPlanStructures{},
	)
}

func mustAggregatePlan(t testing.TB, aggregation domain.AggregationMode, method domain.PricingWeightMethod, rules ...domain.FixedChargeRule) domain.PricingPlanVersion {
	t.Helper()
	plan, err := aggregatePlan(t, aggregation, method, rules...)
	if err != nil {
		t.Fatalf("aggregate plan: %v", err)
	}
	return plan
}

func perPiece(t testing.TB, rule domain.FixedChargeRule) domain.FixedChargeRule {
	t.Helper()
	piece, err := rule.PerPiece()
	if err != nil {
		t.Fatalf("per piece: %v", err)
	}
	return piece
}

// Covers: ADR-0111 Decision 二「一份快照对应一个委托……携带成员包裹引用与合计量」与 Decision 四「委托主体引用
// parcel-shipment 拥有的委托身份」——本上下文不铸身份，成员清单非空，包裹主体不得带成员清单。
func TestShipmentSubjectCarriesItsMembers(t *testing.T) {
	input := aggregateInput(t, shipmentSubject(t, "SR-1"), members(t, "7", nil, "pkg-1", "pkg-2", "pkg-3"))
	manifest, ok := input.Members()
	if !ok || manifest.Count() != 3 || manifest.TotalActualWeight().Value().String() != "7" {
		t.Fatalf("members = %#v %v", manifest, ok)
	}
	if _, isPackage := input.PackageID(); isPackage {
		t.Fatal("a shipment-level input answered a package id")
	}
	subject, _ := input.Subject()
	if subject.Kind() != domain.SubjectShipment || subject.Reference() != "SR-1" {
		t.Fatalf("subject = %#v", subject)
	}

	if _, err := domain.NewShipmentSubject(" "); !errors.Is(err, domain.ErrPricingInputInvalid) {
		t.Fatalf("blank shipment id accepted: %v", err)
	}
	if _, err := domain.NewMemberManifest(nil, weight(t, "7", domain.WeightUnitKilogram), nil); !errors.Is(err, domain.ErrPricingInputInvalid) {
		t.Fatalf("empty member list accepted: %v", err)
	}
	if _, err := domain.NewAggregatePricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewPricingScopeID, "scope-1"),
		packageSubject(t, "package-1"), "Z1", members(t, "7", nil, "pkg-1"),
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	); !errors.Is(err, domain.ErrPricingInputInvalid) {
		t.Fatalf("package subject with a member manifest accepted: %v", err)
	}
}

// Covers: ADR-0111 Decision 一、二——按 KG 的行在合计计价重量上查表、按票的行取定额一次、按件的行按件数乘定额；
// 费用行上标聚合单位。
func TestPerShipmentPlanEvaluatesOnTheAggregate(t *testing.T) {
	clearance := fixedRule(t, "clearance", domain.ChargeEffectAdd, "5", 1)
	handling := perPiece(t, fixedRule(t, "handling", domain.ChargeEffectAdd, "2", 2))
	plan := mustAggregatePlan(t, domain.AggregationPerShipment, domain.PricingWeightActualOnly, clearance, handling)
	if plan.Aggregation() != domain.AggregationPerShipment {
		t.Fatalf("aggregation = %s", plan.Aggregation())
	}

	evaluation := evaluate(t, "eval-shipment", plan, aggregateInput(t, shipmentSubject(t, "SR-1"), members(t, "7", nil, "pkg-1", "pkg-2", "pkg-3")))
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s %v %v", evaluation.Status(), issueCodes(evaluation), evaluation.Explanation())
	}
	total, _ := evaluation.Total()
	if total.Amount().String() != "21" {
		t.Fatalf("total = %s, want 10 (7 KG in band) + 5 once + 2 × 3 pieces", total.Amount())
	}
	lines := evaluation.ChargeLines()
	if len(lines) != 3 || lines[0].Scope() != domain.ChargeScopeShipment || lines[1].Scope() != domain.ChargeScopeShipment || lines[2].Scope() != domain.ChargeScopePackage {
		t.Fatalf("line scopes = %s %s %s", lines[0].Scope(), lines[1].Scope(), lines[2].Scope())
	}
	if lines[2].Amount().Amount().String() != "6" {
		t.Fatalf("per-piece line = %s, want 2 × 3", lines[2].Amount().Amount())
	}
	if !explanationMentions(evaluation, "3 pieces") {
		t.Fatalf("explanation = %#v, want the piece count", evaluation.Explanation())
	}
	weight, _ := evaluation.PricingWeight()
	if weight.RoundedWeight().Value().String() != "7" {
		t.Fatalf("pricing weight = %s, want the members' total", weight.RoundedWeight().Value())
	}
}

// Covers: ADR-0111 Alternatives「主体不对，标签救不回来」——聚合方式与主体种类必须一致：逐委托的卡拿到包裹主体、
// 逐包裹的卡拿到委托主体，都是冲突不是缺口。
func TestAggregationAndSubjectMustAgree(t *testing.T) {
	shipmentPlan := mustAggregatePlan(t, domain.AggregationPerShipment, domain.PricingWeightActualOnly)
	packageOnShipmentPlan := evaluate(t, "eval-mismatch-1", shipmentPlan, syntheticInput(t, "5", "Z1"))
	if packageOnShipmentPlan.Status() != domain.EvaluationConflict || issueCodes(packageOnShipmentPlan)[0] != "AGGREGATION_SUBJECT_MISMATCH" {
		t.Fatalf("package on a per-shipment plan: %s %v", packageOnShipmentPlan.Status(), issueCodes(packageOnShipmentPlan))
	}
	packagePlan := planWithStructures(t, domain.PricingPlanStructures{})
	shipmentOnPackagePlan := evaluate(t, "eval-mismatch-2", packagePlan, aggregateInput(t, shipmentSubject(t, "SR-1"), members(t, "7", nil, "pkg-1")))
	if shipmentOnPackagePlan.Status() != domain.EvaluationConflict || issueCodes(shipmentOnPackagePlan)[0] != "AGGREGATION_SUBJECT_MISMATCH" {
		t.Fatalf("shipment on a per-package plan: %s %v", shipmentOnPackagePlan.Status(), issueCodes(shipmentOnPackagePlan))
	}
}

// Covers: 按件计收只在聚合主体上有意义——逐包裹的卡上「每件」与「每主体」是同一件事，声明它只会让两张行为相同
// 的卡摘要不同；构造门拒。
func TestPerPieceUnitIsOnlyForAggregatePlans(t *testing.T) {
	if _, err := aggregatePlan(t, domain.AggregationPerPackage, domain.PricingWeightActualOnly, perPiece(t, fixedRule(t, "handling", domain.ChargeEffectAdd, "2", 1))); !errors.Is(err, domain.ErrInvalidPricingPlan) {
		t.Fatalf("per-piece rule on a per-package plan accepted: %v", err)
	}
	if _, err := domain.NewAggregatePricingPlanVersion(domain.AggregationMode("PER_PALLET"),
		versionReference(t, domain.ArtifactPricingPlan, "plan-x", "v1"), mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t), domain.RateTableVersion{}, domain.PricingWeightPolicy{}, nil, domain.PricingPlanStructures{},
	); !errors.Is(err, domain.ErrInvalidPricingPlan) {
		t.Fatalf("aggregation outside the closed set accepted: %v", err)
	}
}

// Covers: ADR-0111 Decision 四「主单级那一半形状定、身份来源缺」——逐主单的聚合方式、承运总单主体与快照形状照常
// 落地；生产上形成一次主单级评价要等 TF 立册，这里的引用只是 SYN 夹具，不是任何承运总单。
func TestMasterDocumentShapeExistsWithoutAProductionSource(t *testing.T) {
	subject, err := domain.NewMasterDocumentSubject("SYN-MAWB-1")
	if err != nil {
		t.Fatalf("master document subject: %v", err)
	}
	plan := mustAggregatePlan(t, domain.AggregationPerMasterDocument, domain.PricingWeightActualOnly, fixedRule(t, "customs", domain.ChargeEffectAdd, "50", 1))
	evaluation := evaluate(t, "eval-mawb", plan, aggregateInput(t, subject, members(t, "9", nil, "pkg-1", "pkg-2")))
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s %v", evaluation.Status(), issueCodes(evaluation))
	}
	total, _ := evaluation.Total()
	if total.Amount().String() != "60" || evaluation.ChargeLines()[1].Scope() != domain.ChargeScopeMasterDocument {
		t.Fatalf("total = %s scope = %s", total.Amount(), evaluation.ChargeLines()[1].Scope())
	}
}

// Covers: ADR-0111 Decision 二「成员清单是评价输入的一部分，进语义摘要：同一主单成员变了就是另一次评价」；快照
// 往返保留成员清单。
func TestMemberManifestEntersTheSemanticDigestAndTheSnapshot(t *testing.T) {
	plan := mustAggregatePlan(t, domain.AggregationPerShipment, domain.PricingWeightActualOnly)
	two := evaluate(t, "eval-members", plan, aggregateInput(t, shipmentSubject(t, "SR-1"), members(t, "7", nil, "pkg-1", "pkg-2")))
	three := evaluate(t, "eval-members", plan, aggregateInput(t, shipmentSubject(t, "SR-1"), members(t, "7", nil, "pkg-1", "pkg-2", "pkg-3")))
	if two.SemanticDigest() == three.SemanticDigest() {
		t.Fatal("two evaluations with different member lists share a semantic digest")
	}
	raw, err := domain.MarshalEvaluationSnapshot(three)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	restored, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	manifest, ok := restored.Input().Members()
	if !ok || manifest.Count() != 3 || restored.SemanticDigest() != three.SemanticDigest() {
		t.Fatalf("members lost across the snapshot: %#v %v", manifest, ok)
	}
}

// Covers: ADR-0111 Decision 二「总体积重……按重量策略派生的合计计价重量是评价内的中间结果，与逐包裹时同形」——
// MAX 策略在聚合主体上取合计实重与合计体积重的较大者；没给合计体积重时待判断，不退回实重。
func TestMaxWeightOnAggregateUsesTheDeclaredTotalVolumetricWeight(t *testing.T) {
	plan := mustAggregatePlan(t, domain.AggregationPerShipment, domain.PricingWeightMax)
	volumetric := "9"
	heavier := evaluate(t, "eval-max-volumetric", plan, aggregateInput(t, shipmentSubject(t, "SR-1"), members(t, "7", &volumetric, "pkg-1", "pkg-2")))
	if heavier.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s %v", heavier.Status(), issueCodes(heavier))
	}
	if pricingWeight, _ := heavier.PricingWeight(); pricingWeight.RoundedWeight().Value().String() != "9" {
		t.Fatalf("pricing weight = %s, want the declared total volumetric weight", pricingWeight.RoundedWeight().Value())
	}
	undeclared := evaluate(t, "eval-max-undeclared", plan, aggregateInput(t, shipmentSubject(t, "SR-1"), members(t, "7", nil, "pkg-1", "pkg-2")))
	if undeclared.Status() != domain.EvaluationPending || issueCodes(undeclared)[0] != "DIMENSIONS_REQUIRED" {
		t.Fatalf("without a total volumetric weight: %s %v", undeclared.Status(), issueCodes(undeclared))
	}
}

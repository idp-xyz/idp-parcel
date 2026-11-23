package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证多件运输收费发生项的评价主体：身份是发生项本身，只在逐委托的卡上形成一次；
// 逐包裹的卡不收。计价重沿用逐件派生再求和。没有分区也没有邮编路线时评价待判断。

func TestATransportChargeOccurrencePricesOnceOnAPerShipmentCard(t *testing.T) {
	subject, err := domain.NewTransportChargeOccurrenceSubject("SYN-OCC-1")
	if err != nil {
		t.Fatal(err)
	}
	if subject.Kind() != domain.SubjectTransportChargeOccurrence || subject.Kind().String() != "TRANSPORT_CHARGE_OCCURRENCE" {
		t.Fatalf("主体 = %s", subject.Kind())
	}
	small := dimensions(t, "30", "20", "10", domain.LengthUnitCentimeter)
	large := dimensions(t, "50", "40", "30", domain.LengthUnitCentimeter)
	// 30×20×10 / 5000 = 1.2，实重 2，取 2；50×40×30 / 5000 = 12，实重 1，取 12；和是 14。
	manifest := shipmentPieces(t, "3", nil,
		piece(t, "SYN-PARCEL-01", "2", &small),
		piece(t, "SYN-PARCEL-02", "1", &large),
	)
	input := aggregateInput(t, subject, manifest)
	plan := declareShipmentWeight(t, segmentedShipmentPlan(t, domain.PricingWeightMax),
		domain.ShipmentWeightPerPieceThenSum, domain.ShipmentWeightSumThenRoundOnce)
	evaluation := evaluate(t, "eval-occ", plan, input)
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("状态 = %s %v", evaluation.Status(), issueCodes(evaluation))
	}
	if got := roundedPricingWeight(t, evaluation); got != "14" {
		t.Fatalf("计价重量 = %s，want 逐件取大者之和 14", got)
	}
	if evaluation.ChargeLines()[0].Scope() != domain.ChargeScopeShipment {
		t.Fatalf("基础行范围 = %s，want 每票一次", evaluation.ChargeLines()[0].Scope())
	}
	packagePlan := planWithStructures(t, domain.PricingPlanStructures{})
	mismatched := evaluate(t, "eval-occ-pkg", packagePlan, input)
	if mismatched.Status() != domain.EvaluationConflict || issueCodes(mismatched)[0] != "AGGREGATION_SUBJECT_MISMATCH" {
		t.Fatalf("逐包裹卡 = %s %v，want 冲突", mismatched.Status(), issueCodes(mismatched))
	}
}

func TestAnOccurrenceWithoutAZoneOrRouteStaysPending(t *testing.T) {
	subject, err := domain.NewTransportChargeOccurrenceSubject("SYN-OCC-1")
	if err != nil {
		t.Fatal(err)
	}
	manifest := shipmentPieces(t, "3", nil, piece(t, "SYN-PARCEL-01", "2", nil), piece(t, "SYN-PARCEL-02", "1", nil))
	input, err := domain.NewAggregatePricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		subject, "", manifest, time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := input.PostalRoute(); present || input.Zone() != "" {
		t.Fatal("没有路线却写进了分区或邮编")
	}
	evaluation := evaluate(t, "eval-occ-open", segmentedShipmentPlan(t, domain.PricingWeightActualOnly), input)
	if evaluation.Status() != domain.EvaluationPending || issueCodes(evaluation)[0] != "ZONE_UNRESOLVED" {
		t.Fatalf("状态 = %s %v，want 待判断", evaluation.Status(), issueCodes(evaluation))
	}
	if _, err := domain.NewAggregatePricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		shipmentSubject(t, "SR-1"), "", manifest, time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	); !errors.Is(err, domain.ErrPricingInputInvalid) {
		t.Fatalf("委托主体空分区被收下：%v", err)
	}
}

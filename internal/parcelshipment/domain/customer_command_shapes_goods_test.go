package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件钉资料修订的规范化形状 PAM-1 在货物明细上的同号改形（ADR-0373 决定八的前提，UC-PS-002「给开发的交接」那一行；票
// declared-goods-lines/06）：不带货物明细的修订逐字节同前，已存摘要与重放照旧成立；带了就进摘要，装箱分配的给入次序不是内容。

func pam1MeasurementSpec(t *testing.T) domain.SourceDataAmendmentPayloadSpec {
	t.Helper()
	scope, err := domain.NewParcelScopedSourceData(mustValue(t, domain.NewShipmentRequestID, "SHR-SYN-1"),
		mustValue(t, domain.NewDeclaredParcelID, "DPCL-SYN-1"), domain.DeclaredMeasurementDataGroup())
	if err != nil {
		t.Fatal(err)
	}
	weight, err := domain.NewDeclaredWeight(mustValue(t, domain.NewMeasurementValue, "1.50"), mustValue(t, domain.NewMeasurementUnitReference, "KG"))
	if err != nil {
		t.Fatal(err)
	}
	measurement, err := domain.NewDeclaredMeasurement(weight, domain.DeclaredDimensions{})
	if err != nil {
		t.Fatal(err)
	}
	return domain.SourceDataAmendmentPayloadSpec{
		OriginalAccessRequestID: "SYN-ACCESS-REQUEST-0001",
		Scope:                   scope,
		Basis:                   domain.NewSupplementOnAcceptanceBaseline(),
		Intent:                  domain.SupplementIntent,
		Reason:                  mustValue(t, domain.NewAmendmentReasonReference, "SYN-REASON-WEIGHT"),
		Measurement:             measurement,
	}
}

// pam1MeasurementGolden 取证于加货物明细键之前的提交（ad522127e）：同一份规格在那里算出的就是这一串。
const pam1MeasurementGolden = "PAM-1:be172086b42c46b408c70ece233d82f0b3dcf509b7d11712e6b68832c7f63097"

// Covers: 同号改形「旧载荷逐字节同前」——不带货物明细的修订摘要与加键之前一字不差。
func TestAnAmendmentWithoutGoodsKeepsItsPAM1Digest(t *testing.T) {
	canonical, err := domain.CanonicalizeCustomerSourceDataAmendment(pam1MeasurementSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := canonical.Digest().String(); got != pam1MeasurementGolden {
		t.Fatalf("digest = %s, want %s（加货物明细键改了旧载荷的摘要，已存的重放判断会失效）", got, pam1MeasurementGolden)
	}
}

func goodsAmendmentSpec(t *testing.T, packing ...[2]string) domain.SourceDataAmendmentPayloadSpec {
	t.Helper()
	scope, err := domain.NewShipmentScopedSourceData(mustValue(t, domain.NewShipmentRequestID, "SHR-SYN-1"), domain.GoodsDetailsDataGroup())
	if err != nil {
		t.Fatal(err)
	}
	basis, err := domain.NewAmendmentOfVersion(mustValue(t, domain.NewSourceDataVersionID, "SUBV-SYN-1"))
	if err != nil {
		t.Fatal(err)
	}
	cell := func(value string) domain.GoodsCellSpec {
		return domain.GoodsCellSpec{Value: value, Mark: domain.SourceMarkThisDeclaration}
	}
	spec := domain.GoodsDetailsSpec{Lines: []domain.GoodsLineSpec{{Quantity: cell("2")}}}
	var members []domain.DeclaredParcelID
	for _, allocation := range packing {
		parcel := mustValue(t, domain.NewDeclaredParcelID, allocation[0])
		members = append(members, parcel)
		spec.Packing = append(spec.Packing, domain.PackingAllocationSpec{Parcel: parcel, Line: 1, Quantity: allocation[1]})
	}
	goods, err := domain.NewGoodsDetails(spec, members)
	if err != nil {
		t.Fatal(err)
	}
	return domain.SourceDataAmendmentPayloadSpec{
		OriginalAccessRequestID: "SYN-ACCESS-REQUEST-0001", Scope: scope, Basis: basis, Intent: domain.CorrectionIntent,
		Reason: mustValue(t, domain.NewAmendmentReasonReference, "SYN-REASON-CUSTOMS-RETURN"), Goods: goods,
	}
}

// Covers: 货物明细进 PAM-1 摘要并成为这一版的内容；装箱分配是集合，给入次序不是内容；同一份修订既带测量又带货物明细立不起来。
func TestAGoodsAmendmentEntersThePAM1DigestAndBecomesTheVersionContent(t *testing.T) {
	forward, err := domain.CanonicalizeCustomerSourceDataAmendment(goodsAmendmentSpec(t, [2]string{"DPCL-A", "1"}, [2]string{"DPCL-B", "1"}))
	if err != nil {
		t.Fatal(err)
	}
	backward, err := domain.CanonicalizeCustomerSourceDataAmendment(goodsAmendmentSpec(t, [2]string{"DPCL-B", "1"}, [2]string{"DPCL-A", "1"}))
	if err != nil {
		t.Fatal(err)
	}
	if forward.Digest() != backward.Digest() {
		t.Fatal("装箱分配换了给入次序，摘要就变了")
	}
	other, err := domain.CanonicalizeCustomerSourceDataAmendment(goodsAmendmentSpec(t, [2]string{"DPCL-A", "2"}))
	if err != nil {
		t.Fatal(err)
	}
	if other.Digest() == forward.Digest() {
		t.Fatal("改了装箱分配，摘要没变")
	}
	if goods, declared := forward.Content().Goods(); !declared || len(goods.Packing()) != 2 {
		t.Fatalf("内容不是那一份货物明细：declared = %v", declared)
	}

	mixed := goodsAmendmentSpec(t)
	mixed.Measurement = pam1MeasurementSpec(t).Measurement
	if _, err := domain.CanonicalizeCustomerSourceDataAmendment(mixed); !errors.Is(err, domain.ErrInvalidSubmissionPayload) {
		t.Fatalf("测量与货物明细同在：err = %v, want ErrInvalidSubmissionPayload", err)
	}
}

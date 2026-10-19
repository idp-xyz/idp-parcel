package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证供应商协议正文的收费发生条件一节（票 demo-chain-gaps/29，ADR-0209）：逐原因声明计不计费，计费带数量形；
// 数量形按原因配；带这一节的正文以 PCC-2 规范化，不带的仍是 PCC-1、逐字节不变。夹具全是合成串，证据只记 S。

func chargedCondition(t *testing.T, reason domain.TransportChargeReason, form domain.ChargeQuantityForm) domain.ChargeOccurrenceCondition {
	t.Helper()
	condition, err := domain.NewChargedOccurrenceCondition(reason, form)
	if err != nil {
		t.Fatalf("charged %s %s: %v", reason, form, err)
	}
	return condition
}

func unchargedCondition(t *testing.T, reason domain.TransportChargeReason) domain.ChargeOccurrenceCondition {
	t.Helper()
	condition, err := domain.NewUnchargedOccurrenceCondition(reason)
	if err != nil {
		t.Fatalf("uncharged %s: %v", reason, err)
	}
	return condition
}

func trunkChargeConditions(t *testing.T) domain.ChargeOccurrenceConditions {
	t.Helper()
	conditions, err := domain.NewChargeOccurrenceConditions(
		chargedCondition(t, domain.ActualFulfillmentChargeReason, domain.PerPieceQuantity),
		unchargedCondition(t, domain.FailedAttemptChargeReason),
	)
	if err != nil {
		t.Fatalf("conditions: %v", err)
	}
	return conditions
}

// Covers: AT-PC-051——每个原因只收它的主要范围配得上的数量形：按件对四个原因都成立；按段只配实际履约、按尝试只配
// 失败尝试、按订舱只配订舱与取消。配不上的、集外的原因与数量形一律拒。
func TestEachReasonTakesOnlyItsQuantityForms(t *testing.T) {
	allowed := map[domain.TransportChargeReason][]domain.ChargeQuantityForm{
		domain.BookingChargeReason:           {domain.PerBookingQuantity, domain.PerPieceQuantity},
		domain.CancellationChargeReason:      {domain.PerBookingQuantity, domain.PerPieceQuantity},
		domain.FailedAttemptChargeReason:     {domain.PerAttemptQuantity, domain.PerPieceQuantity},
		domain.ActualFulfillmentChargeReason: {domain.PerSegmentQuantity, domain.PerPieceQuantity},
	}
	forms := []domain.ChargeQuantityForm{
		domain.PerSegmentQuantity, domain.PerPieceQuantity, domain.PerAttemptQuantity, domain.PerBookingQuantity,
	}
	for reason, accepted := range allowed {
		for _, form := range forms {
			_, err := domain.NewChargedOccurrenceCondition(reason, form)
			wanted := false
			for _, candidate := range accepted {
				wanted = wanted || candidate == form
			}
			if wanted && err != nil {
				t.Errorf("%s %s 应可声明：%v", reason, form, err)
			}
			if !wanted && !errors.Is(err, domain.ErrInvalidChargeOccurrenceCondition) {
				t.Errorf("%s %s 应拒，err = %v", reason, form, err)
			}
		}
		uncharged := unchargedCondition(t, reason)
		if uncharged.Charged() {
			t.Errorf("%s 不计费的声明读成了计费", reason)
		}
		if _, carried := uncharged.QuantityForm(); carried {
			t.Errorf("%s 不计费的声明带着数量形", reason)
		}
	}
	if _, err := domain.NewChargedOccurrenceCondition(domain.TransportChargeReasonInvalid, domain.PerPieceQuantity); err == nil {
		t.Error("集外原因应拒")
	}
	if _, err := domain.NewChargedOccurrenceCondition(domain.ActualFulfillmentChargeReason, domain.ChargeQuantityFormInvalid); err == nil {
		t.Error("集外数量形应拒")
	}
	if _, err := domain.NewUnchargedOccurrenceCondition(domain.TransportChargeReasonInvalid); err == nil {
		t.Error("集外原因不计费也应拒")
	}
}

// Covers: AT-PC-051——条件一节至少一条、每个原因至多一条；按原因的声明顺序排好，换个次序给是同一句话；没声明的原因
// 答 false，不由别的原因推出。
func TestChargeConditionsAreOnePerReasonAndOrdered(t *testing.T) {
	if _, err := domain.NewChargeOccurrenceConditions(); !errors.Is(err, domain.ErrInvalidChargeOccurrenceCondition) {
		t.Fatalf("空清单 err = %v，want ErrInvalidChargeOccurrenceCondition", err)
	}
	if _, err := domain.NewChargeOccurrenceConditions(
		chargedCondition(t, domain.ActualFulfillmentChargeReason, domain.PerPieceQuantity),
		unchargedCondition(t, domain.ActualFulfillmentChargeReason),
	); !errors.Is(err, domain.ErrInvalidChargeOccurrenceCondition) {
		t.Fatalf("同一原因两条 err = %v", err)
	}

	forward := trunkChargeConditions(t)
	backward, err := domain.NewChargeOccurrenceConditions(
		unchargedCondition(t, domain.FailedAttemptChargeReason),
		chargedCondition(t, domain.ActualFulfillmentChargeReason, domain.PerPieceQuantity),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !forward.Equal(backward) {
		t.Fatal("换个次序给出的同一组声明被读成两句话")
	}
	all := forward.All()
	if len(all) != 2 || all[0].Reason() != domain.FailedAttemptChargeReason || all[1].Reason() != domain.ActualFulfillmentChargeReason {
		t.Fatalf("各条没按原因的声明顺序排：%+v", all)
	}
	if condition, declared := forward.For(domain.ActualFulfillmentChargeReason); !declared || !condition.Charged() {
		t.Fatal("实际履约应声明为计费")
	} else if form, _ := condition.QuantityForm(); form != domain.PerPieceQuantity {
		t.Fatalf("实际履约数量形 = %s", form)
	}
	if _, declared := forward.For(domain.BookingChargeReason); declared {
		t.Fatal("没声明的订舱被读成了有声明")
	}
}

// Covers: ADR-0209 决定一——条件只能随正文一次固定：已带条件的正文再加即拒；正文没带时答没声明。
func TestAnAgreementTakesItsChargeConditionsOnce(t *testing.T) {
	agreement := supplierAgreement(t, "supplier-agreement-1", "supplier-1", "scope-buy-1")
	if _, declared := agreement.ChargeConditions(); declared {
		t.Fatal("没带条件的正文答了有声明")
	}
	declared, err := agreement.WithChargeConditions(trunkChargeConditions(t))
	if err != nil {
		t.Fatalf("带条件：%v", err)
	}
	if conditions, present := declared.ChargeConditions(); !present || !conditions.Equal(trunkChargeConditions(t)) {
		t.Fatal("带上的条件读不回来")
	}
	if _, err := declared.WithChargeConditions(trunkChargeConditions(t)); !errors.Is(err, domain.ErrInvalidSupplierAgreement) {
		t.Fatalf("再加一次 err = %v，want ErrInvalidSupplierAgreement", err)
	}
	if _, err := agreement.WithChargeConditions(domain.ChargeOccurrenceConditions{}); !errors.Is(err, domain.ErrInvalidSupplierAgreement) {
		t.Fatalf("零值条件 err = %v", err)
	}
}

// Covers: AT-PC-052——带条件的正文以 PCC-2 规范化，文档在供应商协议一节里多一键 chargeConditions，charged 两值都写出、
// 数量形只随计费；不带条件的同一正文仍是 PCC-1、文档字节与条件一节存在之前逐字节同（见本册既有用例钉的那份）。
// 条件不同即另一个串，换个次序给不是另一个串。
func TestChargeConditionsCanonicalizeAsTheNextShape(t *testing.T) {
	plain := supplierAgreementBody(t, "plan-buy-1", time.Time{})
	withConditions := plain
	conditions := trunkChargeConditions(t)
	withConditions.ChargeConditions = &conditions

	before := canonicalSupplierAgreement(t, plain)
	if before.Canonicalization() != "PCC-1" {
		t.Fatalf("不带条件的正文换了号：%s", before.Canonicalization())
	}
	after := canonicalSupplierAgreement(t, withConditions)
	if after.Canonicalization() != "PCC-2" || !strings.HasPrefix(after.Digest().String(), "PCC-2:") {
		t.Fatalf("带条件的正文 canonicalization = %q digest = %q，want PCC-2", after.Canonicalization(), after.Digest())
	}
	wantDocument := `{"canonicalization":"PCC-2","kind":"SUPPLIER_AGREEMENT",` +
		`"supplierAgreement":{"supplier":"supplier-1","legalEntity":"legal-1","scope":"scope-buy-1",` +
		`"purchasePlan":"plan-buy-1","effectiveStartsAt":"2026-01-03T00:00:00Z",` +
		`"chargeConditions":[{"reason":"FAILED_ATTEMPT","charged":false},` +
		`{"reason":"ACTUAL_FULFILLMENT","charged":true,"quantityForm":"PER_PIECE"}]}}`
	if got := string(after.Document()); got != wantDocument {
		t.Fatalf("document = %s\nwant       %s", got, wantDocument)
	}

	perSegment, err := domain.NewChargeOccurrenceConditions(
		chargedCondition(t, domain.ActualFulfillmentChargeReason, domain.PerSegmentQuantity),
		unchargedCondition(t, domain.FailedAttemptChargeReason),
	)
	if err != nil {
		t.Fatal(err)
	}
	other := plain
	other.ChargeConditions = &perSegment
	if canonicalSupplierAgreement(t, other).Digest() == after.Digest() {
		t.Fatal("数量形不同的两份条件共用了一个摘要")
	}
}

// Covers: AT-PC-052——两版文档都折得回：PCC-2 折回带条件的正文、再算一遍同一个串；标着 PCC-1 却带条件、标着 PCC-2
// 却不带条件、条件一节是空数组、PCC-2 标在别的册上，都不是本构建写下的文档，答不支持或拒，不静默丢掉条件。
func TestBothShapesRehydrateAndAMislabelledDocumentIsRefused(t *testing.T) {
	plain := supplierAgreementBody(t, "plan-buy-1", time.Time{})
	withConditions := plain
	conditions := trunkChargeConditions(t)
	withConditions.ChargeConditions = &conditions
	pcc1 := canonicalSupplierAgreement(t, plain)
	pcc2 := canonicalSupplierAgreement(t, withConditions)

	for _, canonical := range []domain.CanonicalPublicationContent{pcc1, pcc2} {
		content, err := domain.RehydratePublicationContent(canonical.Canonicalization(), canonical.Document())
		if err != nil {
			t.Fatalf("%s 折不回：%v", canonical.Canonicalization(), err)
		}
		again, err := domain.CanonicalizePublicationContent(content)
		if err != nil || again.Digest() != canonical.Digest() {
			t.Fatalf("%s 折回再算 = %v / %v，want %s", canonical.Canonicalization(), again.Digest(), err, canonical.Digest())
		}
	}
	rehydrated, _ := domain.RehydratePublicationContent(pcc2.Canonicalization(), pcc2.Document())
	if rehydrated.SupplierAgreement.ChargeConditions == nil || !rehydrated.SupplierAgreement.ChargeConditions.Equal(conditions) {
		t.Fatal("PCC-2 折回丢了条件")
	}

	relabelled := strings.Replace(string(pcc2.Document()), `"PCC-2"`, `"PCC-1"`, 1)
	if _, err := domain.RehydratePublicationContent("PCC-1", []byte(relabelled)); !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("标 PCC-1 却带条件 err = %v", err)
	}
	upgraded := strings.Replace(string(pcc1.Document()), `"PCC-1"`, `"PCC-2"`, 1)
	if _, err := domain.RehydratePublicationContent("PCC-2", []byte(upgraded)); !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("标 PCC-2 却不带条件 err = %v", err)
	}
	emptied := strings.Replace(string(pcc1.Document()), `"effectiveStartsAt":"2026-01-03T00:00:00Z"}`,
		`"effectiveStartsAt":"2026-01-03T00:00:00Z","chargeConditions":[]}`, 1)
	emptied = strings.Replace(emptied, `"PCC-1"`, `"PCC-2"`, 1)
	if _, err := domain.RehydratePublicationContent("PCC-2", []byte(emptied)); err == nil {
		t.Fatal("条件一节是空数组却折回了")
	}
	credit := canonicalCreditPolicy(t, creditPolicyBody(t, "freight", creditAmount(t, 100), time.Time{}))
	creditAsPCC2 := strings.Replace(string(credit.Document()), `"PCC-1"`, `"PCC-2"`, 1)
	if _, err := domain.RehydratePublicationContent("PCC-2", []byte(creditAsPCC2)); !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("信用政策标 PCC-2 err = %v", err)
	}
	if _, err := domain.RehydratePublicationContent("PCC-3", pcc2.Document()); !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("PCC-3 err = %v", err)
	}
}

// Covers: AT-PC-052——对账门：正文带条件、批文却抄了一个 PCC-1 的串，是输入自己不自洽（改批文抄算出的串），答不等，
// 不答「换构建」；本构建不认的版本才答不支持。
func TestADeclaredDigestOfTheOtherKnownShapeIsAMismatch(t *testing.T) {
	plain := supplierAgreementBody(t, "plan-buy-1", time.Time{})
	withConditions := plain
	conditions := trunkChargeConditions(t)
	withConditions.ChargeConditions = &conditions
	pcc1 := canonicalSupplierAgreement(t, plain)
	pcc2 := canonicalSupplierAgreement(t, withConditions)

	if err := domain.ReconcileDeclaredDigest(pcc2.Digest(), pcc2); err != nil {
		t.Fatalf("相等的串 err = %v", err)
	}
	if err := domain.ReconcileDeclaredDigest(pcc1.Digest(), pcc2); !errors.Is(err, domain.ErrDeclaredDigestMismatch) {
		t.Fatalf("抄了 PCC-1 的串 err = %v，want ErrDeclaredDigestMismatch", err)
	}
	unknown := commercialValue(t, domain.NewCommercialContentDigest, "PCC-9:"+strings.Repeat("0", 64))
	if err := domain.ReconcileDeclaredDigest(unknown, pcc2); !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("PCC-9 err = %v，want ErrCanonicalizationUnsupported", err)
	}
}

// Covers: AT-PC-052——三处共用一个原词翻译：不计费却带数量形、计费却不带、集外原因都拒，不丢掉那一格。
func TestChargeConditionWordsTranslateInOnePlace(t *testing.T) {
	if _, err := domain.ChargeOccurrenceConditionOf("ACTUAL_FULFILLMENT", true, "PER_SEGMENT"); err != nil {
		t.Fatalf("合法声明 err = %v", err)
	}
	for name, input := range map[string]struct {
		reason  string
		charged bool
		form    string
	}{
		"uncharged with a form": {"FAILED_ATTEMPT", false, "PER_ATTEMPT"},
		"charged without form":  {"ACTUAL_FULFILLMENT", true, ""},
		"outside reason":        {"DETENTION", true, "PER_PIECE"},
		"lower case":            {"actual_fulfillment", true, "PER_PIECE"},
		"outside form":          {"ACTUAL_FULFILLMENT", true, "PER_KG"},
		"form for another":      {"FAILED_ATTEMPT", true, "PER_SEGMENT"},
	} {
		if _, err := domain.ChargeOccurrenceConditionOf(input.reason, input.charged, input.form); !errors.Is(err, domain.ErrInvalidChargeOccurrenceCondition) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

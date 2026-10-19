package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证采用协议的收费发生条件在本上下文的样子（票 demo-chain-gaps/29，ADR-0209）：数量形数出数量与单位、按范围计
// 的三形可认、条件按原因各至多一条、按协议判的发生条件只带登记者。夹具全是合成串，证据只记 S。

func members(t *testing.T, objects ...string) []domain.CarriedObjectReference {
	t.Helper()
	references := make([]domain.CarriedObjectReference, 0, len(objects))
	for _, object := range objects {
		reference, err := domain.NewCarriedObjectReference(object)
		if err != nil {
			t.Fatal(err)
		}
		references = append(references, reference)
	}
	return references
}

// Covers: AT-TF-168——按件数成员，按段、按尝试、按订舱都是 1；单位由数量形定。零成员与集外数量形拒。
func TestEachQuantityFormMeasuresItsOccurrence(t *testing.T) {
	three := members(t, "SYN-PARCEL-01", "SYN-PARCEL-02", "SYN-PARCEL-03")
	for form, want := range map[domain.ChargeQuantityForm]struct {
		quantity int64
		unit     string
		scope    bool
	}{
		domain.PerPieceQuantity:   {3, "PIECE", false},
		domain.PerSegmentQuantity: {1, "SEGMENT", true},
		domain.PerAttemptQuantity: {1, "ATTEMPT", true},
		domain.PerBookingQuantity: {1, "BOOKING", true},
	} {
		quantity, unit, err := form.Measure(three)
		if err != nil || quantity != want.quantity || unit.String() != want.unit {
			t.Errorf("%s：%d/%q/%v，want %d/%q", form, quantity, unit, err, want.quantity, want.unit)
		}
		if form.CountsTheScope() != want.scope {
			t.Errorf("%s：按范围计 = %v", form, form.CountsTheScope())
		}
		if parsed, err := domain.ParseChargeQuantityForm(form.String()); err != nil || parsed != form {
			t.Errorf("%s 往返 = %s/%v", form, parsed, err)
		}
	}
	if _, _, err := domain.PerPieceQuantity.Measure(nil); !errors.Is(err, domain.ErrInvalidChargeOccurrence) {
		t.Errorf("零成员 err = %v", err)
	}
	if _, _, err := domain.ChargeQuantityFormInvalid.Measure(three); !errors.Is(err, domain.ErrInvalidChargeOccurrence) {
		t.Errorf("集外数量形 err = %v", err)
	}
	for _, outsider := range []string{"", "per_piece", "PER_KG"} {
		if _, err := domain.ParseChargeQuantityForm(outsider); err == nil {
			t.Errorf("集外词 %q 被认下了", outsider)
		}
	}
	units := map[string]bool{}
	for _, unit := range domain.ScopeCountingUnits() {
		units[unit.String()] = true
	}
	if len(units) != 3 || !units["SEGMENT"] || !units["ATTEMPT"] || !units["BOOKING"] || units["PIECE"] {
		t.Fatalf("按范围计的单位 = %v", units)
	}
}

// Covers: AT-TF-166 / 167——条件按原因各至多一条，没声明的原因读不到；计费带数量形、不计费不带。
func TestAgreementChargeConditionsArePerReason(t *testing.T) {
	charged, err := domain.NewChargedAgreementCondition(domain.PerSegmentQuantity)
	if err != nil {
		t.Fatal(err)
	}
	conditions, err := domain.AgreementChargeConditions{}.DeclareAgreementChargeCondition(domain.ActualFulfillmentOccurrence, charged)
	if err != nil {
		t.Fatal(err)
	}
	if conditions, err = conditions.DeclareAgreementChargeCondition(domain.FailedAttemptOccurrence, domain.NewUnchargedAgreementCondition()); err != nil {
		t.Fatal(err)
	}
	if _, err := conditions.DeclareAgreementChargeCondition(domain.ActualFulfillmentOccurrence, charged); !errors.Is(err, domain.ErrInvalidChargeOccurrence) {
		t.Fatalf("同一原因两条 err = %v", err)
	}
	if _, err := conditions.DeclareAgreementChargeCondition(domain.ChargeOccurrenceReasonInvalid, charged); err == nil {
		t.Fatal("集外原因被收下了")
	}
	if _, err := domain.NewChargedAgreementCondition(domain.ChargeQuantityFormInvalid); err == nil {
		t.Fatal("计费却没有数量形被收下了")
	}
	if fulfillment, declared := conditions.For(domain.ActualFulfillmentOccurrence); !declared || !fulfillment.Charged() {
		t.Fatal("实际履约应声明计费")
	}
	if attempt, declared := conditions.For(domain.FailedAttemptOccurrence); !declared || attempt.Charged() {
		t.Fatal("失败尝试应声明不计费")
	} else if _, carried := attempt.QuantityForm(); carried {
		t.Fatal("不计费的声明带着数量形")
	}
	if _, declared := conditions.For(domain.BookingOccurrence); declared {
		t.Fatal("没声明的订舱读出了一条")
	}
	if _, declared := (domain.AgreementChargeConditions{}).For(domain.ActualFulfillmentOccurrence); declared {
		t.Fatal("零值读出了一条")
	}
}

// Covers: ADR-0209 决定五——按协议判的发生条件只带登记者，没有断言人与原话；登记者缺席即拒；断言项的登记者就是断言人。
func TestAnAgreementDeclaredConditionCarriesOnlyItsRegistrant(t *testing.T) {
	registrant, err := domain.NewRegistrantReference("https://dex.syn.example#SYN-OPERATOR-CONFIG-01")
	if err != nil {
		t.Fatal(err)
	}
	condition, err := domain.NewAgreementDeclaredOccurrenceCondition(registrant)
	if err != nil {
		t.Fatal(err)
	}
	if condition.Basis() != domain.ConditionDeclaredByAgreement || condition.Basis().String() != "AGREEMENT_DECLARED" ||
		condition.RegisteredBy() != registrant {
		t.Fatalf("condition = %+v", condition)
	}
	if _, asserted := condition.AssertedBy(); asserted {
		t.Fatal("按协议判的依据读出了断言人")
	}
	if _, asserted := condition.Assertion(); asserted {
		t.Fatal("按协议判的依据读出了原话")
	}
	if _, err := domain.NewAgreementDeclaredOccurrenceCondition(domain.RegistrantReference{}); err == nil {
		t.Fatal("没有登记者被收下了")
	}

	asserted, err := domain.NewAssertedOccurrenceCondition(registrant, "断言期的原话")
	if err != nil {
		t.Fatal(err)
	}
	if asserted.RegisteredBy() != registrant {
		t.Fatal("断言项的登记者应就是断言人")
	}
}

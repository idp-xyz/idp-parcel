package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var adoptionAt = time.Date(2026, 9, 11, 8, 0, 0, 0, time.UTC)

func verificationReference(t *testing.T) domain.DutyPaymentVerificationReference {
	t.Helper()
	reference, err := domain.NewDutyPaymentVerificationReference(
		mustValue(t, domain.NewDeclarationScopeReference, "SYN-UNIT-01"),
		mustValue(t, domain.NewTaxObligationReference, "duty-1"),
		mustValue(t, domain.NewFundsFactReference, "bank-fact-1"),
		mustValue(t, domain.NewDutyVerificationVersion, "digest-1"),
	)
	if err != nil {
		t.Fatalf("核对引用：%v", err)
	}
	return reference
}

// Covers: 引用四维合起来才指得到一版核对——少任一维都构造不出引用（UC-SA-001 步 2「采用明确版本」）。
func TestAVerificationReferenceNeedsAllFourDimensions(t *testing.T) {
	scope := mustValue(t, domain.NewDeclarationScopeReference, "SYN-UNIT-01")
	duty := mustValue(t, domain.NewTaxObligationReference, "duty-1")
	funds := mustValue(t, domain.NewFundsFactReference, "bank-fact-1")
	version := mustValue(t, domain.NewDutyVerificationVersion, "digest-1")

	cases := map[string]struct {
		scope   domain.DeclarationScopeReference
		duty    domain.TaxObligationReference
		funds   domain.FundsFactReference
		version domain.DutyVerificationVersion
	}{
		"缺申报范围": {duty: duty, funds: funds, version: version},
		"缺税费义务": {scope: scope, funds: funds, version: version},
		"缺资金事实": {scope: scope, duty: duty, version: version},
		"缺版本指纹": {scope: scope, duty: duty, funds: funds},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NewDutyPaymentVerificationReference(test.scope, test.duty, test.funds, test.version)
			if !errors.Is(err, domain.ErrInvalidDutyPaymentVerificationReference) {
				t.Fatalf("err = %v, want ErrInvalidDutyPaymentVerificationReference", err)
			}
		})
	}

	reference, err := domain.NewDutyPaymentVerificationReference(scope, duty, funds, version)
	if err != nil {
		t.Fatalf("四维齐全仍拒：%v", err)
	}
	if reference.Scope().String() != "SYN-UNIT-01" || reference.Duty().String() != "duty-1" ||
		reference.Funds().String() != "bank-fact-1" || reference.Version().String() != "digest-1" {
		t.Fatalf("引用四维没有原样保留：%+v", reference)
	}
}

// Covers: 采用只要引用与时刻——它不带裁决、金额或三态，类型上就分得开「输入已接收」与「判断已形成」。
func TestAdoptingAVerificationKeepsOnlyTheReferenceAndTheInstant(t *testing.T) {
	adoption, err := domain.AdoptDutyPaymentVerification(verificationReference(t), adoptionAt.In(time.FixedZone("X", 3600)))
	if err != nil {
		t.Fatalf("采用：%v", err)
	}
	if adoption.Verification() != verificationReference(t) {
		t.Fatalf("引用没有原样保留：%+v", adoption.Verification())
	}
	if !adoption.AdoptedAt().Equal(adoptionAt) || adoption.AdoptedAt().Location() != time.UTC {
		t.Fatalf("采用时刻 = %v, want %v（UTC）", adoption.AdoptedAt(), adoptionAt)
	}
}

func TestAdoptingAVerificationRejectsAMissingReferenceOrInstant(t *testing.T) {
	if _, err := domain.AdoptDutyPaymentVerification(domain.DutyPaymentVerificationReference{}, adoptionAt); !errors.Is(err, domain.ErrInvalidDutyPaymentVerificationAdoption) {
		t.Fatalf("零值引用：err = %v", err)
	}
	if _, err := domain.AdoptDutyPaymentVerification(verificationReference(t), time.Time{}); !errors.Is(err, domain.ErrInvalidDutyPaymentVerificationAdoption) {
		t.Fatalf("零值时刻：err = %v", err)
	}
}

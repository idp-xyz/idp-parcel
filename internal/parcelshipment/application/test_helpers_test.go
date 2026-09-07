package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

type stringValue interface {
	String() string
}

func mustValue[T stringValue](t *testing.T, constructor func(string) (T, error), value string) T {
	t.Helper()
	got, err := constructor(value)
	if err != nil {
		t.Fatalf("construct %q: %v", value, err)
	}
	return got
}

func sourceIdentity(t *testing.T, tenant, customer, source, key string) domain.SourceIdentity {
	t.Helper()
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, tenant),
		mustValue(t, domain.NewCustomerAccountID, customer),
		mustValue(t, domain.NewSource, source),
		mustValue(t, domain.NewSourceRequestKey, key),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return identity
}

// financialControlOf 造一份结论为 outcome 的接受前财务控制采用结果。结论只能由逐项推出（ADR-0125），
// 夹具反过来按结论挑逐项：`HELD` 一项预付冻结成立、`CREDIT_EXPOSED` 一项信用校验成立、`RESTRICTED`
// 一项预付冻结受限（原因 PC-CONTROL-BASIS-1）、`NOT_APPLICABLE` 走无控制入口带同名依据；resultID 传空即
// 缺席（只有无控制那一支允许）。
func financialControlOf(
	t *testing.T,
	outcome domain.FinancialControlOutcome,
	resultID string,
	asOf domain.JudgmentAsOf,
) domain.FinancialControlResult {
	t.Helper()
	basis := mustValue(t, domain.NewControlBasisReference, "PC-CONTROL-BASIS-1")
	if outcome == domain.FinancialControlNotApplicable {
		result, err := domain.NewInapplicableFinancialControlResult(basis, asOf)
		if err != nil {
			t.Fatalf("new inapplicable financial control result: %v", err)
		}
		return result
	}
	var item domain.ControlItemResult
	var err error
	switch outcome {
	case domain.FinancialControlHeld:
		item, err = domain.NewControlItemResult(
			domain.PrepaidFreezeControlItem, 1, domain.ControlItemSatisfied, domain.ControlBasisReference{})
	case domain.FinancialControlCreditExposed:
		item, err = domain.NewControlItemResult(
			domain.CreditCheckControlItem, 1, domain.ControlItemSatisfied, domain.ControlBasisReference{})
	case domain.FinancialControlRestricted:
		item, err = domain.NewControlItemResult(
			domain.PrepaidFreezeControlItem, 1, domain.ControlItemRestricted, basis)
	default:
		t.Fatalf("no financial control fixture for %q", outcome)
	}
	if err != nil {
		t.Fatalf("new control item result: %v", err)
	}
	result, err := domain.NewExecutedFinancialControlResult(domain.ExecutedFinancialControlSpec{
		ResultID:  mustValue(t, domain.NewFinancialControlResultID, resultID),
		Items:     []domain.ControlItemResult{item},
		JointPass: domain.AllControlsPass,
		AsOf:      asOf,
	})
	if err != nil {
		t.Fatalf("new executed financial control result: %v", err)
	}
	return result
}

func validity(t *testing.T) domain.OwnershipValidityInterval {
	t.Helper()
	interval, err := domain.NewOwnershipValidityInterval(
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new validity interval: %v", err)
	}
	return interval
}

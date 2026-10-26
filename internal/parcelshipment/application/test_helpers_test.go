package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
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
		Scope:     occupationScopeOf(t, resultID),
	})
	if err != nil {
		t.Fatalf("new executed financial control result: %v", err)
	}
	return result
}

// occupationScopeOf 是夹具里已执行结果存下的作用域。账户段带结果标识，好让「释放交的是这一笔存下的那一份」在逐版
// 释放时也验得出来。
func occupationScopeOf(t *testing.T, resultID string) domain.OccupationScope {
	t.Helper()
	scope, err := domain.NewOccupationScope("SYN-LEGAL-ENTITY-1", "SYN-ACCOUNT-"+resultID, "CNY")
	if err != nil {
		t.Fatalf("new occupation scope: %v", err)
	}
	return scope
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

// conflictRegisterDouble 是取消先后冲突册的替身：按（租户，包裹，边界种类，边界引用，取消）去重，
// 与真册的主键同一组维度。
type conflictRegisterDouble struct {
	records []ports.CancellationOrderConflictRecord
	err     error
}

func (double *conflictRegisterDouble) Record(
	_ context.Context,
	record ports.CancellationOrderConflictRecord,
) (ports.CancellationOrderConflictSaveOutcome, error) {
	if double.err != nil {
		return ports.CancellationOrderConflictSaveOutcomeInvalid, double.err
	}
	for _, existing := range double.records {
		if existing.TenantID == record.TenantID &&
			existing.Conflict.Parcel() == record.Conflict.Parcel() &&
			existing.Conflict.Boundary().Kind == record.Conflict.Boundary().Kind &&
			existing.Conflict.Boundary().Reference == record.Conflict.Boundary().Reference &&
			existing.Conflict.Cancellation() == record.Conflict.Cancellation() {
			return ports.CancellationOrderConflictAlreadyRecorded, nil
		}
	}
	double.records = append(double.records, record)
	return ports.CancellationOrderConflictRecorded, nil
}

func (double *conflictRegisterDouble) ListByParcel(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) ([]ports.CancellationOrderConflictEntry, error) {
	if double.err != nil {
		return nil, double.err
	}
	var entries []ports.CancellationOrderConflictEntry
	for _, record := range double.records {
		if record.TenantID != tenant || record.Conflict.Parcel() != parcel {
			continue
		}
		entries = append(entries, ports.CancellationOrderConflictEntry{
			TenantID:     record.TenantID,
			Parcel:       record.Conflict.Parcel(),
			Cancellation: record.Conflict.Cancellation(),
			CancelledAt:  record.Conflict.CancelledAt(),
			Boundary:     record.Conflict.Boundary(),
			DetectedAt:   record.DetectedAt,
		})
	}
	return entries, nil
}

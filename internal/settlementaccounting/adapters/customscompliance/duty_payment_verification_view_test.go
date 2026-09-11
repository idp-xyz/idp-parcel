package customscompliance_test

import (
	"context"
	"errors"
	"testing"

	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/customscompliance"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// dutyVerificationReaderDouble 是提供方只读半边的替身：记下被问的键，按预设答在不在。
type dutyVerificationReaderDouble struct {
	asked []ccports.DutyVerificationKey
	found bool
	err   error
}

func (double *dutyVerificationReaderDouble) FindVerification(
	_ context.Context, key ccports.DutyVerificationKey,
) (ccports.DutyVerificationRecord, bool, error) {
	double.asked = append(double.asked, key)
	if double.err != nil {
		return ccports.DutyVerificationRecord{}, false, double.err
	}
	return ccports.DutyVerificationRecord{Key: key}, double.found, nil
}

func saReference(t *testing.T) (sadomain.TenantID, sadomain.DutyPaymentVerificationReference) {
	t.Helper()
	tenant, err := sadomain.NewTenantID("tenant-a")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	scope, _ := sadomain.NewDeclarationScopeReference("SYN-UNIT-01")
	duty, _ := sadomain.NewTaxObligationReference("duty-1")
	funds, _ := sadomain.NewFundsFactReference("bank-fact-1")
	version, _ := sadomain.NewDutyVerificationVersion("digest-1")
	reference, err := sadomain.NewDutyPaymentVerificationReference(scope, duty, funds, version)
	if err != nil {
		t.Fatalf("引用：%v", err)
	}
	return tenant, reference
}

// Covers: 做法 2 与票 03 裁决同口径——按键取（租户 + 范围 + 税费 + 资金 + 指纹）向提供方只读口问信封所指
// 的那一版；四维与租户一对一译进提供方幂等键，指纹落在 Digest 上、不取 latest。
func TestTheViewAsksTheProviderForExactlyTheReferencedVersion(t *testing.T) {
	reader := &dutyVerificationReaderDouble{found: true}
	view, err := adapter.NewCustomsDutyPaymentVerificationView(reader)
	if err != nil {
		t.Fatalf("构造视图：%v", err)
	}
	tenant, reference := saReference(t)

	visible, err := view.DutyPaymentVerificationExists(t.Context(), tenant, reference)
	if err != nil || !visible {
		t.Fatalf("visible = %v err = %v, want true / nil", visible, err)
	}
	if len(reader.asked) != 1 {
		t.Fatalf("问了 %d 次, want 1", len(reader.asked))
	}
	key := reader.asked[0]
	if key.TenantID.String() != "tenant-a" || key.Scope.String() != "SYN-UNIT-01" || key.Duty.String() != "duty-1" ||
		key.Funds.String() != "bank-fact-1" || key.Digest != "digest-1" {
		t.Fatalf("提供方被问的键 = %+v，want 五维一对一", key)
	}
}

func TestTheViewPassesAbsenceThroughAndWrapsReaderFailures(t *testing.T) {
	tenant, reference := saReference(t)

	absent := &dutyVerificationReaderDouble{found: false}
	view, _ := adapter.NewCustomsDutyPaymentVerificationView(absent)
	if visible, err := view.DutyPaymentVerificationExists(t.Context(), tenant, reference); err != nil || visible {
		t.Fatalf("提供方没有那一版：visible = %v err = %v, want false / nil", visible, err)
	}

	failing := &dutyVerificationReaderDouble{err: errors.New("cc down")}
	view, _ = adapter.NewCustomsDutyPaymentVerificationView(failing)
	if _, err := view.DutyPaymentVerificationExists(t.Context(), tenant, reference); err == nil {
		t.Fatal("读口故障应作为错误交回，不折成「没有」")
	}
}

func TestTheViewRefusesANilReader(t *testing.T) {
	if _, err := adapter.NewCustomsDutyPaymentVerificationView(nil); err == nil {
		t.Fatal("nil reader 被收下了")
	}
}

package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var convertedAt = frozenAt.Add(time.Hour)

// Covers: SA CONTEXT「冻结后必须转为正式扣费或显式释放」——转出的金额离开占用（HeldMinor 随之下降），转满即 CONVERTED；
// 部分转出时仍占着余额；转满的冻结没有可放的东西，Release 原样交回而不是把正式扣费的钱当成放掉。
func TestAFreezeConvertsToChargeInPartsAndStopsOccupyingWhatWasConverted(t *testing.T) {
	ledger := domain.NewFreezeLedger()
	request := freezeRequest(t, "request-1", 9600)
	if _, err := ledger.Freeze(request, balance(t, 20000, 0, 0, 0)); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if got := ledger.HeldMinor(); got != 9600 {
		t.Fatalf("held = %d, want 9600", got)
	}

	partial, err := ledger.Convert(request.RequestID(), 4000, convertedAt)
	if err != nil {
		t.Fatalf("partial convert: %v", err)
	}
	if partial.Status() != domain.FreezeHeld || partial.RemainingMinor() != 5600 || partial.ConvertedMinor() != 4000 {
		t.Fatalf("partial = %s remaining %d converted %d, want HELD 5600 4000",
			partial.Status(), partial.RemainingMinor(), partial.ConvertedMinor())
	}
	if got := ledger.HeldMinor(); got != 5600 {
		t.Fatalf("held after partial = %d, want 5600", got)
	}

	full, err := ledger.Convert(request.RequestID(), 5600, convertedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("convert the rest: %v", err)
	}
	if full.Status() != domain.FreezeConverted || ledger.HeldMinor() != 0 {
		t.Fatalf("after full: %s held %d, want CONVERTED 0", full.Status(), ledger.HeldMinor())
	}

	released, err := ledger.Release(full.FreezeID(), convertedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("release a converted freeze: %v", err)
	}
	if released.Status() != domain.FreezeConverted || !released.ReleasedAt().IsZero() {
		t.Fatalf("release turned a converted freeze into %s (released at %v)", released.Status(), released.ReleasedAt())
	}
	if _, err := ledger.Convert(request.RequestID(), 1, convertedAt.Add(2*time.Hour)); !errors.Is(err, domain.ErrInvalidConversion) {
		t.Fatalf("convert beyond what was frozen: err = %v, want ErrInvalidConversion", err)
	}
}

// 转出不合规则时不截断也不静默放过：金额非正、超出余额、早于冻结、未知请求。释放部分转出的冻结只放余额。
func TestAFreezeRefusesAnIllFormedConversionAndReleasesOnlyWhatIsLeft(t *testing.T) {
	ledger := domain.NewFreezeLedger()
	request := freezeRequest(t, "request-1", 1000)
	if _, err := ledger.Freeze(request, balance(t, 5000, 0, 0, 0)); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	for name, attempt := range map[string]func() error{
		"zero":   func() error { _, err := ledger.Convert(request.RequestID(), 0, convertedAt); return err },
		"beyond": func() error { _, err := ledger.Convert(request.RequestID(), 1001, convertedAt); return err },
		"before": func() error {
			_, err := ledger.Convert(request.RequestID(), 10, frozenAt.Add(-time.Second))
			return err
		},
		"zero value": func() error { _, err := ledger.Convert(request.RequestID(), 10, time.Time{}); return err },
	} {
		if err := attempt(); !errors.Is(err, domain.ErrInvalidConversion) {
			t.Fatalf("%s: err = %v, want ErrInvalidConversion", name, err)
		}
	}
	other, _ := domain.NewControlRequestID("never-frozen")
	if _, err := ledger.Convert(other, 10, convertedAt); !errors.Is(err, domain.ErrFreezeNotFound) {
		t.Fatalf("unknown request: err = %v, want ErrFreezeNotFound", err)
	}

	if _, err := ledger.Convert(request.RequestID(), 300, convertedAt); err != nil {
		t.Fatalf("convert: %v", err)
	}
	found, _ := ledger.FindByRequest(request.RequestID())
	released, err := ledger.Release(found.FreezeID(), convertedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Status() != domain.FreezeReleased || released.ConvertedMinor() != 300 || ledger.HeldMinor() != 0 {
		t.Fatalf("released = %s converted %d held %d, want RELEASED 300 0（已转出的 300 是正式扣费，不随释放回流）",
			released.Status(), released.ConvertedMinor(), ledger.HeldMinor())
	}
}

// 暴露账本的转出与冻结账本同代数、另一份实现：转出的部分不再占暴露额度，转满即 CONVERTED。
func TestAnExposureConvertsToConfirmedUnsettledAndStopsOccupyingTheCredit(t *testing.T) {
	scope := settlementScope(t, "legal-1", "account-1", "CNY")
	policy := creditPolicyReference(t, "credit-1/v1")
	standing, err := domain.NewCreditStanding(scope, 20000, 0, false)
	if err != nil {
		t.Fatalf("standing: %v", err)
	}
	standing, err = standing.WithAuthorizedLimit(20000, policy)
	if err != nil {
		t.Fatalf("authorized: %v", err)
	}
	ledger := domain.NewCreditExposureLedger()
	request := exposureRequestFor(t, scope, "control-1", 9600)
	if _, err := ledger.Expose(request, standing); err != nil {
		t.Fatalf("expose: %v", err)
	}
	if got := ledger.ExposedMinor(); got != 9600 {
		t.Fatalf("exposed = %d, want 9600", got)
	}

	partial, err := ledger.Convert(request.RequestID(), 3600, convertedAt)
	if err != nil {
		t.Fatalf("partial: %v", err)
	}
	if partial.Status() != domain.ExposureRecorded || ledger.ExposedMinor() != 6000 {
		t.Fatalf("partial = %s exposed %d, want RECORDED 6000", partial.Status(), ledger.ExposedMinor())
	}
	full, err := ledger.Convert(request.RequestID(), 6000, convertedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("rest: %v", err)
	}
	if full.Status() != domain.ExposureConverted || ledger.ExposedMinor() != 0 {
		t.Fatalf("full = %s exposed %d, want CONVERTED 0", full.Status(), ledger.ExposedMinor())
	}
	released, err := ledger.Release(full.ExposureID(), convertedAt.Add(time.Hour))
	if err != nil || released.Status() != domain.ExposureConverted {
		t.Fatalf("release of a converted exposure = %v / %v, want unchanged CONVERTED", released.Status(), err)
	}
	if _, err := ledger.Convert(request.RequestID(), 1, convertedAt.Add(2*time.Hour)); !errors.Is(err, domain.ErrInvalidConversion) {
		t.Fatalf("convert beyond: err = %v, want ErrInvalidConversion", err)
	}
}

// 重建门核转出两格互相印证，不放过库里写坏的行。
func TestRehydratingAConvertedFreezeChecksTheConversionColumns(t *testing.T) {
	scope := settlementScope(t, "legal-1", "account-1", "SYN")
	requestID, _ := domain.NewControlRequestID("request-1")
	association, _ := domain.NewBusinessAssociationReference("association-1")
	spec := func(status domain.FreezeStatus, converted int64, at time.Time) domain.RehydrateFundsFreezeSpec {
		return domain.RehydrateFundsFreezeSpec{
			FreezeID: "FRZ-0001", RequestID: requestID, Scope: scope, AmountMinor: 1000, Association: association,
			Status: status, FrozenAt: frozenAt, Digest: "d", ConvertedMinor: converted, ConvertedAt: at,
		}
	}
	good := []domain.RehydrateFundsFreezeSpec{
		spec(domain.FreezeHeld, 0, time.Time{}),
		spec(domain.FreezeHeld, 400, convertedAt),
		spec(domain.FreezeConverted, 1000, convertedAt),
	}
	for _, one := range good {
		if _, err := domain.RehydrateFreezeLedger([]domain.RehydrateFundsFreezeSpec{one}); err != nil {
			t.Fatalf("rehydrate %s/%d: %v", one.Status, one.ConvertedMinor, err)
		}
	}
	bad := []domain.RehydrateFundsFreezeSpec{
		spec(domain.FreezeHeld, 1000, convertedAt),               // 转满却仍占着
		spec(domain.FreezeConverted, 400, convertedAt),           // 没转满却标转满
		spec(domain.FreezeHeld, 400, time.Time{}),                // 有金额无时刻
		spec(domain.FreezeHeld, 0, convertedAt),                  // 有时刻无金额
		spec(domain.FreezeHeld, 400, frozenAt.Add(-time.Second)), // 早于冻结
		spec(domain.FreezeHeld, 1001, convertedAt),               // 超过原金额
	}
	for index, one := range bad {
		if _, err := domain.RehydrateFreezeLedger([]domain.RehydrateFundsFreezeSpec{one}); !errors.Is(err, domain.ErrInvalidRehydratedLedger) {
			t.Fatalf("bad[%d]: err = %v, want ErrInvalidRehydratedLedger", index, err)
		}
	}
}

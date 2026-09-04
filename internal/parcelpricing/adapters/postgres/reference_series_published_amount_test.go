package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// TestPublishedAmountSeriesRegistersAndResolvesWithItsCurrency 对真库证第三种序列（ADR-0110）过得了库层的种类
// 封闭集（迁移 0006 扩的那一格）且币种随快照往返：解析出的取值是带币种的金额。
func TestPublishedAmountSeriesRegistersAndResolvesWithItsCurrency(t *testing.T) {
	register, transactor, _ := newSeriesRegister(t)
	ctx := t.Context()

	period, err := domain.NewSeriesPeriodValue(registerWeekOne, registerWeekTwo, evaluationValue(t, domain.ParseDecimal, "3.5"), "SYN-EVIDENCE/pss-W32")
	if err != nil {
		t.Fatalf("期次：%v", err)
	}
	registration, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           evaluationValue(t, domain.NewTenantID, "tenant-pss"),
		Kind:             domain.ReferenceSeriesPublishedAmount,
		Reference:        seriesReference(t, "SYN-PRC-PSS", "v1"),
		SourceIdentifier: "SYN-CARRIER/peak-bulletin",
		Registrant:       "SYN-PRC-SERIES-REGISTRAR",
		Currency:         evaluationValue(t, domain.NewCurrency, "USD"),
		Periods:          []domain.SeriesPeriodValue{period},
	})
	if err != nil {
		t.Fatalf("构造金额序列登记：%v", err)
	}
	if outcome := registerSeries(t, register, transactor, ctx, registration); outcome != ports.ReferenceSeriesRegistered {
		t.Fatalf("首登 outcome = %d, 想要 ReferenceSeriesRegistered", outcome)
	}

	reading, found, err := register.ResolveAt(ctx, evaluationValue(t, domain.NewTenantID, "tenant-pss"), seriesReference(t, "SYN-PRC-PSS", "v1"), registerWeekOne.Add(time.Hour))
	if err != nil || !found {
		t.Fatalf("解析：found=%v err=%v", found, err)
	}
	amount, ok := reading.Value().Amount()
	if !ok || amount.Amount().String() != "3.5" || amount.Currency().String() != "USD" {
		t.Fatalf("解析出的金额变形：%v %v", amount, ok)
	}
	if _, found, err := register.ResolveAt(ctx, evaluationValue(t, domain.NewTenantID, "tenant-pss"), seriesReference(t, "SYN-PRC-PSS", "v1"), registerWeekTwo.Add(time.Hour)); err != nil || found {
		t.Fatalf("窗外时点不该有期次：found=%v err=%v", found, err)
	}
}

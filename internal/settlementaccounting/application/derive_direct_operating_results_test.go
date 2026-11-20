package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestDirectOperatingResultsUseTheExistingDerive(t *testing.T) {
	tenant := directTenant(t)
	parcel := directParcel(t, "parcel-1")
	facts := &directFacts{byParcel: map[string]ports.ParcelDirectFacts{
		parcel.String(): {
			ServiceForm: ports.ServiceFormLabelChannel,
			Estimates: []ports.DirectAmount{
				{Source: "charge-1", Currency: "CNY", AmountMinor: 10000, Period: "2026-11-20", PeriodStated: true},
			},
			ExpectedCosts: []ports.DirectAmount{
				{Source: "cost-v1", Currency: "CNY", AmountMinor: 4000},
			},
			Receivables: []ports.DirectAmount{
				{Source: "charge-1", Currency: "CNY", AmountMinor: 10000, Period: "2026-11-20", PeriodStated: true},
			},
			Payables: []ports.DirectAmount{
				{Source: "payable-1", Currency: "CNY", AmountMinor: 4500, Period: "2026-11-20", PeriodStated: true},
			},
			Credits: []ports.DirectAmount{
				{Source: "credit-1@v1", Currency: "CNY", AmountMinor: 500, Period: "2026-11-20", PeriodStated: true},
			},
		},
	}}
	handler, results, dispositions := directHandler(t, facts, true, true)

	if err := handler.AfterParcel(context.Background(), tenant, parcel); err != nil {
		t.Fatalf("派生：%v", err)
	}
	if results.saves != 2 {
		t.Fatalf("两种口径应各派生一次，saves=%d", results.saves)
	}
	estimated := directStored(t, results, tenant, parcel.String(), domain.EstimatedBasis)
	_, margin := estimated.Result.Margin()
	if margin != 6000 || len(estimated.Result.Components()) != 2 {
		t.Fatalf("预估毛利或组成 = %d / %d", margin, len(estimated.Result.Components()))
	}
	confirmed := directStored(t, results, tenant, parcel.String(), domain.ConfirmedBasis)
	_, margin = confirmed.Result.Margin()
	if margin != 6000 || len(confirmed.Result.Components()) != 3 {
		t.Fatalf("已确认毛利应是应收减应付再加贷项，margin=%d components=%d", margin, len(confirmed.Result.Components()))
	}
	if dispositions.cleared != 2 {
		t.Fatalf("派生成功应撤下答不出的答案，cleared=%d", dispositions.cleared)
	}

	if err := handler.AfterParcel(context.Background(), tenant, parcel); err != nil {
		t.Fatalf("同内容再来：%v", err)
	}
	if results.saves != 2 {
		t.Fatalf("同内容不应再写一份快照，saves=%d", results.saves)
	}

	facts.byParcel[parcel.String()].ExpectedCosts[0].AmountMinor = 7000
	facts.byParcel[parcel.String()].ExpectedCosts[0].Source = "cost-v2"
	if err := handler.AfterParcel(context.Background(), tenant, parcel); err != nil {
		t.Fatalf("重派生：%v", err)
	}
	revised := directStored(t, results, tenant, parcel.String(), domain.EstimatedBasis)
	corrects, ok := revised.Result.Corrects()
	_, margin = revised.Result.Margin()
	if !ok || corrects != estimated.Result.Version() || margin != 3000 || revised.Result.AsOf() != estimated.Result.AsOf() {
		t.Fatalf("新版本应回指原截点且原截至时点还在：corrects=%s ok=%v margin=%d asOf=%s", corrects, ok, margin, revised.Result.AsOf())
	}
}

func TestDirectOperatingResultsDoNotInventAMargin(t *testing.T) {
	tenant := directTenant(t)
	parcel := directParcel(t, "parcel-1")
	customer := []ports.DirectAmount{{Source: "charge-1", Currency: "CNY", AmountMinor: 10000, Period: "2026-11-20", PeriodStated: true}}

	t.Run("没采用", func(t *testing.T) {
		facts := &directFacts{byParcel: map[string]ports.ParcelDirectFacts{
			parcel.String(): {ServiceForm: ports.ServiceFormLabelChannel, Estimates: customer, ExpectedCosts: []ports.DirectAmount{{Source: "cost", Currency: "CNY", AmountMinor: 1}}},
		}}
		handler, results, dispositions := directHandler(t, facts, false, false)
		if err := handler.AfterParcel(context.Background(), tenant, parcel); err != nil {
			t.Fatal(err)
		}
		if results.saves != 0 || dispositions.saved != "" {
			t.Fatalf("没采用仍写了答案 saves=%d disposition=%s", results.saves, dispositions.saved)
		}
	})

	t.Run("只有客户侧", func(t *testing.T) {
		facts := &directFacts{byParcel: map[string]ports.ParcelDirectFacts{
			parcel.String(): {ServiceForm: ports.ServiceFormLabelChannel, Estimates: customer, Receivables: customer},
		}}
		handler, results, dispositions := directHandler(t, facts, true, true)
		if err := handler.AfterParcel(context.Background(), tenant, parcel); err != nil {
			t.Fatal(err)
		}
		if results.saves != 0 || dispositions.saved != application.DirectOperatingIndeterminate.String() {
			t.Fatalf("只有客户侧应判不出且不出毛利，saves=%d disposition=%s", results.saves, dispositions.saved)
		}
	})

	t.Run("网络服务产品", func(t *testing.T) {
		facts := &directFacts{byParcel: map[string]ports.ParcelDirectFacts{
			parcel.String(): {
				ServiceForm:   ports.ServiceFormNetworkService,
				Estimates:     customer,
				ExpectedCosts: []ports.DirectAmount{{Source: "cost", Currency: "CNY", AmountMinor: 1000}},
				Receivables:   customer,
				Payables:      []ports.DirectAmount{{Source: "payable", Currency: "CNY", AmountMinor: 1000, Period: "2026-11-20", PeriodStated: true}},
			},
		}}
		handler, results, dispositions := directHandler(t, facts, true, true)
		if err := handler.AfterParcel(context.Background(), tenant, parcel); err != nil {
			t.Fatal(err)
		}
		if results.saves != 0 || dispositions.saved != application.DirectOperatingIncompleteCost.String() {
			t.Fatalf("网络服务产品应成本不全，saves=%d disposition=%s", results.saves, dispositions.saved)
		}
	})

	t.Run("币种不同", func(t *testing.T) {
		facts := &directFacts{byParcel: map[string]ports.ParcelDirectFacts{
			parcel.String(): {
				ServiceForm:   ports.ServiceFormLabelChannel,
				Estimates:     customer,
				ExpectedCosts: []ports.DirectAmount{{Source: "cost", Currency: "USD", AmountMinor: 1000}},
			},
		}}
		handler, results, dispositions := directHandler(t, facts, true, false)
		if err := handler.AfterParcel(context.Background(), tenant, parcel); err != nil {
			t.Fatal(err)
		}
		if results.saves != 0 || dispositions.saved != application.DirectOperatingPendingJudgment.String() {
			t.Fatalf("币种不同应待判断，saves=%d disposition=%s", results.saves, dispositions.saved)
		}
	})

	t.Run("供应商账期不同", func(t *testing.T) {
		facts := &directFacts{byParcel: map[string]ports.ParcelDirectFacts{
			parcel.String(): {
				ServiceForm: ports.ServiceFormLabelChannel,
				Receivables: customer,
				Payables:    []ports.DirectAmount{{Source: "payable", Currency: "CNY", AmountMinor: 1000, Period: "2026-12-01", PeriodStated: true}},
			},
		}}
		handler, results, dispositions := directHandler(t, facts, false, true)
		if err := handler.AfterParcel(context.Background(), tenant, parcel); err != nil {
			t.Fatal(err)
		}
		if results.saves != 0 || dispositions.saved != application.DirectOperatingPendingJudgment.String() {
			t.Fatalf("账期不同应待判断，saves=%d disposition=%s", results.saves, dispositions.saved)
		}
	})
}

func TestDirectOperatingDispositionNamesEveryNonZeroValue(t *testing.T) {
	for _, disposition := range []application.DirectOperatingDisposition{
		application.DirectOperatingIndeterminate,
		application.DirectOperatingIncompleteCost,
		application.DirectOperatingPendingJudgment,
	} {
		if disposition.String() == "" {
			t.Fatalf("%d 没有词", disposition)
		}
	}
	if application.DirectOperatingDispositionInvalid.String() != "" {
		t.Fatal("零值应没有词")
	}
}

type directClock struct{ at time.Time }

func (clock directClock) Now() time.Time { return clock.at }

type directAdoption struct {
	adoption domain.OperatingBasisAdoption
	found    bool
}

func (book directAdoption) FindOperatingBasisAdoption(context.Context, domain.TenantID) (domain.OperatingBasisAdoption, bool, error) {
	return book.adoption, book.found, nil
}

func (book directAdoption) SaveOperatingBasisAdoption(context.Context, domain.TenantID, domain.OperatingBasisAdoption, time.Time) (ports.CatalogueRegistrationEffect, error) {
	return 0, nil
}

type directFacts struct {
	byParcel map[string]ports.ParcelDirectFacts
}

func (facts *directFacts) LoadParcelDirectFacts(_ context.Context, _ domain.TenantID, parcel domain.ParcelReference) (ports.ParcelDirectFacts, error) {
	return facts.byParcel[parcel.String()], nil
}

func (facts *directFacts) ParcelsOfCharge(context.Context, domain.TenantID, domain.CustomerChargeID) ([]domain.ParcelReference, error) {
	return nil, nil
}

func (facts *directFacts) ParcelsOfOccurrence(context.Context, domain.TenantID, domain.ChargeOccurrenceID, domain.OccurrenceVersion) ([]domain.ParcelReference, error) {
	return nil, nil
}

func (facts *directFacts) ParcelsOfPayable(context.Context, domain.TenantID, domain.PayableID) ([]domain.ParcelReference, error) {
	return nil, nil
}

type directDispositions struct {
	saved     string
	cleared   int
	forgotten int
}

func (book *directDispositions) SaveDirectOperatingDisposition(context.Context, domain.TenantID, string, domain.OperatingBasis, string, time.Time) error {
	book.saved = book.saved + " " // keep last below
	return nil
}

func (book *directDispositions) SaveDirectOperatingDispositionLast(disposition string) {
	book.saved = disposition
}

func (book *directDispositions) ClearDirectOperatingDisposition(context.Context, domain.TenantID, string, domain.OperatingBasis) error {
	book.cleared++
	return nil
}

func (book *directDispositions) ForgetDirectOperatingAnswer(context.Context, domain.TenantID, string, domain.OperatingBasis) error {
	book.forgotten++
	return nil
}

type rememberingDispositions struct {
	directDispositions
}

func (book *rememberingDispositions) SaveDirectOperatingDisposition(_ context.Context, _ domain.TenantID, _ string, _ domain.OperatingBasis, disposition string, _ time.Time) error {
	book.saved = disposition
	return nil
}

func directHandler(t *testing.T, facts *directFacts, estimated, confirmed bool) (*application.DeriveDirectOperatingResultsHandler, *resultStoreDouble, *rememberingDispositions) {
	t.Helper()
	adoption := directAdoption{}
	if estimated || confirmed {
		adopted, err := domain.NewOperatingBasisAdoption(estimated, confirmed)
		if err != nil {
			t.Fatal(err)
		}
		adoption = directAdoption{adoption: adopted, found: true}
	}
	results := newResultStore()
	dispositions := &rememberingDispositions{}
	handler, err := application.NewDeriveDirectOperatingResultsHandler(application.DeriveDirectOperatingResultsDeps{
		Adoptions:    adoption,
		Facts:        facts,
		Dispositions: dispositions,
		Results:      results,
		Allocate: application.NewAllocateCostsHandler(application.AllocateCostsDeps{
			Results: results, Downstream: &operatingHandoffDouble{}, Clock: directClock{at: operatingAsOf},
		}),
		Clock: directClock{at: operatingAsOf},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler, results, dispositions
}

func directTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-direct")
	if err != nil {
		t.Fatal(err)
	}
	return tenant
}

func directParcel(t *testing.T, value string) domain.ParcelReference {
	t.Helper()
	parcel, err := domain.NewParcelReference(value)
	if err != nil {
		t.Fatal(err)
	}
	return parcel
}

func directStored(t *testing.T, results *resultStoreDouble, tenant domain.TenantID, scope string, basis domain.OperatingBasis) ports.OperatingResultRecord {
	t.Helper()
	scopeRef, err := domain.NewOperatingScopeReference(scope)
	if err != nil {
		t.Fatal(err)
	}
	period, err := domain.NewBillingPeriodReference("2026-11-20")
	if err != nil {
		t.Fatal(err)
	}
	record, found := results.records[resultStoreKey(ports.OperatingResultKey{TenantID: tenant, Scope: scopeRef, Period: period, Basis: basis})]
	if !found {
		t.Fatalf("没有 %s 的快照", basis)
	}
	return record
}

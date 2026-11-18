package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestAMeasurementChangeOpensOneSellGenerationAndARepeatDoesNot(t *testing.T) {
	chain := &sellChain{byID: map[string]ports.SellEvaluationRequestRecord{}, successorOf: map[string]string{}}
	first := submittedSellRequest(t, "SELLREQ-SYN-1")
	chain.current = first
	chain.byID[first.Key.Request.String()] = first
	handler, err := application.NewReevaluateSellOnMeasurementChangeHandler(
		chain, chain, chain, chain, chain, sellClock(time.Date(2026, 11, 17, 4, 0, 0, 0, time.UTC)),
	)
	if err != nil {
		t.Fatal(err)
	}
	command := application.ReevaluateSellOnMeasurementChangeCommand{
		Tenant: first.Key.TenantID, Parcel: first.Request.Parcel(), SourceID: "measure-1",
	}
	opened, err := handler.Handle(context.Background(), command)
	if err != nil || opened != application.SellReevaluationOpened || chain.handed != 1 {
		t.Fatalf("打开 %s handed=%d err=%v", opened, chain.handed, err)
	}
	again, err := handler.Handle(context.Background(), command)
	if err != nil || again != application.SellReevaluationAlreadyOpened || chain.handed != 1 {
		t.Fatalf("重投 %s handed=%d err=%v", again, chain.handed, err)
	}
}

func TestAParcelWithoutASellRequestIsLeftAlone(t *testing.T) {
	chain := &sellChain{byID: map[string]ports.SellEvaluationRequestRecord{}, successorOf: map[string]string{}}
	handler, err := application.NewReevaluateSellOnMeasurementChangeHandler(
		chain, chain, chain, chain, chain, sellClock(time.Now()),
	)
	if err != nil {
		t.Fatal(err)
	}
	tenant := mustSell(t, domain.NewTenantID, "SYN-TENANT-01")
	parcel := mustSell(t, domain.NewParcelReference, "SYN-PARCEL-1")
	outcome, err := handler.Handle(context.Background(), application.ReevaluateSellOnMeasurementChangeCommand{
		Tenant: tenant, Parcel: parcel, SourceID: "measure-1",
	})
	if err != nil || outcome != application.SellReevaluationNoRequest || chain.handed != 0 {
		t.Fatalf("没有请求 %s handed=%d err=%v", outcome, chain.handed, err)
	}
}

type sellChain struct {
	byID        map[string]ports.SellEvaluationRequestRecord
	successorOf map[string]string
	current     ports.SellEvaluationRequestRecord
	handed      int
	minted      int
}

func (chain *sellChain) FindByNaturalKey(context.Context, domain.TenantID, domain.SellEvaluationRequestNaturalKey) (ports.SellEvaluationRequestRecord, bool, error) {
	return ports.SellEvaluationRequestRecord{}, false, nil
}

func (chain *sellChain) FindByID(_ context.Context, key ports.SellEvaluationRequestKey) (ports.SellEvaluationRequestRecord, bool, error) {
	record, found := chain.byID[key.Request.String()]
	return record, found, nil
}

func (chain *sellChain) FindCurrentByParcel(context.Context, domain.TenantID, domain.ParcelReference) (ports.SellEvaluationRequestRecord, bool, error) {
	if chain.current.Key.Request.String() == "" {
		return ports.SellEvaluationRequestRecord{}, false, nil
	}
	return chain.current, true, nil
}

func (chain *sellChain) FindSuccessor(_ context.Context, key ports.SellEvaluationRequestKey) (ports.SellEvaluationRequestRecord, bool, error) {
	next, found := chain.successorOf[key.Request.String()]
	if !found {
		return ports.SellEvaluationRequestRecord{}, false, nil
	}
	return chain.byID[next], true, nil
}

func (chain *sellChain) Save(_ context.Context, record ports.SellEvaluationRequestRecord) (ports.EvaluationRequestSaveOutcome, error) {
	previous, has := record.Request.Previous()
	if has {
		if _, taken := chain.successorOf[previous.String()]; taken {
			return ports.EvaluationRequestAlreadyRequested, nil
		}
		chain.successorOf[previous.String()] = record.Key.Request.String()
	}
	chain.byID[record.Key.Request.String()] = record
	return ports.EvaluationRequestSaved, nil
}

func (chain *sellChain) MintSellEvaluationRequestID(context.Context) (domain.SellEvaluationRequestID, error) {
	chain.minted++
	return domain.NewSellEvaluationRequestID("SELLREQ-SYN-NEXT")
}

func (chain *sellChain) HandOffSellEvaluationRequest(context.Context, ports.SellEvaluationRequestIntent) error {
	chain.handed++
	return nil
}

type sellClock time.Time

func (clock sellClock) Now() time.Time { return time.Time(clock) }

func submittedSellRequest(t *testing.T, id string) ports.SellEvaluationRequestRecord {
	t.Helper()
	request, err := domain.SubmitSellEvaluationRequest(domain.SellEvaluationRequestSpec{
		ID:                 mustSell(t, domain.NewSellEvaluationRequestID, id),
		Parcel:             mustSell(t, domain.NewParcelReference, "SYN-PARCEL-1"),
		Account:            mustSell(t, domain.NewSettlementAccountID, "SYN-ACCT-1"),
		SettlementCurrency: mustSell(t, domain.NewCurrencyCode, "XTS"),
		PricingScope:       mustSell(t, domain.NewEstimatePricingScope, "SYN-SCOPE-SELL-1"),
		Moment:             domain.BillingMomentNetworkIntakeAdopted,
		Source:             mustSell(t, domain.NewSourceFactReference, "SYN-INTAKE-1"),
		PricingBasisAt:     time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return ports.SellEvaluationRequestRecord{
		Key: ports.SellEvaluationRequestKey{
			TenantID: mustSell(t, domain.NewTenantID, "SYN-TENANT-01"),
			Request:  request.ID(),
		},
		Request:    request,
		RecordedAt: time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC),
		State:      domain.SellRequestSubmitted,
	}
}

func mustSell[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

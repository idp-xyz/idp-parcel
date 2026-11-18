package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestAMeasurementChangeOpensOneBuyGenerationAndARepeatDoesNot(t *testing.T) {
	chain := &buyChain{byID: map[string]ports.EvaluationRequestRecord{}, successorOf: map[string]string{}}
	first := submittedBuyRequest(t, "EVREQ-SYN-1", "fee-linehaul")
	chain.current = []ports.EvaluationRequestRecord{first}
	chain.byID[first.Key.Request.String()] = first
	handler, err := application.NewReevaluateBuyOnMeasurementChangeHandler(
		chain, chain, chain, chain, chain, chain, sellClock(time.Date(2026, 11, 17, 4, 0, 0, 0, time.UTC)),
	)
	if err != nil {
		t.Fatal(err)
	}
	command := application.ReevaluateBuyOnMeasurementChangeCommand{
		Tenant: first.Key.TenantID, OccurrenceID: "syn-occ-1", OccurrenceVersion: "v1", SourceID: "measure-1",
	}
	opened, err := handler.Handle(context.Background(), command)
	if err != nil || opened != application.BuyReevaluationOpened || chain.handed != 1 {
		t.Fatalf("打开 %s handed=%d err=%v", opened, chain.handed, err)
	}
	again, err := handler.Handle(context.Background(), command)
	if err != nil || again != application.BuyReevaluationAlreadyOpened || chain.handed != 1 {
		t.Fatalf("重投 %s handed=%d err=%v", again, chain.handed, err)
	}
}

func TestAnOccurrenceWithoutABuyRequestIsLeftAlone(t *testing.T) {
	chain := &buyChain{byID: map[string]ports.EvaluationRequestRecord{}}
	handler, err := application.NewReevaluateBuyOnMeasurementChangeHandler(
		chain, chain, chain, chain, chain, chain, sellClock(time.Now()),
	)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := handler.Handle(context.Background(), application.ReevaluateBuyOnMeasurementChangeCommand{
		Tenant: mustSell(t, domain.NewTenantID, "SYN-TENANT-01"), OccurrenceID: "syn-occ-1",
		OccurrenceVersion: "v1", SourceID: "measure-1",
	})
	if err != nil || outcome != application.BuyReevaluationNoRequest || chain.handed != 0 {
		t.Fatalf("没有请求 %s handed=%d err=%v", outcome, chain.handed, err)
	}
}

type buyChain struct {
	byID        map[string]ports.EvaluationRequestRecord
	successorOf map[string]string
	current     []ports.EvaluationRequestRecord
	handed      int
	minted      int
}

func (chain *buyChain) ListCurrentByOccurrence(
	context.Context, domain.TenantID, domain.ChargeOccurrenceID, domain.OccurrenceVersion,
) ([]ports.EvaluationRequestRecord, error) {
	return chain.current, nil
}

func (chain *buyChain) FindByID(_ context.Context, key ports.EvaluationRequestKey) (ports.EvaluationRequestRecord, bool, error) {
	record, found := chain.byID[key.Request.String()]
	return record, found, nil
}

func (chain *buyChain) FindByNaturalKey(context.Context, domain.TenantID, domain.EvaluationRequestNaturalKey) (ports.EvaluationRequestRecord, bool, error) {
	return ports.EvaluationRequestRecord{}, false, nil
}

func (chain *buyChain) FindSuccessor(_ context.Context, key ports.EvaluationRequestKey) (ports.EvaluationRequestRecord, bool, error) {
	next, found := chain.successorOf[key.Request.String()]
	if !found {
		return ports.EvaluationRequestRecord{}, false, nil
	}
	record, ok := chain.byID[next]
	return record, ok, nil
}

func (chain *buyChain) Save(_ context.Context, record ports.EvaluationRequestRecord) (ports.EvaluationRequestSaveOutcome, error) {
	if _, found := chain.byID[record.Key.Request.String()]; found {
		return ports.EvaluationRequestAlreadyRequested, nil
	}
	previous, has := record.Request.Previous()
	if has {
		if _, taken := chain.successorOf[previous.String()]; taken {
			return ports.EvaluationRequestAlreadyRequested, nil
		}
		chain.successorOf[previous.String()] = record.Key.Request.String()
	}
	chain.byID[record.Key.Request.String()] = record
	chain.current = []ports.EvaluationRequestRecord{record}
	return ports.EvaluationRequestSaved, nil
}

func (chain *buyChain) MintEvaluationRequestID(context.Context) (domain.EvaluationRequestID, error) {
	chain.minted++
	return domain.NewEvaluationRequestID("EVREQ-SYN-NEXT")
}

func (chain *buyChain) HandOffEvaluationRequest(context.Context, ports.EvaluationRequestIntent) error {
	chain.handed++
	return nil
}

func submittedBuyRequest(t *testing.T, id, feeItem string) ports.EvaluationRequestRecord {
	t.Helper()
	occurrence, err := domain.NewTransportChargeOccurrence(
		mustSell(t, domain.NewChargeOccurrenceID, "syn-occ-1"),
		mustSell(t, domain.NewOccurrenceReasonReference, "BOOKING"),
		mustSell(t, domain.NewOccurrenceVersion, "v1"),
		time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	request, err := domain.SubmitEvaluationRequest(domain.EvaluationRequestSpec{
		ID:      mustSell(t, domain.NewEvaluationRequestID, id),
		Scope:   mustSell(t, domain.NewPrimaryScopeReference, "syn-scope-1"),
		Purpose: domain.BuySupplierCost,
		Sources: domain.EligibleSourceReferences{
			Occurrence: occurrence,
			FeeItem:    mustSell(t, domain.NewFeeItemReference, feeItem),
			Agreement:  mustSell(t, domain.NewSupplierAgreementReference, "syn-agreement-1"),
		},
		RequestedAt: time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC),
		RequestedBy: mustSell(t, domain.NewRequesterReference, "syn-settlement-job"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return ports.EvaluationRequestRecord{
		Key: ports.EvaluationRequestKey{
			TenantID: mustSell(t, domain.NewTenantID, "SYN-TENANT-01"),
			Request:  request.ID(),
		},
		Request:    request,
		RecordedAt: time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC),
	}
}

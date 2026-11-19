package application_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 代次链上的走法（找链头、沿前一代找这次变化是否已起过一代）销售与采购两路共用一份，这里两路各验一遍同样的格。

type countingSellChain struct {
	*sellChain
	nextID int
}

func (chain *countingSellChain) MintSellEvaluationRequestID(context.Context) (domain.SellEvaluationRequestID, error) {
	chain.nextID++
	chain.minted++
	return domain.NewSellEvaluationRequestID(fmt.Sprintf("SELLREQ-SYN-GEN-%d", chain.nextID))
}

func newCountingSellHandler(t *testing.T, first ports.SellEvaluationRequestRecord) (*application.ReevaluateSellOnMeasurementChangeHandler, *countingSellChain) {
	t.Helper()
	chain := &countingSellChain{sellChain: &sellChain{byID: map[string]ports.SellEvaluationRequestRecord{}, successorOf: map[string]string{}}}
	chain.current = first
	chain.byID[first.Key.Request.String()] = first
	handler, err := application.NewReevaluateSellOnMeasurementChangeHandler(
		chain, chain, chain, chain, chain, sellClock(time.Date(2026, 11, 17, 4, 0, 0, 0, time.UTC)),
	)
	if err != nil {
		t.Fatal(err)
	}
	return handler, chain
}

func sellCommand(first ports.SellEvaluationRequestRecord, source string) application.ReevaluateSellOnMeasurementChangeCommand {
	return application.ReevaluateSellOnMeasurementChangeCommand{
		Tenant: first.Key.TenantID, Parcel: first.Request.Parcel(), SourceID: source,
	}
}

func TestASellGenerationIsOpenedOnTheChainHead(t *testing.T) {
	first := submittedSellRequest(t, "SELLREQ-SYN-1")
	handler, chain := newCountingSellHandler(t, first)
	for _, source := range []string{"measure-1", "measure-2"} {
		outcome, err := handler.Handle(context.Background(), sellCommand(first, source))
		if err != nil || outcome != application.SellReevaluationOpened {
			t.Fatalf("%s 打开 %s err=%v", source, outcome, err)
		}
	}
	// 当前有效的请求一直停在第一代，第二次变化必须挂在刚起的那一代上，而不是又挂回第一代。
	third := chain.byID["SELLREQ-SYN-GEN-2"]
	previous, has := third.Request.Previous()
	if !has || previous.String() != "SELLREQ-SYN-GEN-1" {
		t.Fatalf("第二次变化挂在 %q（有前一代 %t），应挂在 SELLREQ-SYN-GEN-1", previous.String(), has)
	}
}

func TestASellBasisUsedByAnEarlierGenerationIsNotOpenedAgain(t *testing.T) {
	first := submittedSellRequest(t, "SELLREQ-SYN-1")
	handler, chain := newCountingSellHandler(t, first)
	for _, source := range []string{"measure-1", "measure-2"} {
		if _, err := handler.Handle(context.Background(), sellCommand(first, source)); err != nil {
			t.Fatal(err)
		}
	}
	again, err := handler.Handle(context.Background(), sellCommand(first, "measure-1"))
	if err != nil || again != application.SellReevaluationAlreadyOpened || chain.handed != 2 || chain.minted != 2 {
		t.Fatalf("重投较早一代用过的来源 %s handed=%d minted=%d err=%v", again, chain.handed, chain.minted, err)
	}
}

func TestASellChainMissingAPreviousGenerationIsAnError(t *testing.T) {
	first := submittedSellRequest(t, "SELLREQ-SYN-1")
	basis, err := domain.NewMeasurementChangeResumeBasis("measure-1")
	if err != nil {
		t.Fatal(err)
	}
	next, err := first.Request.Resume(mustSell(t, domain.NewSellEvaluationRequestID, "SELLREQ-SYN-2"), basis)
	if err != nil {
		t.Fatal(err)
	}
	second := ports.SellEvaluationRequestRecord{
		Key:     ports.SellEvaluationRequestKey{TenantID: first.Key.TenantID, Request: next.ID()},
		Request: next, RecordedAt: first.RecordedAt, State: domain.SellRequestSubmitted,
	}
	// 册上只有第二代：它的前一代读不到，是链断了，不能当成没有前一代而再起一代。
	handler, chain := newCountingSellHandler(t, second)
	outcome, err := handler.Handle(context.Background(), sellCommand(first, "measure-2"))
	if err == nil || err.Error() != "sell reevaluation: previous request SELLREQ-SYN-1 is missing" || chain.handed != 0 {
		t.Fatalf("链断 %s handed=%d err=%v", outcome, chain.handed, err)
	}
}

type countingBuyChain struct {
	*buyChain
	nextID int
}

func (chain *countingBuyChain) MintEvaluationRequestID(context.Context) (domain.EvaluationRequestID, error) {
	chain.nextID++
	chain.minted++
	return domain.NewEvaluationRequestID(fmt.Sprintf("EVREQ-SYN-GEN-%d", chain.nextID))
}

func newCountingBuyHandler(t *testing.T, first ports.EvaluationRequestRecord) (*application.ReevaluateBuyOnMeasurementChangeHandler, *countingBuyChain) {
	t.Helper()
	chain := &countingBuyChain{buyChain: &buyChain{byID: map[string]ports.EvaluationRequestRecord{}, successorOf: map[string]string{}}}
	chain.current = []ports.EvaluationRequestRecord{first}
	chain.byID[first.Key.Request.String()] = first
	handler, err := application.NewReevaluateBuyOnMeasurementChangeHandler(
		chain, chain, chain, chain, chain, chain, sellClock(time.Date(2026, 11, 17, 4, 0, 0, 0, time.UTC)),
	)
	if err != nil {
		t.Fatal(err)
	}
	return handler, chain
}

func buyCommand(first ports.EvaluationRequestRecord, source string) application.ReevaluateBuyOnMeasurementChangeCommand {
	return application.ReevaluateBuyOnMeasurementChangeCommand{
		Tenant: first.Key.TenantID, OccurrenceID: "syn-occ-1", OccurrenceVersion: "v1", SourceID: source,
	}
}

func TestABuyGenerationIsOpenedOnTheChainHead(t *testing.T) {
	first := submittedBuyRequest(t, "EVREQ-SYN-1", "fee-linehaul")
	handler, chain := newCountingBuyHandler(t, first)
	for _, source := range []string{"measure-1", "measure-2"} {
		outcome, err := handler.Handle(context.Background(), buyCommand(first, source))
		if err != nil || outcome != application.BuyReevaluationOpened {
			t.Fatalf("%s 打开 %s err=%v", source, outcome, err)
		}
		// 册上给出的当前请求一直停在第一代：第二次变化必须沿后继走到链头再起。
		chain.current = []ports.EvaluationRequestRecord{first}
	}
	third := chain.byID["EVREQ-SYN-GEN-2"]
	previous, has := third.Request.Previous()
	if !has || previous.String() != "EVREQ-SYN-GEN-1" {
		t.Fatalf("第二次变化挂在 %q（有前一代 %t），应挂在 EVREQ-SYN-GEN-1", previous.String(), has)
	}
}

func TestABuyBasisUsedByAnEarlierGenerationIsNotOpenedAgain(t *testing.T) {
	first := submittedBuyRequest(t, "EVREQ-SYN-1", "fee-linehaul")
	handler, chain := newCountingBuyHandler(t, first)
	for _, source := range []string{"measure-1", "measure-2"} {
		if _, err := handler.Handle(context.Background(), buyCommand(first, source)); err != nil {
			t.Fatal(err)
		}
		chain.current = []ports.EvaluationRequestRecord{first}
	}
	again, err := handler.Handle(context.Background(), buyCommand(first, "measure-1"))
	if err != nil || again != application.BuyReevaluationAlreadyOpened || chain.handed != 2 || chain.minted != 2 {
		t.Fatalf("重投较早一代用过的来源 %s handed=%d minted=%d err=%v", again, chain.handed, chain.minted, err)
	}
}

func TestABuyChainMissingAPreviousGenerationIsAnError(t *testing.T) {
	first := submittedBuyRequest(t, "EVREQ-SYN-1", "fee-linehaul")
	basis, err := domain.NewMeasurementChangeResumeBasis("measure-1")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC)
	next, err := first.Request.Resume(
		mustSell(t, domain.NewEvaluationRequestID, "EVREQ-SYN-2"), basis, now,
		mustSell(t, domain.NewRequesterReference, "syn-settlement-job"),
	)
	if err != nil {
		t.Fatal(err)
	}
	second := ports.EvaluationRequestRecord{
		Key:     ports.EvaluationRequestKey{TenantID: first.Key.TenantID, Request: next.ID()},
		Request: next, RecordedAt: now,
	}
	// 册上只有第二代：它的前一代读不到，是链断了，不能当成没有前一代而再起一代。
	handler, chain := newCountingBuyHandler(t, second)
	outcome, err := handler.Handle(context.Background(), buyCommand(first, "measure-2"))
	if err == nil || err.Error() != "buy reevaluation: previous request EVREQ-SYN-1 is missing" || chain.handed != 0 {
		t.Fatalf("链断 %s handed=%d err=%v", outcome, chain.handed, err)
	}
}

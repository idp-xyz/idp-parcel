package application_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉 ADR-0229 决定三的计价一段（AT-SA-198）：计价答不出评价时，SELL 请求记`未形成`连同停因，BUY 请求记停因；同一个停因
// 再来是重放；请求不在册上、册上另有结论都是矛盾，上抛；停因不是计价那一段的、方向与请求标识对不上都不受理；册答不出
// 是未决。

func newUnformedRecording(t *testing.T) (*application.RecordEvaluationUnformedHandler, *estimatedChargeFixture, *requestedCostFixture) {
	t.Helper()
	sell := newEstimatedChargeFixture(t, nil)
	buy := newRequestedCostFixture(t)
	handler, err := application.NewRecordEvaluationUnformedHandler(application.RecordEvaluationUnformedDeps{
		SellRequests: sell.requests, SellConclusions: sell.requests,
		BuyRequests: buy.requests, BuyStops: buy.requests,
		Clock: fixedClock{at: costOccurredAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler, sell, buy
}

func TestAnUnformedSellRequestIsConcludedNotFormedWithItsCause(t *testing.T) {
	handler, sell, _ := newUnformedRecording(t)
	command := application.RecordEvaluationUnformedCommand{
		TenantID: sell.tenant, Direction: domain.SellEvaluationDirection,
		SellRequest: mustAccountValue(t, domain.NewSellEvaluationRequestID, sellRequestID), Cause: domain.StopPriceCardNotConfigured,
	}
	for range 2 {
		result, err := handler.Handle(t.Context(), command)
		if err != nil || result.Outcome() != application.EvaluationUnformedRecorded {
			t.Fatalf("记停因 = %s err = %v", result.Outcome(), err)
		}
	}
	if stored := sell.storedState(t); stored.State != domain.SellRequestNotFormed || stored.StopCause != domain.StopPriceCardNotConfigured {
		t.Fatalf("SELL 请求册 = %s / %s，want NOT_FORMED / PRICE_CARD_NOT_CONFIGURED", stored.State, stored.StopCause)
	}

	sell.requests.forceEffect = ports.SellEvaluationRequestConcludedOtherwise
	if _, err := handler.Handle(t.Context(), command); !errors.Is(err, application.ErrUnformedEvaluationRequestConcluded) {
		t.Fatalf("另有结论 err = %v", err)
	}
	sell.requests.forceEffect = ports.SellEvaluationRequestConclusionEffectInvalid
	missing := command
	missing.SellRequest = mustAccountValue(t, domain.NewSellEvaluationRequestID, "SELLREQ-SYN-NONE")
	if _, err := handler.Handle(t.Context(), missing); !errors.Is(err, application.ErrUnformedEvaluationRequestMissing) {
		t.Fatalf("请求不在册上 err = %v", err)
	}
	sell.requests.viewErr = errors.New("register down")
	if result, err := handler.Handle(t.Context(), command); err != nil || result.Outcome() != application.EvaluationUnformedUndecided {
		t.Fatalf("册答不出 = %s err = %v，want 未决", result.Outcome(), err)
	}
}

func TestAnUnformedBuyRequestRecordsItsCauseOnce(t *testing.T) {
	handler, _, buy := newUnformedRecording(t)
	tenant := mustAccountValue(t, domain.NewTenantID, "tenant-1")
	command := application.RecordEvaluationUnformedCommand{
		TenantID: tenant, Direction: domain.BuyEvaluationDirection,
		BuyRequest: mustAccountValue(t, domain.NewEvaluationRequestID, "EVREQ-SYN-1"), Cause: domain.StopPricingInputUnavailable,
	}
	for range 2 {
		if result, err := handler.Handle(t.Context(), command); err != nil || result.Outcome() != application.EvaluationUnformedRecorded {
			t.Fatalf("记停因 = %s err = %v", result.Outcome(), err)
		}
	}
	if stored := buy.requests.records["tenant-1/EVREQ-SYN-1"]; stored.StopCause != domain.StopPricingInputUnavailable {
		t.Fatalf("BUY 请求册停因 = %s", stored.StopCause)
	}
	other := command
	other.Cause = domain.StopPriceCardApplicabilityConflict
	if _, err := handler.Handle(t.Context(), other); !errors.Is(err, application.ErrUnformedEvaluationRequestConcluded) {
		t.Fatalf("另一个停因 err = %v", err)
	}
	for name, broken := range map[string]application.RecordEvaluationUnformedCommand{
		"采用那一段的停因": {TenantID: tenant, Direction: domain.BuyEvaluationDirection, BuyRequest: command.BuyRequest, Cause: domain.StopEvaluationFailed},
		"方向与标识对不上": {TenantID: tenant, Direction: domain.SellEvaluationDirection, BuyRequest: command.BuyRequest, Cause: domain.StopPriceCardNotConfigured},
		"没有方向":     {TenantID: tenant, BuyRequest: command.BuyRequest, Cause: domain.StopPriceCardNotConfigured},
	} {
		if result, err := handler.Handle(t.Context(), broken); err != nil || result.Outcome() != application.EvaluationUnformedNotAccepted {
			t.Fatalf("%s = %s err = %v，want 未受理", name, result.Outcome(), err)
		}
	}
	if _, err := application.NewRecordEvaluationUnformedHandler(application.RecordEvaluationUnformedDeps{}); !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("缺依赖 err = %v", err)
	}
}

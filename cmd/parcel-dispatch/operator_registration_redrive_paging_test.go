package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 证登记续办门的翻页（票 declared-goods-lines/23 评审）：自 ADR-0383 起，一次登记只解得开它管的那几份，
// 被重驱又停回`等待运营登记`是常态。翻页若每页重读队首，比满一页还多的旧委托停着，后面那份这次登记正好解得开的委托就永远
// 轮不到。这里把页大小压到 2，先停四份——前三份这次照旧停、最新的一份解得开——一封 SELL「参数已登记」之后，最新那份必须
// 被重驱并接受，前三份各被问恰一次、照旧等。

const redrivePagingPageSize = 2

// Covers: 票 declared-goods-lines/23 评审——登记续办门按（提交时刻、委托标识）往后翻页，满页之后的委托照样被重驱。
func TestARedriveReachesTheRequestBehindAFullPageOfStillWaitingOnes(t *testing.T) {
	control := &synRPerRequestControl{t: t, resolvable: map[string]bool{}}
	fixture := newOperatorRegistrationLoopFixtureWith(t, control)
	fixture.commercial.configured = true
	ctx := t.Context()

	requests := []string{"SYN-REQ-PAGE-1", "SYN-REQ-PAGE-2", "SYN-REQ-PAGE-3", "SYN-REQ-PAGE-4"}
	for index, requestID := range requests {
		fixture.parkOnPriceCard(t, ctx, requestID, fmt.Sprintf("SYN-KEY-PAGE-%d", index+1),
			fmt.Sprintf("SYN-PARCEL-PAGE-%d", index+1))
	}
	if queue := fixture.listRegistrationQueue(t, ctx); len(queue) != len(requests) {
		t.Fatalf("等待运营登记队列 = %d 份, want %d", len(queue), len(requests))
	}

	// 这次登记只解得开最新的那一份。
	newest := requests[len(requests)-1]
	control.resolvable[newest] = true
	control.asked = map[string]int{}
	gate, err := psinbox.NewPricingParameterRegisteredConsumer(
		fixture.transactor, fixture.inboxStore, fixture.requests, fixture.chain, redrivePagingPageSize)
	if err != nil {
		t.Fatalf("构造小页的计价参数续办门：%v", err)
	}
	// 信封由计价真交接口随登卡铸出。
	registerPriceCardForTest(t, fixture.db,
		pricingResumeSellCard(t, "SYN-PLAN-SELL-PAGE", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	delivery := fixture.claimDeliveryOfType(t, string(psinbox.PricingParameterRegisteredEventType))
	if err := gate.Consume(ctx, delivery.Envelope); err != nil {
		t.Fatalf("一份接受、其余照旧停，本封应入账：%v", err)
	}

	for _, requestID := range requests {
		if control.asked[requestID] != 1 {
			t.Fatalf("%s 被问了 %d 次, want 恰 1 次——翻页漏了或重复了它", requestID, control.asked[requestID])
		}
	}
	queue := fixture.listRegistrationQueue(t, ctx)
	if len(queue) != len(requests)-1 {
		t.Fatalf("重驱之后队列 = %d 份, want %d", len(queue), len(requests)-1)
	}
	for _, record := range queue {
		if record.ShipmentRequestID.String() == newest {
			t.Fatalf("%s 仍在等——满页之后的那一份没轮到", newest)
		}
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.shipment_request
		WHERE shipment_request_id = '`+newest+`' AND state = `+fmt.Sprint(uint8(psdomain.ShipmentRequestAccepted))); n != 1 {
		t.Fatalf("%s 没有被接受", newest)
	}
}

// parkOnPriceCard 照 SYN 纵切的配方另建一份委托（来源键、委托号与成员各自独立），交「委托已提交」并让提交门推进一拍：
// 控制那一口答价卡没配，委托停进`等待运营登记`。
func (fixture *operatorRegistrationLoopFixture) parkOnPriceCard(
	t *testing.T,
	ctx context.Context,
	requestID, key, parcel string,
) {
	t.Helper()
	identity, err := psdomain.NewSourceIdentity(fixture.identity.TenantID(), fixture.identity.CustomerAccountID(),
		fixture.identity.Source(), mustPS(t, psdomain.NewSourceRequestKey, key))
	if err != nil {
		t.Fatalf("来源身份：%v", err)
	}
	command := fixture.submitCommand(t)
	command.Identity = identity
	command.ShipmentRequestID = mustPS(t, psdomain.NewShipmentRequestID, requestID)
	command.PayloadDigest = mustPS(t, psdomain.NewPayloadDigest, "SYN-DIGEST-"+requestID)
	command.DeclaredParcelIDs = []psdomain.DeclaredParcelID{mustPS(t, psdomain.NewDeclaredParcelID, parcel)}
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		result, err := fixture.submitHandler.Handle(txCtx, command)
		if err != nil {
			return err
		}
		if result.Outcome() != psapplication.OutcomeSubmitted {
			return fmt.Errorf("submit %s: %s", requestID, result.Outcome())
		}
		return nil
	})
	request, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回 %s：found=%v err=%v", requestID, found, err)
	}
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: identity, Request: request})
	})
	delivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	if err := fixture.submittedGate.Consume(ctx, delivery.Envelope); err != nil {
		t.Fatalf("%s 停在价卡那一格应入账：%v", requestID, err)
	}
	fixture.markPublished(t, delivery)
}

// synRPerRequestControl 是 settlement-accounting 权威替身（隔离合成 `S`）：按委托答——resolvable 里的那几份照
// synRFinancialAuthority 答`已冻结`，其余答端口的`未配置`、带 SA-ESTIMATE_PRICE_CARD_NOT_CONFIGURED（生产里 PS 适配器把
// 同名的结算原因译到这一格）。asked 按委托计数。
type synRPerRequestControl struct {
	t          *testing.T
	resolvable map[string]bool
	asked      map[string]int
}

func (double *synRPerRequestControl) ApplyPreAcceptanceFinancialControl(
	ctx context.Context,
	request psports.FinancialControlRequest,
) (psports.PreAcceptanceControlAssessment, error) {
	double.t.Helper()
	if double.asked == nil {
		double.asked = map[string]int{}
	}
	double.asked[request.ShipmentRequestID.String()]++
	if double.resolvable[request.ShipmentRequestID.String()] {
		return synRFinancialAuthority{t: double.t}.ApplyPreAcceptanceFinancialControl(ctx, request)
	}
	return psports.PreAcceptanceControlAssessment{
		Outcome:          psports.PreAcceptanceControlNotConfigured,
		Reason:           mustPS(double.t, psdomain.NewCheckReason, "SA-ESTIMATE_PRICE_CARD_NOT_CONFIGURED"),
		SettlementReason: mustPS(double.t, psdomain.NewSettlementNotFormedReason, "ESTIMATE_PRICE_CARD_NOT_CONFIGURED"),
	}, nil
}

var _ psports.PreAcceptanceFinancialController = (*synRPerRequestControl)(nil)

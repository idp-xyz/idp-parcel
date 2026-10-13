package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 16 证票 product-strategy-boundary/25 金额那一格的闭环：受理前估价采用册上没有这一户时，接受前财务控制
// 在金额那一步答端口的`未配置`（SA-CONTROL_AMOUNT_NOT_CONFIGURED），链落`等待运营登记`，提交门入账而不回滚重投，处理尝试带原因与
// 续办引用留在库里，两本账与留痕册一行不落；settlement-accounting 侧登记采用行时经真交接适配器铸出「受理前估价采用已登记」信封，
// 那一份登记续办门按租户重驱，同一条链越过金额那一格、拿登记的计价范围去问试算。
//
// 装配与作用域那条闭环（TestAScopeWaitIsResumedByTheSettlementAccountRegistrationEnvelope）同一份：生产的 PS 适配器、作用域源与
// 账户目录，生产的 SA 施加编排（两本账、采用册、留痕册真库），控制策略与试算两口替身。结算账户先登好，作用域一开始就形成。试算
// 替身答「没有在用的售价卡」：那一格是`未形成`、回滚重投，所以重驱那一轮整封交回未决哨兵，账本无痕等重投——选卡那一格的登记
// 不在本票。

const estimateAdoptionResumeConsumer = "parcel-shipment/advance-acceptance-chain-on-pre-acceptance-estimate-adoption"

// Covers: 票 product-strategy-boundary/25 完成判据一（空册：控制停在金额未配置；委托进`等待运营登记`，处理尝试落库，信封入账而不是
// ABANDONED。登记采用行之后，信封重驱同一委托，越过金额格）。
func TestAnAmountWaitIsResumedByTheEstimateAdoptionEnvelope(t *testing.T) {
	control := &synRLateBoundControl{}
	fixture := newOperatorRegistrationLoopFixtureWith(t, control)
	fixture.commercial.configured = true
	ctx := t.Context()
	tenant := mustPS(t, sadomain.NewTenantID, fixture.identity.TenantID().String())

	accounts, err := sapostgres.NewSettlementAccounts(fixture.db)
	if err != nil {
		t.Fatalf("构造结算账户登记册：%v", err)
	}
	account := synRReceivableAccount(t)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		_, err := accounts.Save(txCtx, tenant, account, time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC))
		return err
	})
	directory, err := pssettlement.NewRegisteredAccountDirectory(accounts)
	if err != nil {
		t.Fatalf("构造账户目录：%v", err)
	}
	settlement := newSynRSettlementControl(t, fixture.db)
	control.inner = pssettlement.NewPreAcceptanceControlAdapter(pssettlement.PreAcceptanceControlAdapterDeps{
		Apply: settlement.apply,
		Scopes: pssettlement.NewPolicyBackedControlScopeSource(
			&synRSettledCommercialBasis{synRSwitchableAsOfCommercialBasis: fixture.commercial}, directory),
	})

	fixture.submit(t, ctx)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})

	// 第一扇门：作用域形成、控制策略要求冻结，采用册上没有这一户——金额那一格`未配置`，等待态先落库，消费门入账。
	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	if err := fixture.submittedGate.Consume(ctx, submittedDelivery.Envelope); err != nil {
		t.Fatalf("金额未配置必须按处理完毕入账：%v", err)
	}
	if n := fixture.countInbox(t, "parcel-shipment/advance-acceptance-chain", string(submittedDelivery.Envelope.ID)); n != 1 {
		t.Fatalf("提交门 inbox 行数 = %d, want 1——等待态没有随本份投递入账", n)
	}
	if settlement.adoptions.asked == 0 || settlement.trials.asked != 0 {
		t.Fatalf("采用册被问 %d 次、试算被问 %d 次, want 问到采用册、没问试算——停顿不是本用例要的那一格",
			settlement.adoptions.asked, settlement.trials.asked)
	}
	waiting := fixture.mustLoadRequest(t, ctx)
	if waitingOn, present := waiting.AcceptanceDecisionTask().WaitingOn(); !present || waitingOn != psdomain.ResumeByOperatorRegistration {
		t.Fatalf("等待态 = %v（present=%v）, want OPERATOR_REGISTRATION", waitingOn, present)
	}
	if queue := fixture.listRegistrationQueue(t, ctx); len(queue) != 1 || queue[0].ShipmentRequestID != fixture.requestID {
		t.Fatalf("等待运营登记队列 = %+v, want 恰好这一份委托", queue)
	}
	const attemptsOnTheAmountWait = `SELECT count(*) FROM parcel_shipment.acceptance_processing_attempt
		WHERE reason_ref = 'FINANCIAL_CONTROL_NOT_CONFIGURED' AND resume_path = 'OPERATOR_REGISTRATION'
		  AND continuation_ref LIKE 'CONT-%'`
	if n := fixture.countSQL(t, attemptsOnTheAmountWait); n != 1 {
		t.Fatalf("带原因与续办引用的处理尝试 = %d 行, want 1——停在哪一格没有留在任务上", n)
	}
	for _, table := range []string{"funds_freeze", "credit_exposure", "pre_acceptance_estimate_trace"} {
		if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.`+table); n != 0 {
			t.Fatalf("金额没形成却在 %s 落了 %d 行", table, n)
		}
	}
	fixture.markPublished(t, submittedDelivery)

	// 登记：settlement-accounting 的登记编排落采用行，同事务经真交接适配器铸「受理前估价采用已登记」信封。
	adoptions, err := sapostgres.NewPreAcceptanceEstimateAdoptions(fixture.db)
	if err != nil {
		t.Fatalf("构造采用册：%v", err)
	}
	handoff, err := sapostgres.NewOutboxPreAcceptanceEstimateAdoptionRegisteredHandoff(fixture.db, fixture.store, systemClock{})
	if err != nil {
		t.Fatalf("构造采用已登记交接：%v", err)
	}
	register, err := saapplication.NewRegisterPreAcceptanceEstimateAdoptionHandler(adoptions, handoff, systemClock{})
	if err != nil {
		t.Fatalf("构造采用登记：%v", err)
	}
	adoption, err := sadomain.NewPreAcceptanceEstimateAdoption(account.ID(), sadomain.PerParcelSellTrialSum,
		mustPS(t, sadomain.NewEstimatePricingScope, "SYN-PRICING-SCOPE-R1"))
	if err != nil {
		t.Fatalf("采用行：%v", err)
	}
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		_, err := register.Register(txCtx, saapplication.RegisterPreAcceptanceEstimateAdoptionCommand{Tenant: tenant, Adoption: adoption})
		return err
	})
	if n := fixture.countOutboxOfType(t, string(psinbox.PreAcceptanceEstimateAdoptionRegisteredEventType)); n != 1 {
		t.Fatalf("「受理前估价采用已登记」信封 = %d 封, want 恰好 1——消费侧认的类型串与发布侧写的不是同一个", n)
	}

	// 估价采用那一份续办门：按租户取队列再驱一拍——越过金额那一格、按登记的计价范围问到试算；试算答没有在用的售价卡，那一格
	// `未形成`、回滚重投。
	adoptionDelivery := fixture.claimDeliveryOfType(t, string(psinbox.PreAcceptanceEstimateAdoptionRegisteredEventType))
	err = fixture.estimateAdoptionGate.Consume(ctx, adoptionDelivery.Envelope)
	if !errors.Is(err, psinbox.ErrAcceptanceChainUndecided) {
		t.Fatalf("越过金额之后停在选卡那一格，整封应交回未决哨兵等重投，实际 err = %v", err)
	}
	if !strings.Contains(err.Error(), "FINANCIAL_CONTROL_NOT_FORMED") {
		t.Fatalf("重驱停在的不是`未形成`那一格：%v", err)
	}
	if settlement.trials.asked != 1 || settlement.trials.query.PricingScope.String() != "SYN-PRICING-SCOPE-R1" ||
		settlement.trials.query.ShipmentRequest != fixture.requestID.String() {
		t.Fatalf("试算被问 %d 次、问法 %+v, want 问一次、计价范围取登记的那一行、委托就是这一份——金额那一格没有越过",
			settlement.trials.asked, settlement.trials.query)
	}
	if n := fixture.countInbox(t, estimateAdoptionResumeConsumer, string(adoptionDelivery.Envelope.ID)); n != 0 {
		t.Fatalf("未决必须让账本无痕等重投，实际 inbox 行数 = %d", n)
	}
	still := fixture.mustLoadRequest(t, ctx)
	if still.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("重驱后状态 = %q, want SUBMITTED", still.State())
	}
	if n := fixture.countSQL(t, attemptsOnTheAmountWait); n != 1 {
		t.Fatalf("重驱那一轮回滚之后处理尝试 = %d 行, want 仍为 1", n)
	}
}

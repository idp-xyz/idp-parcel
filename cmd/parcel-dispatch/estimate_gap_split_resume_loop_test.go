package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 16 证票 declared-goods-lines/26 的闭环（ADR-0381 决定一至四）：受理前估价答`申报不全`时，
// 接受链按缺项该谁补分流——客户补得上的（测量、尺寸、收寄邮编）停`等待受控补充`，缺口按本上下文封闭码落在任务上、
// 计价缺项码原样记上处理尝试，补件队列列得出，受控补充重报整份内容后新版本信封把同一条链再驱一拍直到接受；只有
// 登记才解得开的（分区、地址性质）照票 23 停`等待运营登记`、两类同场登记优先；缺项缺席照旧内部重试、不留一行痕。
//
// 装配照计价参数那条闭环（newOperatorRegistrationLoopFixtureWith / newCustomerSupplementLoopFixtureWith + 生产的
// PS→SA 适配器、生产的 SA 施加编排与两本账），试算那一口装的是**生产的受理前试算链**（preAcceptanceTrials：真价卡册
// 选卡 → 按提交版本读申报 → 对给定价卡形成试算），缺项真的由计价的输入完备性门点名，不由替身编。分区目录用演示种子
// SYN-CAT-ZONE-CN-SG：它的始发维度声明就是按邮编前缀分表（200/510），正好同时供`始发邮编没报`那一格取证。
// 时点照「提交接收」折（生产的 SubmissionReceiptAsOf）。证据只记 S。

const (
	gapSplitPlanID = "SYN-PLAN-T26-SELL"
	// gapSplitTrialMinor 是 Z1 一行 73 XTS 按最小币单位两位换写的单件合计。
	gapSplitTrialMinor = int64(7300)
)

// gapSplitCardStart 让卡与演示分区目录同起点：判断时点（提交接收，测试运行当下）落在两者的生效期里。
var gapSplitCardStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Covers: 票 declared-goods-lines/26 完成判据一——真库：一件缺重量、委托缺收件邮编（卡绑分区目录，缺项全是客户补得上
// 的）→ 停`等待受控补充`，任务缺口两条（ESTIMATE_RECIPIENT_POSTAL_MISSING 委托级、ESTIMATE_MEASUREMENT_MISSING 指名
// 那一件），处理尝试一行带 ESTIMATE_DECLARATION_INCOMPLETE、申报产品与计价缺项码原样，补件队列列出它，委托详情的
// 「上次处理」带同几格；受控补充重报整份内容（两件测量 + 收寄邮编）→ 新版本信封再驱一拍，估价形成、冻结占下、接受。
func TestDeclarationGapsStallForCustomerSupplementAndResumeOnTheSupplementedVersion(t *testing.T) {
	control := &synRLateBoundControl{}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: control})
	fixture.commercial.asOfAt = submissionReceiptAsOf(t, fixture.requests)
	ctx := t.Context()

	settlement := wireGapSplitSettlement(t, fixture.synVerticalFixture, fixture.commercial, control, 2*gapSplitTrialMinor)
	registerDemoZoneCatalogue(t, fixture.db)
	registerPriceCardForTest(t, fixture.db, gapSplitSellCard(t, "v1", true))
	drainGapSplitPricingEnvelopes(t, fixture.store)

	// 提交：P-01 报了重量、P-02 没报；寄件邮编在、收件邮编缺——路线拼不出来，卡要的整条路线缺的是收件那半。
	gapSplitSubmit(t, ctx, fixture.synVerticalFixture, []gapSplitParcel{
		{id: "SYN-PARCEL-01", kilograms: "2"},
		{id: "SYN-PARCEL-02"},
	}, "200000", "")
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})

	// 第一扇门：生产试算链点名两件缺项，分流落`等待受控补充`——等待态先落库，消费门入账（ADR-0094 决定五、三）。
	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	if err := fixture.submittedGate.Consume(ctx, submittedDelivery.Envelope); err != nil {
		t.Fatalf("等待受控补充必须按处理完毕入账：%v", err)
	}
	if n := fixture.countInbox(t, "parcel-shipment/advance-acceptance-chain", string(submittedDelivery.Envelope.ID)); n != 1 {
		t.Fatalf("提交门 inbox 行数 = %d, want 1——等待态没有随本份投递入账", n)
	}
	if settlement.trials.asked != 1 {
		t.Fatalf("试算被问 %d 次, want 1——停顿不是生产试算链点的名", settlement.trials.asked)
	}
	paused := fixture.mustLoadRequest(t, ctx)
	if paused.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("等待期状态 = %q, want SUBMITTED", paused.State())
	}
	if waitingOn, present := paused.AcceptanceDecisionTask().WaitingOn(); !present || waitingOn != psdomain.ResumeByCustomerSupplement {
		t.Fatalf("等待态 = %v（present=%v）, want CUSTOMER_SUPPLEMENT", waitingOn, present)
	}
	// 任务缺口：邮编那一格委托级不指件（计价逐件点名只是逐件读到同一格申报），测量指名缺的那一件；全在
	// 「接受前财务控制」判组上（ADR-0381 决定四）。
	gaps := paused.AcceptanceDecisionTask().SupplementGaps()
	wantGaps := []gapSplitGapShape{
		{parcel: "", reason: "ESTIMATE_RECIPIENT_POSTAL_MISSING"},
		{parcel: "SYN-PARCEL-02", reason: "ESTIMATE_MEASUREMENT_MISSING"},
	}
	if got := gapSplitGapShapes(gaps); !slices.Equal(got, wantGaps) {
		t.Fatalf("任务缺口 = %+v, want %+v", got, wantGaps)
	}
	for _, gap := range gaps {
		if gap.Group() != psdomain.PreAcceptanceFinancialControlCheck {
			t.Fatalf("缺口判组 = %v, want 接受前财务控制", gap.Group())
		}
	}
	if queue := fixture.listSupplementQueue(t, ctx); len(queue) != 1 || queue[0].ShipmentRequestID != fixture.requestID {
		t.Fatalf("补件队列 = %+v, want 恰好这一份委托", queue)
	}
	// 处理尝试：待判原因与续办方用受控补充既有语言，结算原因码、申报产品照票 23 的两格，计价缺项码原样去重
	// （ADR-0383 决定三；ADR-0381 决定四）。
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.acceptance_processing_attempt
		WHERE reason_ref = 'CUSTOMER_SUPPLEMENT_PENDING' AND resume_path = 'CUSTOMER_SUPPLEMENT'
		  AND settlement_reason = 'ESTIMATE_DECLARATION_INCOMPLETE' AND service_product = '`+pricingResumeProduct+`'
		  AND estimate_gap_codes = ARRAY['POSTAL_ROUTE','MEASUREMENT']::text[]
		  AND continuation_ref LIKE 'CONT-%'`); n != 1 {
		t.Fatalf("带结算原因、产品与缺项码的处理尝试 = %d 行, want 1", n)
	}
	// 委托详情的「上次处理」从尝试表读出同几格。
	detailScope, err := psdomain.NewAuthorizedQueryScope(
		mustPS(t, psdomain.NewQueryScopeReference, "SYN-QS-01"),
		fixture.identity.TenantID(),
		[]psdomain.CustomerAccountID{fixture.identity.CustomerAccountID()},
	)
	if err != nil {
		t.Fatalf("查询作用域：%v", err)
	}
	detail, found, err := fixture.views.FindVisibleByID(ctx, detailScope, fixture.requestID)
	if err != nil || !found {
		t.Fatalf("查阅详情：found=%v err=%v", found, err)
	}
	task := detail.Task
	if !task.HasAttempt || task.LastAttemptReason != "CUSTOMER_SUPPLEMENT_PENDING" ||
		task.LastAttemptResumePath != psdomain.ResumeByCustomerSupplement ||
		task.LastAttemptSettlementReason != "ESTIMATE_DECLARATION_INCOMPLETE" ||
		task.LastAttemptServiceProduct != pricingResumeProduct ||
		!slices.Equal(task.LastAttemptEstimateGapCodes, []string{"POSTAL_ROUTE", "MEASUREMENT"}) {
		t.Fatalf("委托详情的上次处理 = %+v, want 等受控补充、带结算原因、产品与缺项码", task)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.funds_freeze`); n != 0 {
		t.Fatalf("申报不全却占了 %d 笔冻结", n)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 0 {
		t.Fatalf("等待期间入队了 %d 封接受信封", n)
	}
	fixture.markPublished(t, submittedDelivery)

	// 受控补充：整份重报——两件都带测量、收寄邮编齐、产品照报（补充不从旧版本继承）。新版本与信封同一事务。
	supplemented := gapSplitSupplementOn(t, ctx, fixture, paused.CurrentSubmissionVersion().VersionID(), []gapSplitParcel{
		{id: "SYN-PARCEL-01", kilograms: "2"},
		{id: "SYN-PARCEL-02", kilograms: "3"},
	}, "200000", "018000")
	if supplemented.Outcome() != psapplication.SupplementRecorded {
		t.Fatalf("受控补充 outcome = %q, want SUPPLEMENT_RECORDED（未决原因 %q）",
			supplemented.Outcome(), supplemented.PendingReason())
	}
	if remaining := fixture.listSupplementQueue(t, ctx); len(remaining) != 0 {
		t.Fatalf("新版本形成后补件队列仍列出 %d 行", len(remaining))
	}

	// 第四扇门：新版本信封把同一条链再驱一拍——生产试算链这回评得出两件合计，冻结占下，决定形成为接受。
	resumeDelivery := fixture.claimDeliveryOfType(t, string(psinbox.SubmissionVersionFormedEventType))
	if err := fixture.supplementGate.Consume(ctx, resumeDelivery.Envelope); err != nil {
		t.Fatalf("受控补充续办一拍：%v", err)
	}
	if settlement.trials.asked != 2 {
		t.Fatalf("续办后试算被问 %d 次, want 2——委托没按新版本重判", settlement.trials.asked)
	}
	accepted := fixture.mustLoadRequest(t, ctx)
	if accepted.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("续办后状态 = %q, want ACCEPTED", accepted.State())
	}
	if waitingOn, present := accepted.AcceptanceDecisionTask().WaitingOn(); present {
		t.Fatalf("接受之后等待态仍是 %v", waitingOn)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.funds_freeze`); n != 1 {
		t.Fatalf("冻结账本 %d 行, want 1——控制没有按两件试算合计占下这一笔", n)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 1 {
		t.Fatalf("接受信封 = %d 封, want 恰好 1", n)
	}
}

// Covers: 票 declared-goods-lines/26 完成判据二——卡绑的分区目录按始发邮编前缀分表而寄件邮编没报：计价答输入不全缺
// ORIGIN_POSTAL（不再走完评价停 ZONE_UNRESOLVED 无合计那一路），委托停`等待受控补充`，缺口委托级一条、缺项码原样。
func TestAMissingOriginPostalOnAPrefixSplitCatalogueWaitsForCustomerSupplement(t *testing.T) {
	control := &synRLateBoundControl{}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: control})
	fixture.commercial.asOfAt = submissionReceiptAsOf(t, fixture.requests)
	ctx := t.Context()

	settlement := wireGapSplitSettlement(t, fixture.synVerticalFixture, fixture.commercial, control, gapSplitTrialMinor)
	registerDemoZoneCatalogue(t, fixture.db)
	registerPriceCardForTest(t, fixture.db, gapSplitSellCard(t, "v1", true))
	drainGapSplitPricingEnvelopes(t, fixture.store)

	// 收件邮编在、寄件邮编缺：路线拼得出来（始发那半空着），目录的始发维度按前缀分表——缺的是始发那半，
	// 不是整条路线。
	gapSplitSubmit(t, ctx, fixture.synVerticalFixture, []gapSplitParcel{
		{id: "SYN-PARCEL-01", kilograms: "2"},
	}, "", "018000")
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})

	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	if err := fixture.submittedGate.Consume(ctx, submittedDelivery.Envelope); err != nil {
		t.Fatalf("等待受控补充必须按处理完毕入账：%v", err)
	}
	if settlement.trials.asked != 1 {
		t.Fatalf("试算被问 %d 次, want 1", settlement.trials.asked)
	}
	paused := fixture.mustLoadRequest(t, ctx)
	if waitingOn, present := paused.AcceptanceDecisionTask().WaitingOn(); !present || waitingOn != psdomain.ResumeByCustomerSupplement {
		t.Fatalf("等待态 = %v（present=%v）, want CUSTOMER_SUPPLEMENT", waitingOn, present)
	}
	wantGaps := []gapSplitGapShape{{parcel: "", reason: "ESTIMATE_ORIGIN_POSTAL_MISSING"}}
	if got := gapSplitGapShapes(paused.AcceptanceDecisionTask().SupplementGaps()); !slices.Equal(got, wantGaps) {
		t.Fatalf("任务缺口 = %+v, want %+v", got, wantGaps)
	}
	if queue := fixture.listSupplementQueue(t, ctx); len(queue) != 1 || queue[0].ShipmentRequestID != fixture.requestID {
		t.Fatalf("补件队列 = %+v, want 恰好这一份委托", queue)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.acceptance_processing_attempt
		WHERE reason_ref = 'CUSTOMER_SUPPLEMENT_PENDING' AND resume_path = 'CUSTOMER_SUPPLEMENT'
		  AND settlement_reason = 'ESTIMATE_DECLARATION_INCOMPLETE'
		  AND estimate_gap_codes = ARRAY['ORIGIN_POSTAL']::text[]`); n != 1 {
		t.Fatalf("带 ORIGIN_POSTAL 缺项码的处理尝试 = %d 行, want 1", n)
	}
}

// Covers: 票 declared-goods-lines/26 完成判据三——卡要直接给分区（没绑分区目录）：缺项 ZONE 只有登记才解得开，委托停
// `等待运营登记`（票 23 那一路），处理尝试带缺项码；原卡按 ADR-0234 收尾、换上一张绑分区目录且生效期覆盖提交时点的
// 售价卡之后重驱、接受。中途直接另登一张在用卡落适用冲突、解不开——那一拍也取了证。
func TestAZoneGapWaitsForOperatorRegistrationUntilACatalogueBoundCardIsInForce(t *testing.T) {
	control := &synRLateBoundControl{}
	fixture := newOperatorRegistrationLoopFixtureWith(t, control)
	fixture.commercial.configured = true
	fixture.commercial.asOfAt = submissionReceiptAsOf(t, fixture.requests)
	ctx := t.Context()

	settlement := wireGapSplitSettlement(t, fixture.synVerticalFixture, fixture.commercial, control, gapSplitTrialMinor)
	registerDemoZoneCatalogue(t, fixture.db)
	registerPriceCardForTest(t, fixture.db, gapSplitSellCard(t, "v1", false))
	drainGapSplitPricingEnvelopes(t, fixture.store)

	// 申报齐全（重量、收寄邮编、产品）：缺的不是客户那一侧，是这张卡读不到的分区。
	gapSplitSubmit(t, ctx, fixture.synVerticalFixture, []gapSplitParcel{
		{id: "SYN-PARCEL-01", kilograms: "2"},
	}, "200000", "018000")
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})

	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	if err := fixture.submittedGate.Consume(ctx, submittedDelivery.Envelope); err != nil {
		t.Fatalf("等待运营登记必须按处理完毕入账：%v", err)
	}
	if settlement.trials.asked != 1 {
		t.Fatalf("试算被问 %d 次, want 1", settlement.trials.asked)
	}
	fixture.assertWaitingOnRegistration(t, ctx)
	if gaps := fixture.mustLoadRequest(t, ctx).AcceptanceDecisionTask().SupplementGaps(); len(gaps) != 0 {
		t.Fatalf("等运营登记却在任务上落了 %d 条补件缺口", len(gaps))
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.acceptance_processing_attempt
		WHERE reason_ref = 'FINANCIAL_CONTROL_NOT_CONFIGURED' AND resume_path = 'OPERATOR_REGISTRATION'
		  AND settlement_reason = 'ESTIMATE_DECLARATION_INCOMPLETE' AND service_product = '`+pricingResumeProduct+`'
		  AND estimate_gap_codes = ARRAY['ZONE']::text[]`); n != 1 {
		t.Fatalf("带 ZONE 缺项码的处理尝试 = %d 行, want 1——登记那一路没有留下计价当时点的名", n)
	}
	fixture.markPublished(t, submittedDelivery)

	// 直接另登一张绑目录的 v2：两版在判断时点同时适用，SELL 那一封叫醒委托，落适用冲突、照旧停，本封入账。
	registerPriceCardForTest(t, fixture.db, gapSplitSellCard(t, "v2", true))
	conflictDelivery := fixture.claimDeliveryOfType(t, string(psinbox.PricingParameterRegisteredEventType))
	if err := fixture.pricingParameterGate.Consume(ctx, conflictDelivery.Envelope); err != nil {
		t.Fatalf("适用冲突解不开，本封应按处理完毕入账：%v", err)
	}
	fixture.markPublished(t, conflictDelivery)
	if settlement.trials.asked != 2 {
		t.Fatalf("v2 登上之后试算被问 %d 次, want 2——委托没被叫醒", settlement.trials.asked)
	}
	fixture.assertWaitingOnRegistration(t, ctx)
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.acceptance_processing_attempt
		WHERE settlement_reason = 'ESTIMATE_PRICE_CARD_CONFLICT'`); n != 1 {
		t.Fatalf("适用冲突那一轮的处理尝试 = %d 行, want 1", n)
	}

	// 按 ADR-0234 让 v1 在 v2 的同起点覆盖下退役：收尾那一封重驱，计价选到 v2，路线经目录解出 Z1，
	// 估价形成、冻结占下、接受成立。
	retireGapSplitSellCard(t, fixture.db)
	closureDelivery := fixture.claimDeliveryOfType(t, string(psinbox.PricingParameterRegisteredEventType))
	if err := fixture.pricingParameterGate.Consume(ctx, closureDelivery.Envelope); err != nil {
		t.Fatalf("收尾那一封的续办一拍：%v", err)
	}
	accepted := fixture.mustLoadRequest(t, ctx)
	if accepted.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("续办后状态 = %q, want ACCEPTED", accepted.State())
	}
	if remaining := fixture.listRegistrationQueue(t, ctx); len(remaining) != 0 {
		t.Fatalf("接受之后等待运营登记队列仍列出 %d 行", len(remaining))
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.funds_freeze`); n != 1 {
		t.Fatalf("冻结账本 %d 行, want 1", n)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 1 {
		t.Fatalf("接受信封 = %d 封, want 恰好 1", n)
	}
}

// Covers: 票 declared-goods-lines/26 完成判据四——客户可补的缺项与卡侧缺项同场：登记优先，停`等待运营登记`，任务上
// 不落补件缺口，处理尝试带全部缺项码原样（ADR-0381 决定一）。
func TestMixedCustomerAndRegistrationGapsWaitForOperatorRegistrationFirst(t *testing.T) {
	control := &synRLateBoundControl{}
	fixture := newOperatorRegistrationLoopFixtureWith(t, control)
	fixture.commercial.configured = true
	fixture.commercial.asOfAt = submissionReceiptAsOf(t, fixture.requests)
	ctx := t.Context()

	settlement := wireGapSplitSettlement(t, fixture.synVerticalFixture, fixture.commercial, control, gapSplitTrialMinor)
	registerPriceCardForTest(t, fixture.db, gapSplitSellCard(t, "v1", false))
	drainGapSplitPricingEnvelopes(t, fixture.store)

	// P-01 缺重量（客户补得上），卡又读不到分区（登记才解得开）：两类同场。
	gapSplitSubmit(t, ctx, fixture.synVerticalFixture, []gapSplitParcel{
		{id: "SYN-PARCEL-01"},
		{id: "SYN-PARCEL-02", kilograms: "2"},
	}, "200000", "018000")
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})

	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	if err := fixture.submittedGate.Consume(ctx, submittedDelivery.Envelope); err != nil {
		t.Fatalf("登记优先那一停必须按处理完毕入账：%v", err)
	}
	if settlement.trials.asked != 1 {
		t.Fatalf("试算被问 %d 次, want 1", settlement.trials.asked)
	}
	fixture.assertWaitingOnRegistration(t, ctx)
	if gaps := fixture.mustLoadRequest(t, ctx).AcceptanceDecisionTask().SupplementGaps(); len(gaps) != 0 {
		t.Fatalf("登记优先却在任务上落了 %d 条补件缺口——两类同场不得先催客户", len(gaps))
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.acceptance_processing_attempt
		WHERE reason_ref = 'FINANCIAL_CONTROL_NOT_CONFIGURED' AND resume_path = 'OPERATOR_REGISTRATION'
		  AND settlement_reason = 'ESTIMATE_DECLARATION_INCOMPLETE'
		  AND estimate_gap_codes = ARRAY['MEASUREMENT','ZONE']::text[]`); n != 1 {
		t.Fatalf("带两类缺项码的处理尝试 = %d 行, want 1——先见先记去重的那份点名没留下", n)
	}
}

// Covers: 票 declared-goods-lines/26 完成判据五（ADR-0381 决定一「缺项缺席」那一格）——`申报不全`没带任何缺项时照旧
// 内部重试：链停`未形成`，提交门交回未决哨兵、整笔回滚，等待态不落、处理尝试一行不留。
func TestAnIncompleteAnswerWithoutGapsStillRollsBack(t *testing.T) {
	control := &synRLateBoundControl{}
	fixture := newOperatorRegistrationLoopFixtureWith(t, control)
	fixture.commercial.configured = true
	ctx := t.Context()

	settlement := wirePricedSettlement(t, fixture, control)
	settlement.trials.answer = func(context.Context, saports.PreAcceptanceTrialQuery) saports.PreAcceptanceTrialAnswer {
		return saports.PreAcceptanceTrialAnswer{
			Outcome:   saports.PreAcceptanceTrialsIncomplete,
			PriceCard: "PRICE_CARD:" + gapSplitPlanID + "@v1",
		}
	}

	fixture.submitDeclaringProduct(t, ctx, pricingResumeProduct)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})
	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	err := fixture.submittedGate.Consume(ctx, submittedDelivery.Envelope)
	if !errors.Is(err, psinbox.ErrAcceptanceChainUndecided) || !strings.Contains(err.Error(), "FINANCIAL_CONTROL_NOT_FORMED") {
		t.Fatalf("缺项缺席应交回未决哨兵、停在`未形成`，实际 err = %v", err)
	}
	if n := fixture.countInbox(t, "parcel-shipment/advance-acceptance-chain", string(submittedDelivery.Envelope.ID)); n != 0 {
		t.Fatalf("内部重试必须让账本无痕等重投，实际 inbox 行数 = %d", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.acceptance_processing_attempt`); n != 0 {
		t.Fatalf("回滚之后处理尝试 = %d 行, want 0", n)
	}
	if _, present := fixture.mustLoadRequest(t, ctx).AcceptanceDecisionTask().WaitingOn(); present {
		t.Fatal("缺项缺席那一轮落了等待态")
	}
}

// gapSplitParcel 是一件申报的最小描述：kilograms 空串即这一件不报测量。
type gapSplitParcel struct {
	id        string
	kilograms string
}

// gapSplitGapShape 把一条任务缺口折成可比形：委托级的缺口 parcel 为空串。
type gapSplitGapShape struct {
	parcel string
	reason string
}

func gapSplitGapShapes(gaps []psdomain.SupplementGap) []gapSplitGapShape {
	shapes := make([]gapSplitGapShape, 0, len(gaps))
	for _, gap := range gaps {
		shape := gapSplitGapShape{reason: gap.Reason().String()}
		if parcel, present := gap.Parcel(); present {
			shape.parcel = parcel.String()
		}
		shapes = append(shapes, shape)
	}
	return shapes
}

// wireGapSplitSettlement 照 wirePricedSettlement 登好 SA 那一侧（结算账户、采用行、入账余额——余额由用例按要冻几件给），
// 把生产的 PS→SA 适配器装进控制那一口；试算那一口装**生产的受理前试算链**，缺项由计价的输入完备性门真点名。
func wireGapSplitSettlement(
	t *testing.T,
	base *synVerticalFixture,
	commercial *synRSwitchableAsOfCommercialBasis,
	control *synRLateBoundControl,
	balanceMinor int64,
) *synRSettlementControl {
	t.Helper()
	ctx := t.Context()
	tenant := mustPS(t, sadomain.NewTenantID, base.identity.TenantID().String())
	accounts, err := sapostgres.NewSettlementAccounts(base.db)
	if err != nil {
		t.Fatalf("构造结算账户登记册：%v", err)
	}
	adoptions, err := sapostgres.NewPreAcceptanceEstimateAdoptions(base.db)
	if err != nil {
		t.Fatalf("构造采用册：%v", err)
	}
	freezes, err := sapostgres.NewFreezeLedgers(base.db)
	if err != nil {
		t.Fatalf("构造冻结账本：%v", err)
	}
	balances, err := sapostgres.NewOperationalBalances(base.db, freezes)
	if err != nil {
		t.Fatalf("构造运营余额：%v", err)
	}
	account := synRReceivableAccount(t)
	adoption, err := sadomain.NewPreAcceptanceEstimateAdoption(account.ID(), sadomain.PerParcelSellTrialSum,
		mustPS(t, sadomain.NewEstimatePricingScope, pricingResumeScope))
	if err != nil {
		t.Fatalf("采用行：%v", err)
	}
	scope, err := sadomain.NewSettlementScope(
		mustPS(t, sadomain.NewLegalEntityReference, synRLegalEntity), account.ID(), mustPS(t, sadomain.NewCurrencyCode, synRCurrency))
	if err != nil {
		t.Fatalf("结算作用域：%v", err)
	}
	registeredAt := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	mustWithinTX(t, base.transactor, ctx, func(txCtx context.Context) error {
		if _, err := accounts.Save(txCtx, tenant, account, registeredAt); err != nil {
			return err
		}
		if _, err := adoptions.SaveAdoption(txCtx, tenant, adoption, registeredAt); err != nil {
			return err
		}
		_, err := balances.RecordBalance(txCtx, tenant, scope, sapostgres.BalancePosting{
			PostedMinor: balanceMinor,
			AsOf:        registeredAt,
		})
		return err
	})
	directory, err := pssettlement.NewRegisteredAccountDirectory(accounts)
	if err != nil {
		t.Fatalf("构造账户目录：%v", err)
	}
	settlement := newSynRSettlementControl(t, base.db)
	settlement.trials.answer = gapSplitProductionTrials(t, base.db)
	control.inner = pssettlement.NewPreAcceptanceControlAdapter(pssettlement.PreAcceptanceControlAdapterDeps{
		Apply: settlement.apply,
		Scopes: pssettlement.NewPolicyBackedControlScopeSource(
			&synRSettledCommercialBasis{synRSwitchableAsOfCommercialBasis: commercial}, directory),
	})
	return settlement
}

// gapSplitProductionTrials 让试算那一口答生产受理前试算链（assemble.go 的 preAcceptanceTrials）的原话：真价卡册按
// （租户、登记的计价范围、SELL·客户费用、估价时点、申报产品）选卡，按提交版本读申报，缺项由输入完备性门点名。
// 解析在收到的 ctx 上做，理由同 cardBackedTrials：重驱那一轮在自己的顶层事务里，另开连接会等不到。
func gapSplitProductionTrials(
	t *testing.T,
	db *bentopg.DB,
) func(context.Context, saports.PreAcceptanceTrialQuery) saports.PreAcceptanceTrialAnswer {
	t.Helper()
	trials, err := preAcceptanceTrials(db, systemClock{})
	if err != nil {
		t.Fatalf("装配生产受理前试算链：%v", err)
	}
	return func(ctx context.Context, query saports.PreAcceptanceTrialQuery) saports.PreAcceptanceTrialAnswer {
		answer, err := trials.FormPreAcceptanceTrials(ctx, query)
		if err != nil {
			t.Errorf("生产受理前试算链：%v", err)
			return saports.PreAcceptanceTrialAnswer{}
		}
		return answer
	}
}

// gapSplitSellCard 造一张演示租户的 SELL·客户费用合成卡：计价范围照采用行登的那一个、币种与结算作用域同一个、
// Z1 一行 [0, 30] 公斤 73、声明金额取整。catalogueBound 为真即绑演示分区目录 SYN-CAT-ZONE-CN-SG（始发按邮编前缀
// 分表），为假即这张卡要调用方直接给分区——受理前申报里没有那一格，正是缺项 ZONE 的形。
func gapSplitSellCard(t *testing.T, version string, catalogueBound bool) ppdomain.PriceCardRegistration {
	t.Helper()
	plan := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, gapSplitPlanID, version),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, gapSplitPlanID+"-TABLE", version),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, gapSplitPlanID+"-WEIGHT", version),
		Scope:                 pricingResumeScope,
		Direction:             ppdomain.PricingDirectionSell,
		Purpose:               ppdomain.PricingPurposeCustomerCharge,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              synRCurrency,
		Period:                pptest.Period{StartsAt: gapSplitCardStart},
		RateEntryID:           gapSplitPlanID + "-Z1-" + version,
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            "73",
		WeightRounding:        ppdomain.RoundingNone,
		WeightStepKilograms:   "1",
		AmountRounding: &pptest.AmountRoundingSpec{
			Mode: ppdomain.RoundingHalfUp, Increment: "0.01", Points: []ppdomain.AmountRoundingPoint{ppdomain.AmountRoundingTotal},
		},
	})
	if catalogueBound {
		link, err := ppdomain.NewReferenceCatalogueLink(ppdomain.CatalogueKindZone, "SYN-CAT-ZONE-CN-SG")
		if err != nil {
			t.Fatalf("分区目录绑定：%v", err)
		}
		structures, err := plan.Structures().WithReferenceCatalogues(link)
		if err != nil {
			t.Fatalf("方案结构绑分区目录：%v", err)
		}
		plan, err = ppdomain.NewPricingPlanVersion(plan.Reference(), plan.Scope(), plan.Direction(), plan.Purpose(),
			plan.BaseChargeCode(), plan.EffectivePeriod(), plan.RateTable(), plan.WeightPolicy(), nil, structures)
		if err != nil {
			t.Fatalf("绑目录的卡 %s：%v", version, err)
		}
	}
	return pricingResumeRegistration(t, gapSplitPlanID+"-"+version, plan)
}

// retireGapSplitSellCard 经生产的收尾用例让 v1 在 v2 的同起点覆盖下退役（ADR-0234），同一笔事务里锁、判、写、交信封。
func retireGapSplitSellCard(t *testing.T, db *bentopg.DB) {
	t.Helper()
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatalf("构造价卡库：%v", err)
	}
	handler, err := ppapplication.NewClosePriceCardVersionHandler(ppapplication.ClosePriceCardVersionDeps{
		Registry: cards, Registered: pricingParameterHandoffForTest(t, db), Clock: systemClock{},
	})
	if err != nil {
		t.Fatalf("构造收尾用例：%v", err)
	}
	var result ppapplication.PriceCardClosureResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		var err error
		result, err = handler.RetireAtCoveringSuccessor(ctx, ppapplication.RetirePriceCardVersionCommand{
			PriceCardClosureTarget: ppapplication.PriceCardClosureTarget{
				Tenant: ppValue(t, ppdomain.NewTenantID, demoTenant), PlanID: gapSplitPlanID, Version: "v1", Successor: "v2",
			},
			Reference: "SYN-RETIRE/" + gapSplitPlanID,
		})
		return err
	})
	if cause := result.Cause(); cause != nil {
		t.Fatalf("收尾 v1 不成立：%v", cause)
	}
	if result.Outcome() != ppapplication.PriceCardClosureRecorded {
		t.Fatalf("收尾 v1 = %s", result.Outcome())
	}
}

// gapSplitSubmit 照 SYN 纵切的提交配方建单，另按用例给的件、测量与收寄邮编申报；产品照 pricingResumeProduct 申报
// ——处理尝试那一格要带它。
func gapSplitSubmit(
	t *testing.T,
	ctx context.Context,
	base *synVerticalFixture,
	parcels []gapSplitParcel,
	senderPostal, deliveryPostal string,
) {
	t.Helper()
	command := base.submitCommand(t)
	command.DeclaredParcelIDs = nil
	command.DeclaredProfiles = nil
	for _, parcel := range parcels {
		command.DeclaredParcelIDs = append(command.DeclaredParcelIDs, mustPS(t, psdomain.NewDeclaredParcelID, parcel.id))
		if parcel.kilograms != "" {
			command.DeclaredProfiles = append(command.DeclaredProfiles, gapSplitProfile(t, parcel.id, parcel.kilograms))
		}
	}
	command.DeclaredElements = gapSplitPostalElements(t, senderPostal, deliveryPostal)
	command.RequestedServiceProduct = gapSplitDeclaredProduct(t)
	var outcome psapplication.SubmitOutcome
	mustWithinTX(t, base.transactor, ctx, func(txCtx context.Context) error {
		result, err := base.submitHandler.Handle(txCtx, command)
		if err != nil {
			return err
		}
		outcome = result.Outcome()
		return nil
	})
	if outcome != psapplication.OutcomeSubmitted {
		t.Fatalf("submit outcome = %q, want SUBMITTED", outcome)
	}
}

// gapSplitSupplementOn 以 basis 为基准发一次受控补充：整份重报——成员集合不变，测量、收寄邮编与产品全在载荷里
// （画像与要素不从旧版本继承）。补充请求有自己的来源身份与载荷摘要。
func gapSplitSupplementOn(
	t *testing.T,
	ctx context.Context,
	fixture *customerSupplementLoopFixture,
	basis psdomain.SubmissionVersionID,
	parcels []gapSplitParcel,
	senderPostal, deliveryPostal string,
) psapplication.FormNewSubmissionVersionResult {
	t.Helper()
	supplementIdentity, err := psdomain.NewSourceIdentity(
		fixture.identity.TenantID(),
		fixture.identity.CustomerAccountID(),
		fixture.identity.Source(),
		mustPS(t, psdomain.NewSourceRequestKey, "SYN-KEY-01-SUPPLEMENT"),
	)
	if err != nil {
		t.Fatalf("补充来源身份：%v", err)
	}
	now := time.Now().UTC()
	command := psapplication.FormNewSubmissionVersionCommand{
		Identity:                fixture.identity,
		SupplementIdentity:      supplementIdentity,
		PayloadDigest:           mustPS(t, psdomain.NewPayloadDigest, "SYN-DIGEST-02"),
		OccurredAt:              now.Add(-2 * time.Second),
		ReceivedAt:              now.Add(-time.Second),
		ShipmentRequestID:       fixture.requestID,
		BasisVersion:            basis,
		DeclaredElements:        gapSplitPostalElements(t, senderPostal, deliveryPostal),
		RequestedServiceProduct: gapSplitDeclaredProduct(t),
	}
	for _, parcel := range parcels {
		command.DeclaredParcelIDs = append(command.DeclaredParcelIDs, mustPS(t, psdomain.NewDeclaredParcelID, parcel.id))
		if parcel.kilograms != "" {
			command.DeclaredProfiles = append(command.DeclaredProfiles, gapSplitProfile(t, parcel.id, parcel.kilograms))
		}
	}
	result, err := fixture.supplement.Handle(ctx, command)
	if err != nil {
		t.Fatalf("受控补充：%v", err)
	}
	return result
}

// gapSplitProfile 立一件只报毛重的声明画像：单位原词 KG 进计价的封闭单位表。
func gapSplitProfile(t *testing.T, parcel, kilograms string) psdomain.DeclaredParcelProfile {
	t.Helper()
	weight, err := psdomain.NewDeclaredWeight(
		mustPS(t, psdomain.NewMeasurementValue, kilograms),
		mustPS(t, psdomain.NewMeasurementUnitReference, "KG"),
	)
	if err != nil {
		t.Fatalf("申报毛重：%v", err)
	}
	measurement, err := psdomain.NewDeclaredMeasurement(weight, psdomain.DeclaredDimensions{})
	if err != nil {
		t.Fatalf("申报测量：%v", err)
	}
	profile, err := psdomain.NewDeclaredParcelProfile(mustPS(t, psdomain.NewDeclaredParcelID, parcel), measurement)
	if err != nil {
		t.Fatalf("声明画像：%v", err)
	}
	return profile
}

// gapSplitPostalElements 立收寄两段的邮编要素：空串即该段没报（整段缺席），与「报了空白」分得开。
func gapSplitPostalElements(t *testing.T, senderPostal, deliveryPostal string) psdomain.DeclaredAddressElements {
	t.Helper()
	group := func(scope psdomain.SourceDataGroupReference, postal string) psdomain.AddressElements {
		if postal == "" {
			return psdomain.AddressElementsOf(scope, nil)
		}
		entry, err := psdomain.NewCanonicalContentEntry(psdomain.AddressElementEntryName(scope, psdomain.PostalCodeElement), postal)
		if err != nil {
			t.Fatalf("邮编条目：%v", err)
		}
		return psdomain.AddressElementsOf(scope, []psdomain.CanonicalContentEntry{entry})
	}
	return psdomain.NewDeclaredAddressElements(
		group(psdomain.SenderPlaceDataGroup(), senderPostal),
		group(psdomain.DeliveryPlaceDataGroup(), deliveryPostal),
	)
}

// gapSplitDeclaredProduct 按与计价参数那条闭环同一个产品字面立服务产品声明。
func gapSplitDeclaredProduct(t *testing.T) psdomain.DeclaredServiceProduct {
	t.Helper()
	entry, err := psdomain.NewCanonicalContentEntry(psdomain.RequestedServiceProductEntryName, pricingResumeProduct)
	if err != nil {
		t.Fatalf("服务产品条目：%v", err)
	}
	return psdomain.RequestedServiceProductOf([]psdomain.CanonicalContentEntry{entry})
}

// drainGapSplitPricingEnvelopes 把夹具搭建期铸的「参数已登记」信封（登目录、登卡各一封）定稿掉：之后用例按类型认领
// 时，拿到的才是自己那一步铸的那一封。
func drainGapSplitPricingEnvelopes(t *testing.T, store *outbox.Store) {
	t.Helper()
	deliveries, err := store.Claim(t.Context(), eventing.OutboxClaim{
		Now:         time.Now().UTC().Add(time.Minute),
		Limit:       50,
		LeaseFor:    time.Minute,
		MaxAttempts: 50,
	})
	if err != nil {
		t.Fatalf("认领搭建期信封：%v", err)
	}
	for _, delivery := range deliveries {
		if string(delivery.Envelope.Type) != string(psinbox.PricingParameterRegisteredEventType) {
			continue
		}
		if err := store.MarkPublished(t.Context(), delivery.Ref, time.Now().UTC()); err != nil {
			t.Fatalf("定稿搭建期信封 %s：%v", delivery.Envelope.ID, err)
		}
	}
}

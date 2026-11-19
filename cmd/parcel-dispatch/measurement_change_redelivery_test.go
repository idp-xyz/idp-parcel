package main

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证实测变化的处理方自己是幂等的，不靠收件箱那一道去重兜底：同一次变化（同一个来源身份）换个信封身份再交给
// 生产装配里的消费者办一遍，收件箱不认它是办过的，处理方沿代次链认出这次变化已经另起过一代，销售与采购两路都不再
// 多出任何东西。
//
// 绕开收件箱的办法是把 outbox 里那封实测变化信封原样再入队一份、只换信封 ID（reissueUnderAnotherEnvelopeID）：
// 收件箱按（消费者、来源、信封 ID）记账，换了信封 ID 就是一条没见过的键，信封仍走路由表、派发器与收件箱门的整条
// 生产路径，不是在测试里直接调处理方。信封的载荷从库里读回而不重拼，证的是「同一份内容」。
//
// 两个起点各走一遍：前一笔预估未确认（新一代的评价替代它）、前一笔预估已确认（新一代的评价结清差额调整）。采购那一路
// 在两个起点下都带一份已有预期成本的请求，新一代的评价让预期成本重述。快照取整个 settlement_accounting schema 的
// 每张表（行数加内容摘要）与 outbox 里除实测变化信封以外的信封标识集：多出一行、改了一格、多发一封都显形，不靠
// 事先猜哪几张表会变。证据只记 S。

const (
	measurementRedeliveryUnit       = "SYN-HU-MC-01"
	measurementRedeliveryParcel     = "SYN-PARCEL-01"
	measurementRedeliveryOccurrence = "SYN-OCC-TRUNK-01"
	measurementRedeliveryBuyFirst   = "EVREQ-SYN-MC-BUY-1"
	measurementRedeliveryEnvelope   = "SYN-MC-REDELIVERY-1"
	measurementRedeliverySellSource = "SELLREQ-SYN-CONFIRM"
)

// Covers: 实测变化消费者在生产装配上的幂等：同一次变化换信封身份再办一遍，销售（替代 / 差额调整两条路）与采购两路都
// 不多出请求代次、费用、替代、调整、预期成本与信封。
func TestARedeliveredMeasurementChangeAddsNothingOnTheProductionAssembly(t *testing.T) {
	for _, start := range []struct {
		name      string
		confirmed bool
	}{
		{name: "前一笔预估未确认：走替代", confirmed: false},
		{name: "前一笔预估已确认：走差额调整", confirmed: true},
	} {
		t.Run(start.name, func(t *testing.T) { redeliverMeasurementChange(t, start.confirmed) })
	}
}

func redeliverMeasurementChange(t *testing.T, confirmed bool) {
	observed := map[string]error{}
	beat, db, store := wiredBeat(t, dispatch.WithDeliveryFailureObserver(func(delivery eventing.Delivery, _ eventing.FailureCode, err error) {
		observed[string(delivery.Envelope.ID)] = err
	}))
	tenant := saValue(t, sadomain.NewTenantID, demoTenant)

	seedConfirmingTenant(t, db, tenant)
	adoptReweighAdjustment(t, db, tenant)
	registerTrunkAgreement(t, db, demoTenant)
	insertChargeOccurrence(t, db, demoTenant, measurementRedeliveryOccurrence, tfdomain.ActualFulfillmentOccurrence, trunkAgreementObject+"/v1")
	seedMeasuredReception(t, db)
	buyRequests, err := sapostgres.NewEvaluationRequests(db)
	if err != nil {
		t.Fatalf("SA BUY 请求册：%v", err)
	}
	firstBuy := measurementBuyRequest(t, tenant, measurementRedeliveryBuyFirst)
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		_, err := buyRequests.Save(txCtx, firstBuy)
		return err
	})

	// 第一代：销售与采购各一份评价，一拍到底，形成预估客户费用与供应商预期成本首版。
	recordEvaluations(t, db, store,
		measurementEvaluation(t, measurementEvaluationSpec{
			id: "SYN-EVAL-SELL-MC-1", request: measurementRedeliverySellSource, cardVersion: "v1", rate: "100", currency: "CNY",
			direction: ppdomain.PricingDirectionSell, purpose: ppdomain.PricingPurposeCustomerCharge,
		}),
		measurementEvaluation(t, measurementEvaluationSpec{
			id: "SYN-EVAL-BUY-MC-1", request: measurementRedeliveryBuyFirst, cardVersion: "v1", rate: "12.5", currency: "USD",
			direction: ppdomain.PricingDirectionBuy, purpose: ppdomain.PricingPurposeSupplierCost,
		}))
	settleBeats(t, beat)
	assertEveryEnvelopePublished(t, db, observed, "第一代评价")

	charges, err := sapostgres.NewCustomerCharges(db)
	if err != nil {
		t.Fatalf("SA 客户费用库：%v", err)
	}
	costs, err := sapostgres.NewSupplierExpectedCosts(db)
	if err != nil {
		t.Fatalf("SA 预期成本库：%v", err)
	}
	firstCharge := saValue(t, sadomain.NewCustomerChargeID, "SELLCC-"+measurementRedeliverySellSource)
	if charge, found, err := charges.FindByID(t.Context(), tenant, firstCharge); err != nil || !found || charge.Stage() != sadomain.ChargeEstimated {
		t.Fatalf("第一代费用：found=%v err=%v 阶段=%s，want 预估", found, err, charge.Stage())
	}
	firstCost := saValue(t, sadomain.NewSupplierCostVersionID, "BUYSC-"+measurementRedeliveryBuyFirst)
	if _, found, err := costs.LoadExpectedCost(t.Context(), tenant, firstCost); err != nil || !found {
		t.Fatalf("第一代预期成本：found=%v err=%v", found, err)
	}

	if confirmed {
		confirmFirstCharge(t, beat, db, store, tenant, firstCharge)
		assertEveryEnvelopePublished(t, db, observed, "终局确认")
	}

	// 第二代：一次真的实测变化（来源 SYN-DEVICE-MC-01/WEIGH-2）各另起一代，评价重算出不同的合计。
	const firstSource = "SYN-DEVICE-MC-01/WEIGH-2"
	noticeMeasurementChange(t, db, store, firstSource)
	changeEnvelope := outboxEventIDOfSubject(t, db, nopostgres.ActualMeasurementChangedEventType, firstSource)
	if published, err := beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("实测变化那一拍：published = %d err = %v, want 1", published, err)
	}
	if status := outboxStatus(t, db, changeEnvelope); status != "PUBLISHED" {
		t.Fatalf("实测变化信封 status = %q；失败码 = %q 原文 = %v", status, recordedFailureCode(t, db, changeEnvelope), observed[changeEnvelope])
	}
	sellChain := requestChain(t, db, "sell_evaluation_request")
	buyChain := requestChain(t, db, "evaluation_request")
	wantBasis, err := sadomain.NewMeasurementChangeResumeBasis(firstSource)
	if err != nil {
		t.Fatalf("续办依据：%v", err)
	}
	for name, chain := range map[string][]requestLink{"销售": sellChain, "采购": buyChain} {
		if len(chain) != 2 || chain[1].generation != 2 || chain[1].previous != chain[0].id || chain[1].basis != wantBasis.String() {
			t.Fatalf("%s请求代次链 = %+v，want 两代、第二代回指第一代、依据 %s", name, chain, wantBasis)
		}
	}
	// 新一代的请求信封里 PP 会按回指找评价：评价先于那一拍落库，PP 的消费者就认出这一代已有评价，不再另造一份。
	recordEvaluations(t, db, store,
		measurementEvaluation(t, measurementEvaluationSpec{
			id: "SYN-EVAL-SELL-MC-2", request: sellChain[1].id, cardVersion: "v2", rate: "130", currency: "CNY",
			direction: ppdomain.PricingDirectionSell, purpose: ppdomain.PricingPurposeCustomerCharge,
		}),
		measurementEvaluation(t, measurementEvaluationSpec{
			id: "SYN-EVAL-BUY-MC-2", request: buyChain[1].id, cardVersion: "v2", rate: "15", currency: "USD",
			direction: ppdomain.PricingDirectionBuy, purpose: ppdomain.PricingPurposeSupplierCost,
		}))
	settleBeats(t, beat)
	assertEveryEnvelopePublished(t, db, observed, "第二代评价")

	assertSecondGenerationOutcome(t, db, confirmed)
	if _, found, err := costs.LoadExpectedCost(t.Context(), tenant, saValue(t, sadomain.NewSupplierCostVersionID, "BUYSC-"+buyChain[1].id)); err != nil || !found {
		t.Fatalf("第二代预期成本：found=%v err=%v", found, err)
	}

	// 同一次变化换信封身份再办一遍。
	before := settlementFingerprint(t, db)
	reissueUnderAnotherEnvelopeID(t, &synVerticalFixture{db: db, transactor: db.Transactor()}, changeEnvelope, measurementRedeliveryEnvelope)
	if published, err := beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("重办那一拍：published = %d err = %v, want 1（只有换了身份的那一封）", published, err)
	}
	if status := outboxStatus(t, db, measurementRedeliveryEnvelope); status != "PUBLISHED" {
		t.Fatalf("重办的信封 status = %q；失败码 = %q 原文 = %v", status, recordedFailureCode(t, db, measurementRedeliveryEnvelope), observed[measurementRedeliveryEnvelope])
	}
	// 重办确实被处理方办了一遍而不是被收件箱拦掉：同一个消费者名下，新信封身份各记一行，原来那行还在。
	original, redelivered := inboxConsumersOf(t, db, changeEnvelope), inboxConsumersOf(t, db, measurementRedeliveryEnvelope)
	if len(original) != 1 || len(redelivered) != 1 || original[0] != redelivered[0] {
		t.Fatalf("收件箱消费者：原信封 %v、重办信封 %v，want 各一行且同一个消费者", original, redelivered)
	}
	settleBeats(t, beat)
	assertEveryEnvelopePublished(t, db, observed, "重办")
	after := settlementFingerprint(t, db)
	if changed := changedFingerprints(before, after); len(changed) != 0 {
		t.Fatalf("同一次变化再办一遍多出了东西：\n%v", changed)
	}

	// 对照：来源身份不同的下一次真变化照样另起一代，说明上面的零变化不是因为这条路在这个夹具里本来就是死的。
	noticeMeasurementChange(t, db, store, "SYN-DEVICE-MC-01/WEIGH-3")
	if published, err := beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("对照那一拍：published = %d err = %v, want 1", published, err)
	}
	for name, table := range map[string]string{"销售": "sell_evaluation_request", "采购": "evaluation_request"} {
		if chain := requestChain(t, db, table); len(chain) != 3 {
			t.Fatalf("%s请求代次链 = %+v，want 另一个来源的真变化另起第三代", name, chain)
		}
	}
}

// assertSecondGenerationOutcome 钉住第二代落在预期的那条路上，免得起点没走到而「再办一遍什么都没多」假绿。
func assertSecondGenerationOutcome(t *testing.T, db *bentopg.DB, confirmed bool) {
	t.Helper()
	want := map[string]int{
		"customer_charge":              2,
		"customer_charge_supersession": 1,
		"charge_adjustment":            0,
		"supplier_expected_cost":       2,
		"sell_evaluation_request":      2,
		"evaluation_request":           2,
	}
	if confirmed {
		want["customer_charge"], want["customer_charge_supersession"], want["charge_adjustment"] = 1, 0, 1
	}
	for table, rows := range want {
		if got := tableRows(t, db, table); got != rows {
			t.Fatalf("第二代之后 %s 有 %d 行, want %d（confirmed=%v）", table, got, rows, confirmed)
		}
	}
}

type measurementEvaluationSpec struct {
	id, request, cardVersion, rate, currency string
	direction                                ppdomain.PricingDirection
	purpose                                  ppdomain.PricingPurpose
}

// measurementEvaluation 真算一份回指 request 的评价：单行价表、不带附加费，合计就是 rate。重算的差额由 rate 的不同
// 给出，卡不经演示价卡，用例因此不随演示租户的卡配置变。
func measurementEvaluation(t *testing.T, spec measurementEvaluationSpec) ppdomain.PricingEvaluation {
	t.Helper()
	name := "SYN-MC-" + string(spec.direction)
	plan := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, name, spec.cardVersion),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, name+"-TABLE", spec.cardVersion),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, name+"-WEIGHT", spec.cardVersion),
		Scope:                 "SYN-SCOPE-01",
		Direction:             spec.direction,
		Purpose:               spec.purpose,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              spec.currency,
		Period:                pptest.Period{StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		RateEntryID:           "SYN-RATE-Z1",
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            spec.rate,
		WeightRounding:        ppdomain.RoundingNone,
		WeightStepKilograms:   "1",
		AmountRounding: &pptest.AmountRoundingSpec{
			Mode:      ppdomain.RoundingHalfUp,
			Increment: "0.01",
			Points:    []ppdomain.AmountRoundingPoint{ppdomain.AmountRoundingTotal},
		},
	})
	input := pptest.Input(t, pptest.InputSpec{
		Tenant: demoTenant, Scope: "SYN-SCOPE-01", PackageID: measurementRedeliveryParcel, Zone: "Z1", Kilograms: "1.2", BusinessAt: sellBasisAt,
	})
	evaluationRequest, err := ppdomain.NewEvaluationRequest(pptest.Value(t, ppdomain.NewEvaluationID, spec.id), plan, input, ppdomain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("评价请求：%v", err)
	}
	evaluationRequest, err = evaluationRequest.WithRequestReference(pptest.Value(t, ppdomain.NewEvaluationRequestReference, spec.request))
	if err != nil {
		t.Fatalf("回指：%v", err)
	}
	evaluation := ppdomain.EvaluatePricing(evaluationRequest)
	if evaluation.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("夹具评价 %s 状态 = %s %v", spec.id, evaluation.Status(), evaluation.Issues())
	}
	return evaluation
}

// recordEvaluations 经 PP 真评价库与真 Outbox 交接口落评价、各交一封评价已记录信封。
func recordEvaluations(t *testing.T, db *bentopg.DB, store *outbox.Store, evaluations ...ppdomain.PricingEvaluation) {
	t.Helper()
	repository, err := pppostgres.NewEvaluations(db)
	if err != nil {
		t.Fatalf("PP 评价库：%v", err)
	}
	handoff, err := pppostgres.NewOutboxEvaluationHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatalf("PP 评价交接：%v", err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		for _, evaluation := range evaluations {
			if _, err := repository.Save(txCtx, evaluation); err != nil {
				return err
			}
			if err := handoff.HandOffEvaluation(txCtx, ppports.EvaluationHandoffIntent{Evaluation: evaluation}); err != nil {
				return err
			}
		}
		return nil
	})
}

// settleBeats 一拍接一拍地跑到没有信封可派：一拍里入队的后继信封要到下一拍才被认领。
func settleBeats(t *testing.T, beat Beat) {
	t.Helper()
	for round := 0; round < 12; round++ {
		published, err := beat.DispatchOnce(t.Context())
		if err != nil {
			t.Fatalf("一拍：%v", err)
		}
		if published == 0 {
			return
		}
	}
	t.Fatal("连跑十二拍仍有信封被派出：链条没有收敛")
}

// assertEveryEnvelopePublished 钉住拍与拍之间没有留着等重试的信封：留着的信封会在后面某一拍无声地落库，让「再办一遍
// 什么都没多」的快照对不上是哪一封造成的。没有订阅者的信封（路由表上还没有接它的消费者，例如费用确认信封）不在此列：
// 它们留在 PENDING 是路由表的现状，不会再落任何库。
func assertEveryEnvelopePublished(t *testing.T, db *bentopg.DB, observed map[string]error, when string) {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	rows, err := querier.Query(t.Context(),
		`SELECT event_id, event_type, status, coalesce(failure_code, '')
		   FROM `+migrate.SchemaBento+`.outbox
		  WHERE status <> 'PUBLISHED' AND coalesce(failure_code, '') <> 'dispatch.no_subscriber'
		  ORDER BY enqueued_seq`)
	if err != nil {
		t.Fatalf("读未定稿信封：%v", err)
	}
	defer rows.Close()
	var stuck []string
	for rows.Next() {
		var id, eventType, status, code string
		if err := rows.Scan(&id, &eventType, &status, &code); err != nil {
			t.Fatalf("读未定稿信封：%v", err)
		}
		stuck = append(stuck, fmt.Sprintf("%s [%s] %s %s 原文=%v", id, eventType, status, code, observed[id]))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读未定稿信封：%v", err)
	}
	if len(stuck) != 0 {
		t.Fatalf("%s之后仍有信封没定稿为 PUBLISHED：\n%v", when, stuck)
	}
}

// adoptReweighAdjustment 登演示租户的结算账户采用实测差异自动调整：没登时已确认费用的差额不记调整，新一代请求停在
// 未采用上。
func adoptReweighAdjustment(t *testing.T, db *bentopg.DB, tenant sadomain.TenantID) {
	t.Helper()
	adoptions, err := sapostgres.NewReweighAdjustmentAdoptions(db)
	if err != nil {
		t.Fatalf("SA 差额调整采用册：%v", err)
	}
	account := saValue(t, sadomain.NewSettlementAccountID, "SYN-SA-SHIPPER-01-RECEIVABLE")
	at := beatInstant()
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		_, err := adoptions.SaveReweighAdjustmentAdoption(txCtx, tenant, account, "SYN-CLAUSE-REWEIGH-01", at)
		return err
	})
}

// seedMeasuredReception 在 NO 的收寄册上登一条当前有效的收寄：作业实物 measurementRedeliveryUnit 关联着恰好一件
// 包裹。实测变化的两路重评都先按这条关联解出包裹。
func seedMeasuredReception(t *testing.T, db *bentopg.DB) {
	t.Helper()
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		executor, err := db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(txCtx, `
			INSERT INTO node_operations.reception
				(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict, service_markers,
				 service_result_check, recorded_at)
			VALUES ($1, 'SYN-SCAN-MC-01', 'syn-digest', 'INTAKE_FORMED',
			        jsonb_build_object('unit', $2::text, 'association', $3::text, 'version', 'SYN-INTAKE-MC-01'),
			        '{}'::jsonb, '[]'::jsonb, false, '[]'::jsonb, 'CHECKED', now())`,
			demoTenant, measurementRedeliveryUnit, measurementRedeliveryParcel)
		return err
	})
}

// measurementBuyRequest 造一份干线发生项上的 BUY 评价请求首代：发生项、费用项目与协议同 registerTrunkAgreement /
// insertChargeOccurrence 登下的那一份。
func measurementBuyRequest(t *testing.T, tenant sadomain.TenantID, id string) saports.EvaluationRequestRecord {
	t.Helper()
	occurrence, err := sadomain.NewTransportChargeOccurrence(
		saValue(t, sadomain.NewChargeOccurrenceID, measurementRedeliveryOccurrence),
		saValue(t, sadomain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"),
		saValue(t, sadomain.NewOccurrenceVersion, "V1"),
		sellBasisAt,
	)
	if err != nil {
		t.Fatalf("发生项引用：%v", err)
	}
	request, err := sadomain.SubmitEvaluationRequest(sadomain.EvaluationRequestSpec{
		ID:      saValue(t, sadomain.NewEvaluationRequestID, id),
		Scope:   saValue(t, sadomain.NewPrimaryScopeReference, "SYN-SCOPE-01"),
		Purpose: sadomain.BuySupplierCost,
		Sources: sadomain.EligibleSourceReferences{
			Occurrence: occurrence,
			FeeItem:    saValue(t, sadomain.NewFeeItemReference, trunkFeeItem),
			Agreement:  saValue(t, sadomain.NewSupplierAgreementReference, trunkAgreementObject+"/v1"),
		},
		RequestedAt: sellBasisAt.Add(5 * time.Minute),
		RequestedBy: saValue(t, sadomain.NewRequesterReference, "SYN-SETTLEMENT-JOB"),
	})
	if err != nil {
		t.Fatalf("BUY 评价请求：%v", err)
	}
	return saports.EvaluationRequestRecord{
		Key:        saports.EvaluationRequestKey{TenantID: tenant, Request: request.ID()},
		Request:    request,
		RecordedAt: beatInstant(),
	}
}

// confirmFirstCharge 让首笔预估费用经「有效交付已终局」那一路确认，走的是生产路由表上的终局信封。
func confirmFirstCharge(
	t *testing.T, beat Beat, db *bentopg.DB, store *outbox.Store, tenant sadomain.TenantID, charge sadomain.CustomerChargeID,
) {
	t.Helper()
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		t.Fatalf("PS 终局册：%v", err)
	}
	record := effectiveDeliveryFinalRecord(t, effectiveDeliveryFinalKey(t, measurementRedeliveryParcel))
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		_, err := finals.Save(txCtx, record)
		return err
	})
	enqueueForBeat(t, db, store, "final-mc-1", sainbox.FinalOutcomeFormedEventType,
		`{"tenantId":"`+demoTenant+`","parcel":"`+measurementRedeliveryParcel+`","kind":"EFFECTIVE_DELIVERY","version":"SYN-OUTCOME-V1"}`)
	settleBeats(t, beat)
	charges, err := sapostgres.NewCustomerCharges(db)
	if err != nil {
		t.Fatalf("SA 客户费用库：%v", err)
	}
	if found, stage := chargeStage(t, charges, tenant, charge); !found || stage != sadomain.ChargeConfirmed {
		t.Fatalf("终局之后费用 found=%v 阶段=%s, want 已确认", found, stage)
	}
}

func chargeStage(t *testing.T, charges *sapostgres.CustomerCharges, tenant sadomain.TenantID, id sadomain.CustomerChargeID) (bool, sadomain.ChargeStage) {
	t.Helper()
	charge, found, err := charges.FindByID(t.Context(), tenant, id)
	if err != nil {
		t.Fatalf("读费用 %s：%v", id, err)
	}
	return found, charge.Stage()
}

// noticeMeasurementChange 经 NO 的真交接口把一次测量变化写进 Outbox：信封类型、载荷、信封 ID 与分区键都由生产方
// 自己写，本用例不拼。
func noticeMeasurementChange(t *testing.T, db *bentopg.DB, store *outbox.Store, source string) {
	t.Helper()
	tenant, err := nodomain.NewTenantID(demoTenant)
	if err != nil {
		t.Fatalf("NO 租户：%v", err)
	}
	notice, err := nopostgres.NewOutboxMeasurementChangeNotice(db, store, systemClock{})
	if err != nil {
		t.Fatalf("NO 测量变化交接：%v", err)
	}
	change := noports.MeasurementChanged{
		Tenant: tenant, SourceID: source, ObjectKind: nodomain.HandlingUnitObject,
		ObjectID: measurementRedeliveryUnit, OccurredAt: beatInstant(),
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		return notice.NoticeMeasurementChanged(txCtx, change)
	})
}

func outboxEventIDOfSubject(t *testing.T, db *bentopg.DB, eventType, subject string) string {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var id string
	if err := querier.QueryRow(t.Context(),
		`SELECT event_id FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1 AND subject = $2`, eventType, subject,
	).Scan(&id); err != nil {
		t.Fatalf("按主题 %s 读 %s 信封：%v", subject, eventType, err)
	}
	return id
}

func inboxConsumersOf(t *testing.T, db *bentopg.DB, eventID string) []string {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	rows, err := querier.Query(t.Context(),
		`SELECT consumer FROM `+migrate.SchemaBento+`.inbox WHERE event_id = $1 ORDER BY consumer`, eventID)
	if err != nil {
		t.Fatalf("读 inbox：%v", err)
	}
	defer rows.Close()
	var consumers []string
	for rows.Next() {
		var consumer string
		if err := rows.Scan(&consumer); err != nil {
			t.Fatalf("读 inbox：%v", err)
		}
		consumers = append(consumers, consumer)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读 inbox：%v", err)
	}
	return consumers
}

type requestLink struct {
	id         string
	generation int
	previous   string
	basis      string
}

// requestChain 按代次读演示租户在某张请求册上的全部请求。table 只传本文件里写死的两张册名。
func requestChain(t *testing.T, db *bentopg.DB, table string) []requestLink {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	rows, err := querier.Query(t.Context(),
		`SELECT request_id, generation, coalesce(previous_request_id, ''), coalesce(resume_basis, '')
		   FROM `+pgx.Identifier{"settlement_accounting", table}.Sanitize()+`
		  WHERE tenant_id = $1 ORDER BY generation, request_id`, demoTenant)
	if err != nil {
		t.Fatalf("读 %s：%v", table, err)
	}
	defer rows.Close()
	var chain []requestLink
	for rows.Next() {
		var link requestLink
		if err := rows.Scan(&link.id, &link.generation, &link.previous, &link.basis); err != nil {
			t.Fatalf("读 %s：%v", table, err)
		}
		chain = append(chain, link)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读 %s：%v", table, err)
	}
	return chain
}

func tableRows(t *testing.T, db *bentopg.DB, table string) int {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM `+pgx.Identifier{"settlement_accounting", table}.Sanitize()).Scan(&count); err != nil {
		t.Fatalf("数 %s：%v", table, err)
	}
	return count
}

// settlementFingerprint 给 settlement_accounting 的每张表取行数加整行文本的摘要，再加 outbox 里除实测变化信封以外
// 的信封标识集的摘要。表清单从目录里读，不在测试里列：新增的表自动进快照。
func settlementFingerprint(t *testing.T, db *bentopg.DB) map[string]string {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	tables, err := querier.Query(t.Context(),
		`SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = 'settlement_accounting' ORDER BY tablename`)
	if err != nil {
		t.Fatalf("读表清单：%v", err)
	}
	var names []string
	for tables.Next() {
		var name string
		if err := tables.Scan(&name); err != nil {
			tables.Close()
			t.Fatalf("读表清单：%v", err)
		}
		names = append(names, name)
	}
	tables.Close()
	if err := tables.Err(); err != nil {
		t.Fatalf("读表清单：%v", err)
	}
	if len(names) == 0 {
		t.Fatal("settlement_accounting 下读不到表：快照会空着假绿")
	}

	prints := map[string]string{}
	for _, name := range names {
		var (
			count  int64
			digest string
		)
		if err := querier.QueryRow(t.Context(),
			`SELECT count(*), coalesce(md5(string_agg(r::text, '|' ORDER BY r::text)), '')
			   FROM `+pgx.Identifier{"settlement_accounting", name}.Sanitize()+` AS r`,
		).Scan(&count, &digest); err != nil {
			t.Fatalf("取 %s 的快照：%v", name, err)
		}
		prints["settlement_accounting."+name] = fmt.Sprintf("%d 行 %s", count, digest)
	}
	var (
		count  int64
		digest string
	)
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*), coalesce(md5(string_agg(event_id, ',' ORDER BY event_id)), '')
		   FROM `+migrate.SchemaBento+`.outbox WHERE event_type <> $1`, nopostgres.ActualMeasurementChangedEventType,
	).Scan(&count, &digest); err != nil {
		t.Fatalf("取 outbox 的快照：%v", err)
	}
	prints["bento.outbox（不含实测变化信封）"] = fmt.Sprintf("%d 封 %s", count, digest)
	return prints
}

func changedFingerprints(before, after map[string]string) []string {
	var changed []string
	for key, was := range before {
		if now, present := after[key]; !present || now != was {
			changed = append(changed, fmt.Sprintf("%s：%s → %s", key, was, after[key]))
		}
	}
	for key, now := range after {
		if _, present := before[key]; !present {
			changed = append(changed, fmt.Sprintf("%s：新出现 %s", key, now))
		}
	}
	sort.Strings(changed)
	return changed
}

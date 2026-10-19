package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"

	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件钉路由表的 PP 评价已记录一条扇出之后的 BUY 那一路（sa-cc/01 消费门接上形成路；票 demo-chain-gaps/08、
// ADR-0200）：在生产依赖图上，SA 用真 BUY 请求册落一份评价请求；PP 用真评价库与真 Outbox 适配器落几份评价、各交一封
// 评价已记录信封，另有一封指着 PP 还没有的评价。一拍之后：
//   - 正：回指在册请求的 BUY·供应商成本评价形成恰一份供应商预期成本首版，版本标识由请求派生，合格来源引用取自请求，
//     金额整组出自评价。
//   - 入账、零写入：没有回指的 BUY 评价、SELL·客户费用评价（SELL 那一路照常入账）。
//   - 回指不在 BUY 请求册上：两侧矛盾，落 dispatch.publish_failed，BUY 那一路不记账、零写入。
//   - PP 还没有的评价：可见性未决，落 dispatch.consumer_undecided。
// 信封类型由 PP 的交接口自己写，路由表按 SA 消费方自写的常量认，两串相等由这一拍在真库上钉住。证据只记 S。

const (
	buyFormationConsumer  = "settlement-accounting/form-supplier-expected-cost"
	sellFormationConsumer = "settlement-accounting/form-estimated-customer-charge"
)

// syntheticPricingEvaluation 造一份 PP 评价：一张 SYN 卡（USD 价表 12.5，合计 HALF_UP 到 0.01——声明了取整策略，
// SA 读口才换写得出最小币单位）对一份 5 kg / Z1 的包裹输入评价。方向与目的由调用方给：同一张卡的形状换个方向就是
// SELL 评价，正是提供方对两个方向发同一种信封的那个事实。request 非空时评价回指它，形照计价「按评价请求形成评价」
// 带上回指那一步。构造走 PP 的测试专属夹具 pptest，值全部在这里显式给出（票 sa-cc/17）。
func syntheticPricingEvaluation(
	t *testing.T, id, request string, direction ppdomain.PricingDirection, purpose ppdomain.PricingPurpose,
) ppdomain.PricingEvaluation {
	t.Helper()
	plan := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-CARD-"+string(direction), "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, "SYN-TABLE", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, "SYN-WEIGHT", "v1"),
		Scope:                 "SYN-SCOPE-01",
		Direction:             direction,
		Purpose:               purpose,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "USD",
		Period: pptest.Period{
			StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndsAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		RateEntryID:         "entry",
		RateZone:            "Z1",
		MinimumKilograms:    "0",
		MaximumKilograms:    "10",
		RateAmount:          "12.5",
		WeightRounding:      ppdomain.RoundingNone,
		WeightStepKilograms: "1",
		AmountRounding: &pptest.AmountRoundingSpec{
			Mode:      ppdomain.RoundingHalfUp,
			Increment: "0.01",
			Points:    []ppdomain.AmountRoundingPoint{ppdomain.AmountRoundingTotal},
		},
	})
	input := pptest.Input(t, pptest.InputSpec{
		Tenant:     "tenant-a",
		Scope:      "SYN-SCOPE-01",
		PackageID:  "SYN-PKG-01",
		Zone:       "Z1",
		Kilograms:  "5",
		BusinessAt: time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	})
	evaluationRequest, err := ppdomain.NewEvaluationRequest(pptest.Value(t, ppdomain.NewEvaluationID, id), plan, input, ppdomain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("评价请求：%v", err)
	}
	if request != "" {
		evaluationRequest, err = evaluationRequest.WithRequestReference(pptest.Value(t, ppdomain.NewEvaluationRequestReference, request))
		if err != nil {
			t.Fatalf("回指：%v", err)
		}
	}
	evaluation := ppdomain.EvaluatePricing(evaluationRequest)
	if evaluation.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("夹具评价 %s 没完成：%s %#v", id, evaluation.Status(), evaluation.Issues())
	}
	return evaluation
}

// buyEvaluationRequestRecord 合成一份 tenant-a 的 BUY 评价请求：发生项 SYN-OCC-DCG08@v1、费用项目 SYN-FEE-LINEHAUL、
// 供应商协议 SYN-AGREEMENT-01。
func buyEvaluationRequestRecord(t *testing.T, id string) saports.EvaluationRequestRecord {
	t.Helper()
	occurrence, err := sadomain.NewTransportChargeOccurrence(
		saValue(t, sadomain.NewChargeOccurrenceID, "SYN-OCC-DCG08"),
		saValue(t, sadomain.NewOccurrenceReasonReference, "SYN-REASON-BOOKING"),
		saValue(t, sadomain.NewOccurrenceVersion, "v1"),
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
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
			FeeItem:    saValue(t, sadomain.NewFeeItemReference, "SYN-FEE-LINEHAUL"),
			Agreement:  saValue(t, sadomain.NewSupplierAgreementReference, "SYN-AGREEMENT-01"),
		},
		RequestedAt: time.Date(2026, 8, 7, 10, 5, 0, 0, time.UTC),
		RequestedBy: saValue(t, sadomain.NewRequesterReference, "SYN-SETTLEMENT-JOB"),
	})
	if err != nil {
		t.Fatalf("评价请求：%v", err)
	}
	return saports.EvaluationRequestRecord{
		Key:        saports.EvaluationRequestKey{TenantID: saValue(t, sadomain.NewTenantID, "tenant-a"), Request: request.ID()},
		Request:    request,
		RecordedAt: time.Date(2026, 8, 7, 10, 5, 1, 0, time.UTC),
	}
}

// Covers: 一封回指 BUY 请求的评价已记录信封经生产路由表形成恰一份供应商预期成本（正）；没有回指的 BUY 评价与 SELL
// 评价入账、零写入，SELL 那一路照常记账；回指不在册是 publish_failed；PP 还没有的评价是 consumer_undecided。
func TestABuyEvaluationRecordedFormsASupplierExpectedCostThroughTheRouteTable(t *testing.T) {
	observed := map[string]error{}
	beat, db, store := wiredBeat(t, dispatch.WithDeliveryFailureObserver(func(delivery eventing.Delivery, _ eventing.FailureCode, err error) {
		observed[string(delivery.Envelope.ID)] = err
	}))

	requests, err := sapostgres.NewEvaluationRequests(db)
	if err != nil {
		t.Fatalf("SA BUY 请求册：%v", err)
	}
	requested := buyEvaluationRequestRecord(t, "EVREQ-SYN-DCG08")
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		_, err := requests.Save(txCtx, requested)
		return err
	})

	evaluations, err := pppostgres.NewEvaluations(db)
	if err != nil {
		t.Fatalf("PP 评价库：%v", err)
	}
	handoff, err := pppostgres.NewOutboxEvaluationHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatalf("PP 评价交接：%v", err)
	}
	formed := syntheticPricingEvaluation(t, "SYN-EVAL-BUY-REQ", "EVREQ-SYN-DCG08", ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost)
	unreferenced := syntheticPricingEvaluation(t, "SYN-EVAL-BUY-01", "", ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost)
	sell := syntheticPricingEvaluation(t, "SYN-EVAL-SELL-01", "", ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge)
	stray := syntheticPricingEvaluation(t, "SYN-EVAL-BUY-STRAY", "EVREQ-SYN-404", ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost)
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		for _, evaluation := range []ppdomain.PricingEvaluation{formed, unreferenced, sell, stray} {
			if _, err := evaluations.Save(txCtx, evaluation); err != nil {
				return err
			}
			if err := handoff.HandOffEvaluation(txCtx, ppports.EvaluationHandoffIntent{Evaluation: evaluation}); err != nil {
				return err
			}
		}
		return nil
	})
	enqueueForBeat(t, db, store, "SYN-EVAL-MISSING", sainbox.BuyEvaluationRecordedEventType,
		`{"tenantId":"tenant-a","evaluationId":"SYN-EVAL-MISSING"}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	for _, id := range []string{"SYN-EVAL-BUY-REQ", "SYN-EVAL-BUY-01", "SYN-EVAL-SELL-01"} {
		if status := outboxStatus(t, db, id); status != "PUBLISHED" {
			t.Fatalf("%s status = %q, want PUBLISHED；失败码 = %q 原文 = %v", id, status, recordedFailureCode(t, db, id), observed[id])
		}
		for _, consumer := range []string{buyFormationConsumer, sellFormationConsumer} {
			if n := inboxRowsOf(t, db, consumer, id); n != 1 {
				t.Fatalf("%s 在 %s 的 inbox 行数 = %d, want 1（两路各记各的账）", id, consumer, n)
			}
		}
	}
	if published != 3 {
		t.Fatalf("published = %d, want 3", published)
	}
	if got := recordedFailureCode(t, db, "SYN-EVAL-BUY-STRAY"); got != "dispatch.publish_failed" {
		t.Fatalf("回指不在册：failure_code = %q, want dispatch.publish_failed；原文 = %v", got, observed["SYN-EVAL-BUY-STRAY"])
	}
	if !errors.Is(observed["SYN-EVAL-BUY-STRAY"], saapplication.ErrBuyEvaluationRequestNotRecorded) {
		t.Fatalf("回指不在册的原文 = %v, want ErrBuyEvaluationRequestNotRecorded", observed["SYN-EVAL-BUY-STRAY"])
	}
	if n := inboxRowsOf(t, db, buyFormationConsumer, "SYN-EVAL-BUY-STRAY"); n != 0 {
		t.Fatalf("矛盾必须回滚：BUY 那一路 inbox 行数 = %d, want 0", n)
	}
	if got := recordedFailureCode(t, db, "SYN-EVAL-MISSING"); got != "dispatch.consumer_undecided" {
		t.Fatalf("PP 还没有的评价：failure_code = %q, want dispatch.consumer_undecided", got)
	}

	costs, err := sapostgres.NewSupplierExpectedCosts(db)
	if err != nil {
		t.Fatalf("SA 预期成本库：%v", err)
	}
	cost, found, err := costs.LoadExpectedCost(t.Context(), saValue(t, sadomain.NewTenantID, "tenant-a"),
		saValue(t, sadomain.NewSupplierCostVersionID, "BUYSC-EVREQ-SYN-DCG08"))
	if err != nil || !found {
		t.Fatalf("按派生版本读预期成本：found=%v err=%v", found, err)
	}
	if cost.Occurrence().ID().String() != "SYN-OCC-DCG08" || cost.Occurrence().Version().String() != "v1" ||
		cost.FeeItem().String() != "SYN-FEE-LINEHAUL" || cost.Agreement().String() != "SYN-AGREEMENT-01" {
		t.Fatalf("来源引用没取自请求：发生项 %s@%s 项目 %s 协议 %s",
			cost.Occurrence().ID(), cost.Occurrence().Version(), cost.FeeItem(), cost.Agreement())
	}
	total, ok := formed.Total()
	if !ok || total.Currency().String() != "USD" || total.Amount().String() != "12.5" {
		t.Fatalf("夹具评价合计 = %v present=%v, want USD 12.5", total, ok)
	}
	settlementCurrency, settlementMinor := cost.SettlementAmount()
	originalCurrency, originalMinor := cost.OriginalAmount()
	if settlementCurrency.String() != "USD" || settlementMinor != 1250 || originalCurrency.String() != "USD" || originalMinor != 1250 ||
		cost.Evaluation().String() != "SYN-EVAL-BUY-REQ" || cost.RuleVersion().String() != "SYN-CARD-BUY@v1" {
		t.Fatalf("成本 = 结算 %s %d / 原币 %s %d / 评价 %s / 规则 %s, want 评价合计 USD 12.5 按 0.01 换写的 1250",
			settlementCurrency, settlementMinor, originalCurrency, originalMinor, cost.Evaluation(), cost.RuleVersion())
	}

	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var costRows int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM settlement_accounting.supplier_expected_cost WHERE tenant_id = $1`, "tenant-a",
	).Scan(&costRows); err != nil {
		t.Fatalf("数预期成本行：%v", err)
	}
	if costRows != 1 {
		t.Fatalf("supplier_expected_cost 有 %d 行, want 恰一份", costRows)
	}
	var inboxTotal int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.inbox WHERE consumer = $1`, buyFormationConsumer,
	).Scan(&inboxTotal); err != nil {
		t.Fatalf("数 inbox：%v", err)
	}
	if inboxTotal != 3 {
		t.Fatalf("BUY 那一路 inbox 共 %d 行, want 3（矛盾与未决都不记账）", inboxTotal)
	}
}

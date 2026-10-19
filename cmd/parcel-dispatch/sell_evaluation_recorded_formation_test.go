package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件钉路由表的 PP 评价已记录一条扇出之后的 SELL 那一路（ADR-0190 决定五、六；票 sell-customer-charge/04 完成判据
// 「真库装配」）：在生产依赖图上，SA 用真请求册落两份演示租户的 SELL 请求、用真映射册把 BASE_FREIGHT 与
// REMOTE_AREA_SURCHARGE 映到同一个合成费用项目；PP 用真评价库与真 Outbox 适配器落两份回指这两份请求的 SELL·客户费用
// 评价、交两封评价已记录信封。一拍之后：
//   - 正：费用行全有映射的那一份形成恰一笔预估客户费用，标识由请求派生，金额整组出自评价；请求改记`已形成`并带那笔
//     费用标识。
//   - 反：带一条没映射代码的那一份记`待判断`、不建费用，同样入账不重投。
//   - 两封在 BUY 那一路都入账（不是 BUY·供应商成本），两路各记各的 inbox 账。
// 信封类型由 PP 的交接口自己写，路由表按 SA 消费方自写的常量认，两串相等由这一拍在真库上钉住。证据只记 S。
//
// 评价用夹具真算，不经演示价卡：夹具卡自带金额取整声明，本用例因此不随演示租户的卡配置变。端到端由票 05 实走。

// sellChargeCard 造一张演示租户的 SELL·客户费用合成卡：CNY，Z1 一行 100、一条固定附加费 5（代码由调用方给），合计
// HALF_UP 到 0.01。
func sellChargeCard(t *testing.T, surchargeCode string) ppdomain.PricingPlanVersion {
	t.Helper()
	base := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-PLAN-SELL-"+surchargeCode, "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, "SYN-TABLE-SELL", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, "SYN-WEIGHT-SELL", "v1"),
		Scope:                 "SYN-SCOPE-01",
		Direction:             ppdomain.PricingDirectionSell,
		Purpose:               ppdomain.PricingPurposeCustomerCharge,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
		Period:                pptest.Period{StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		RateEntryID:           "SYN-RATE-Z1",
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            "100",
		WeightRounding:        ppdomain.RoundingNone,
		WeightStepKilograms:   "1",
		AmountRounding: &pptest.AmountRoundingSpec{
			Mode:      ppdomain.RoundingHalfUp,
			Increment: "0.01",
			Points:    []ppdomain.AmountRoundingPoint{ppdomain.AmountRoundingTotal},
		},
	})
	cny := pptest.Value(t, ppdomain.NewCurrency, "CNY")
	amount, err := ppdomain.NewMoney(pptest.Value(t, ppdomain.ParseDecimal, "5"), cny)
	if err != nil {
		t.Fatalf("附加费金额：%v", err)
	}
	surcharge, err := ppdomain.NewFixedChargeRule("SYN-RULE-"+surchargeCode,
		pptest.Value(t, ppdomain.NewChargeCode, surchargeCode), "SYN 附加费（合成演示值）", ppdomain.ChargeEffectAdd, amount, 1)
	if err != nil {
		t.Fatalf("附加费规则：%v", err)
	}
	plan, err := ppdomain.NewPricingPlanVersion(base.Reference(), base.Scope(), base.Direction(), base.Purpose(),
		base.BaseChargeCode(), base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(),
		[]ppdomain.FixedChargeRule{surcharge}, base.Structures())
	if err != nil {
		t.Fatalf("卡：%v", err)
	}
	return plan
}

// sellChargeEvaluation 真算一份回指 request 的评价：包裹 parcel、Z1、1.2 KG。
func sellChargeEvaluation(t *testing.T, id, request, parcel string, plan ppdomain.PricingPlanVersion) ppdomain.PricingEvaluation {
	t.Helper()
	input := pptest.Input(t, pptest.InputSpec{
		Tenant: demoTenant, Scope: "SYN-SCOPE-01", PackageID: parcel, Zone: "Z1", Kilograms: "1.2", BusinessAt: sellBasisAt,
	})
	evaluationRequest, err := ppdomain.NewEvaluationRequest(pptest.Value(t, ppdomain.NewEvaluationID, id), plan, input, ppdomain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("评价请求：%v", err)
	}
	evaluationRequest, err = evaluationRequest.WithRequestReference(pptest.Value(t, ppdomain.NewEvaluationRequestReference, request))
	if err != nil {
		t.Fatalf("回指：%v", err)
	}
	evaluation := ppdomain.EvaluatePricing(evaluationRequest)
	if evaluation.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("夹具评价 %s 状态 = %s %v", id, evaluation.Status(), evaluation.Issues())
	}
	return evaluation
}

func inboxRowsOf(t *testing.T, db *bentopg.DB, consumer, eventID string) int {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.inbox WHERE consumer = $1 AND event_id = $2`, consumer, eventID,
	).Scan(&count); err != nil {
		t.Fatalf("数 inbox：%v", err)
	}
	return count
}

// Covers: 一封评价已记录信封经生产路由表形成恰一笔预估客户费用，金额整组出自评价，请求改记`已形成`（正）；带一条没
// 映射代码的评价让请求记`待判断`、不建费用（反）；两封在 BUY 那一路都入账。
func TestASellEvaluationRecordedFormsAnEstimatedCustomerChargeThroughTheRouteTable(t *testing.T) {
	observed := map[string]error{}
	beat, db, store := wiredBeat(t, dispatch.WithDeliveryFailureObserver(func(delivery eventing.Delivery, _ eventing.FailureCode, err error) {
		observed[string(delivery.Envelope.ID)] = err
	}))
	tenant := saValue(t, sadomain.NewTenantID, demoTenant)

	requests, err := sapostgres.NewSellEvaluationRequests(db)
	if err != nil {
		t.Fatalf("SA SELL 请求册：%v", err)
	}
	mappings, err := sapostgres.NewFeeItemMappings(db)
	if err != nil {
		t.Fatalf("SA 费用项目映射册：%v", err)
	}
	mapped := sellRequestRecord(t, "SELLREQ-SYN-MAPPED", "SYN-PARCEL-01", "SYN-SCOPE-01")
	unmapped := sellRequestRecord(t, "SELLREQ-SYN-UNMAPPED", "SYN-PARCEL-02", "SYN-SCOPE-01")
	// 映射行在事务外先造好：造值失败会 Fatal，不能落在事务回调里（internal/architecture 事务闭包门禁的理由）。
	var freight []sadomain.FeeItemMapping
	for _, code := range []string{"BASE_FREIGHT", "REMOTE_AREA_SURCHARGE"} {
		mapping, err := sadomain.NewFeeItemMapping(
			saValue(t, sadomain.NewPricingChargeCode, code),
			saValue(t, sadomain.NewFeeItemReference, "SYN-FEE-ITEM-FREIGHT"),
			saValue(t, sadomain.NewFeeItemMappingBasisReference, "SYN-CLAUSE-FREIGHT-01"))
		if err != nil {
			t.Fatalf("映射行 %s：%v", code, err)
		}
		freight = append(freight, mapping)
	}
	registeredAt := beatInstant()
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		for _, record := range []saports.SellEvaluationRequestRecord{mapped, unmapped} {
			if _, err := requests.Save(txCtx, record); err != nil {
				return err
			}
		}
		for _, mapping := range freight {
			if _, err := mappings.SaveFeeItemMapping(txCtx, tenant, mapping, registeredAt); err != nil {
				return err
			}
		}
		return nil
	})

	evaluations, err := pppostgres.NewEvaluations(db)
	if err != nil {
		t.Fatalf("PP 评价库：%v", err)
	}
	handoff, err := pppostgres.NewOutboxEvaluationHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatalf("PP 评价交接：%v", err)
	}
	formed := sellChargeEvaluation(t, "SYN-EVAL-SELL-MAPPED", "SELLREQ-SYN-MAPPED", "SYN-PARCEL-01",
		sellChargeCard(t, "REMOTE_AREA_SURCHARGE"))
	pending := sellChargeEvaluation(t, "SYN-EVAL-SELL-UNMAPPED", "SELLREQ-SYN-UNMAPPED", "SYN-PARCEL-02",
		sellChargeCard(t, "SYN_UNMAPPED_SURCHARGE"))
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		for _, evaluation := range []ppdomain.PricingEvaluation{formed, pending} {
			if _, err := evaluations.Save(txCtx, evaluation); err != nil {
				return err
			}
			if err := handoff.HandOffEvaluation(txCtx, ppports.EvaluationHandoffIntent{Evaluation: evaluation}); err != nil {
				return err
			}
		}
		return nil
	})

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	for _, id := range []string{"SYN-EVAL-SELL-MAPPED", "SYN-EVAL-SELL-UNMAPPED"} {
		if status := outboxStatus(t, db, id); status != "PUBLISHED" {
			t.Fatalf("%s status = %q, want PUBLISHED；失败码 = %q 原文 = %v", id, status, recordedFailureCode(t, db, id), observed[id])
		}
		for _, consumer := range []string{"settlement-accounting/form-estimated-customer-charge", "settlement-accounting/form-supplier-expected-cost"} {
			if n := inboxRowsOf(t, db, consumer, id); n != 1 {
				t.Fatalf("%s 在 %s 的 inbox 行数 = %d, want 1（两路各记各的账）", id, consumer, n)
			}
		}
	}
	if published != 2 {
		t.Fatalf("published = %d, want 2", published)
	}

	charges, err := sapostgres.NewCustomerCharges(db)
	if err != nil {
		t.Fatalf("SA 客户费用库：%v", err)
	}
	total, ok := formed.Total()
	if !ok || total.Currency().String() != "CNY" || total.Amount().String() != "105" {
		t.Fatalf("夹具评价合计 = %v present=%v, want CNY 105（100 + 5）", total, ok)
	}
	charge, found, err := charges.FindByID(t.Context(), tenant, saValue(t, sadomain.NewCustomerChargeID, "SELLCC-SELLREQ-SYN-MAPPED"))
	if err != nil || !found {
		t.Fatalf("按派生标识读预估客户费用：found=%v err=%v", found, err)
	}
	if charge.Stage() != sadomain.ChargeEstimated || charge.FeeItem().String() != "SYN-FEE-ITEM-FREIGHT" || charge.Evaluation().String() != "SYN-EVAL-SELL-MAPPED" {
		t.Fatalf("费用 = 阶段 %s 项目 %s 评价 %s", charge.Stage(), charge.FeeItem(), charge.Evaluation())
	}
	settlementCurrency, settlementMinor := charge.SettlementAmount()
	originalCurrency, originalMinor := charge.OriginalAmount()
	if settlementCurrency.String() != "CNY" || settlementMinor != 10500 || originalCurrency.String() != "CNY" || originalMinor != 10500 {
		t.Fatalf("费用金额 = 结算 %s %d / 原币 %s %d, want 评价合计 CNY 105 按合计 0.01 换写的 10500", settlementCurrency, settlementMinor, originalCurrency, originalMinor)
	}
	if _, converted := charge.Conversion(); converted {
		t.Fatal("同币种的评价却带出了换算步骤")
	}

	formedRequest, found, err := requests.FindByID(t.Context(), mapped.Key)
	if err != nil || !found || formedRequest.State != sadomain.SellRequestFormed || formedRequest.FormedCharge.String() != "SELLCC-SELLREQ-SYN-MAPPED" {
		t.Fatalf("映射齐的请求 = %s / %q found=%v err=%v, want FORMED / SELLCC-SELLREQ-SYN-MAPPED",
			formedRequest.State, formedRequest.FormedCharge, found, err)
	}
	pendingRequest, found, err := requests.FindByID(t.Context(), unmapped.Key)
	if err != nil || !found || pendingRequest.State != sadomain.SellRequestPendingJudgment || pendingRequest.FormedCharge.String() != "" {
		t.Fatalf("有代码没映射的请求 = %s / %q found=%v err=%v, want PENDING_JUDGMENT 不带费用",
			pendingRequest.State, pendingRequest.FormedCharge, found, err)
	}

	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var chargeRows int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM settlement_accounting.customer_charge WHERE tenant_id = $1`, demoTenant,
	).Scan(&chargeRows); err != nil {
		t.Fatalf("数费用行：%v", err)
	}
	if chargeRows != 1 {
		t.Fatalf("customer_charge 有 %d 行, want 恰一笔（待判断不建占位费用）", chargeRows)
	}
}

package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	pricingports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: `/pricing-evaluation-replays` 的第二参是真编排（ADR-0124；票 wiring-baseline-remainder/06 件③）
// ——回放用例、真库评价册、真库价卡登记册与装配点的事务包装在真实 PostgreSQL 上装得起来：登一张合成
// 价卡、按它形成一份 `S` 评价入册，再经装配好的编排回放——回放评价以新引用入册、回指原评价、语义摘要
// 与原评价逐字相等；同回放引用第二次到达答「重复返原」——读得到首行即证首笔事务确实提交了；原方案版本
// 从未登记的评价答「原方案版本不在册」，不退回在用版本、不形成评价。输入是隔离合成，只记 `S`。
func TestTheWiredEvaluationReplayRecordsAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	cards, err := buildPriceCardRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配价卡登记编排：%v", err)
	}
	replay, err := buildEvaluationReplayOrchestration(db)
	if err != nil {
		t.Fatalf("装配回放编排：%v", err)
	}
	evaluations, err := pppostgres.NewEvaluations(db)
	if err != nil {
		t.Fatalf("构造评价册：%v", err)
	}

	registration := syntheticPriceCardRegistration(t)
	if outcome, err := cards.Handle(t.Context(), pricingapp.RegisterPriceCardCommand{Registration: registration}); err != nil || outcome != pricingapp.PriceCardRecorded {
		t.Fatalf("价卡首登 outcome = %s err = %v", outcome, err)
	}
	tenant := registration.Tenant()
	original := recordSyntheticEvaluation(t, db, evaluations, "SYN-PRC-REPLAY-ORIGINAL", registration.Plan())
	if original.Status() != pricingdomain.EvaluationCompleted {
		t.Fatalf("夹具没造出一份已完成的原评价：%s", original.Status())
	}

	command := pricingapp.ReplayPricingEvaluationCommand{
		Tenant:   tenant,
		Original: original.ID(),
		ReplayID: mustValue(t, pricingdomain.NewEvaluationID, "SYN-PRC-REPLAY-1"),
		Evidence: pricingdomain.EvidenceSynthetic,
	}
	recorded, err := replay.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("回放：%v", err)
	}
	if recorded.Outcome != pricingapp.ReplayRecorded || !recorded.HasEvaluation {
		t.Fatalf("回放 outcome = %s, 想要 RECORDED 且带评价", recorded.Outcome)
	}
	if recorded.Evaluation.Status() != pricingdomain.EvaluationCompleted || recorded.Evaluation.SemanticDigest() != original.SemanticDigest() {
		t.Fatalf("回放没重现原评价：status=%s digest 相等=%t", recorded.Evaluation.Status(), recorded.Evaluation.SemanticDigest() == original.SemanticDigest())
	}
	if replayOf, isReplay := recorded.Evaluation.ReplayOf(); !isReplay || replayOf != original.ID() {
		t.Fatalf("回放评价没回指原评价：%v", replayOf)
	}

	again, err := replay.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重复回放：%v", err)
	}
	if again.Outcome != pricingapp.ReplayExistingResult {
		t.Fatalf("重复回放 outcome = %s, 想要 EXISTING_RESULT——没读到首行说明首笔事务没提交", again.Outcome)
	}
	stored, found, err := evaluations.FindByID(t.Context(), command.ReplayID)
	if err != nil || !found || stored.SemanticDigest() != original.SemanticDigest() {
		t.Fatalf("回放评价没入册：found=%t err=%v", found, err)
	}
	untouched, _, err := evaluations.FindByID(t.Context(), original.ID())
	if err != nil || untouched.SemanticDigest() != original.SemanticDigest() {
		t.Fatalf("原评价被动过：err=%v", err)
	}

	// 原方案版本从未登进价卡登记册：那是「结构上重放不了」，不退回此刻适用的那一版（同方案 v1 正在册）。
	orphan := recordSyntheticEvaluation(t, db, evaluations, "SYN-PRC-REPLAY-ORPHAN", syntheticReplayPlan(t, "v9"))
	unregistered, err := replay.Handle(t.Context(), pricingapp.ReplayPricingEvaluationCommand{
		Tenant:   tenant,
		Original: orphan.ID(),
		ReplayID: mustValue(t, pricingdomain.NewEvaluationID, "SYN-PRC-REPLAY-2"),
		Evidence: pricingdomain.EvidenceSynthetic,
	})
	if err != nil || unregistered.Outcome != pricingapp.ReplayPlanVersionNotOnRegister || unregistered.HasEvaluation {
		t.Fatalf("原方案版本不在册：outcome=%s hasEvaluation=%t err=%v", unregistered.Outcome, unregistered.HasEvaluation, err)
	}
	if _, found, err := evaluations.FindByID(t.Context(), mustValue(t, pricingdomain.NewEvaluationID, "SYN-PRC-REPLAY-2")); err != nil || found {
		t.Fatalf("结构上重放不了却入册了：found=%t err=%v", found, err)
	}
}

// recordSyntheticEvaluation 按 plan 形成一份 `S` 评价并在事务内入册，交回入册的那份。
func recordSyntheticEvaluation(t *testing.T, db *bentopg.DB, evaluations *pppostgres.Evaluations, id string, plan pricingdomain.PricingPlanVersion) pricingdomain.PricingEvaluation {
	t.Helper()
	subject, err := pricingdomain.NewAcceptedPackageSubject(mustValue(t, pricingdomain.NewPackageID, "SYN-PRC-REPLAY-PACKAGE"))
	if err != nil {
		t.Fatalf("构造评价对象：%v", err)
	}
	input, err := pricingdomain.NewPricingInputSnapshot(
		mustValue(t, pricingdomain.NewTenantID, "SYN-TENANT-1"),
		mustValue(t, pricingdomain.NewPricingScopeID, "SYN-PRC-REG-SCOPE"),
		subject,
		"Z1",
		syntheticPricingWeight(t, "5"),
		nil,
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("构造输入快照：%v", err)
	}
	request, err := pricingdomain.NewEvaluationRequest(mustValue(t, pricingdomain.NewEvaluationID, id), plan, input, pricingdomain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("构造评价请求：%v", err)
	}
	evaluation := pricingdomain.EvaluatePricing(request)
	// 闭包只做 IO 并回 error：断言留在闭包外（t.Fatalf 走 runtime.Goexit，会让提交与回滚两条分支都被跳过）。
	var saved pricingports.EvaluationSaveOutcome
	err = db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, err := evaluations.Save(txCtx, evaluation)
		saved = outcome
		return err
	})
	if err != nil {
		t.Fatalf("评价入册：%v", err)
	}
	if saved != pricingports.EvaluationSaved {
		t.Fatalf("评价 %s 入册 outcome = %d", id, saved)
	}
	return evaluation
}

// syntheticReplayPlan 造与 syntheticPriceCardRegistration 同身份、不同版本号的合成方案——用来形成一份
// 「原方案版本从未登记」的评价。费率与结构照抄，只换版本，免得一份评价因为别的原因算不出来。
func syntheticReplayPlan(t *testing.T, version string) pricingdomain.PricingPlanVersion {
	t.Helper()
	currency := mustValue(t, pricingdomain.NewCurrency, "USD")
	amount, err := pricingdomain.NewMoneyFromString("10", currency)
	if err != nil {
		t.Fatalf("构造金额：%v", err)
	}
	entry, err := pricingdomain.NewRateEntry(
		mustValue(t, pricingdomain.NewRateEntryID, "SYN-PRC-REG-ENTRY-1"),
		"Z1", syntheticPricingWeight(t, "0"), syntheticPricingWeight(t, "10"), amount)
	if err != nil {
		t.Fatalf("构造费率段：%v", err)
	}
	period, err := pricingdomain.NewEffectivePeriod(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("构造适用期：%v", err)
	}
	table, err := pricingdomain.NewRateTableVersion(
		syntheticPricingReference(t, pricingdomain.ArtifactRateTable, "SYN-PRC-REG-TABLE", version),
		pricingdomain.RateTableFamilyWeightZone, currency, pricingdomain.WeightUnitKilogram,
		period, []pricingdomain.RateEntry{entry})
	if err != nil {
		t.Fatalf("构造价表：%v", err)
	}
	rounding, err := pricingdomain.NewWeightRoundingPolicy(pricingdomain.RoundingCeiling, syntheticPricingWeight(t, "0.5"))
	if err != nil {
		t.Fatalf("构造取整策略：%v", err)
	}
	weightPolicy, err := pricingdomain.NewPricingWeightPolicy(
		syntheticPricingReference(t, pricingdomain.ArtifactWeightPolicy, "SYN-PRC-REG-WEIGHT", version),
		pricingdomain.PricingWeightActualOnly, rounding, nil)
	if err != nil {
		t.Fatalf("构造计价重策略：%v", err)
	}
	plan, err := pricingdomain.NewPricingPlanVersion(
		syntheticPricingReference(t, pricingdomain.ArtifactPricingPlan, "SYN-PRC-REG-PLAN", version),
		mustValue(t, pricingdomain.NewPricingScopeID, "SYN-PRC-REG-SCOPE"),
		pricingdomain.PricingDirectionSell,
		pricingdomain.PricingPurposeCustomerCharge,
		mustValue(t, pricingdomain.NewChargeCode, "BASE_FREIGHT"),
		period, table, weightPolicy, nil, pricingdomain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatalf("构造价卡：%v", err)
	}
	return plan
}

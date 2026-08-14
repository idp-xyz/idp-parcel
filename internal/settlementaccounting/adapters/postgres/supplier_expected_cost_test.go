package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 16 证供应商预期成本的持久化行为：版本往返后金额与依据一字
// 不差、纠错版本带着回指与原因回来、跨币种的换算步骤随行、指错版本是`不存在`而不是
// 报错、幂等三维只许形成一次首版、无事务拒。

var (
	expectedCostOccurredAt = time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)
	expectedCostRecordedAt = time.Date(2026, 9, 10, 9, 30, 0, 0, time.UTC)
)

func TestAnExpectedCostVersionRoundTrips(t *testing.T) {
	repository, transactor := newExpectedCosts(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")

	cost := formedExpectedCost(t, "cost-v1", "occurrence-1")
	mustSaveExpectedCost(t, transactor, repository, tenant, cost)

	loaded, found, err := repository.LoadExpectedCost(ctx, tenant, cost.Version())
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	if !found {
		t.Fatal("刚登记的版本读不回来")
	}

	if loaded.Version() != cost.Version() ||
		loaded.Occurrence().ID() != cost.Occurrence().ID() ||
		loaded.FeeItem() != cost.FeeItem() ||
		loaded.RuleVersion() != cost.RuleVersion() ||
		loaded.Agreement() != cost.Agreement() ||
		loaded.Evaluation() != cost.Evaluation() {
		t.Fatalf("依据没有原样带回：%+v", loaded)
	}
	if currency, minor := loaded.SettlementAmount(); minor != 4200 || currency.String() != "USD" {
		t.Fatalf("结算金额 = %s %d，want USD 4200", currency, minor)
	}
	if !loaded.Occurrence().OccurredAt().Equal(expectedCostOccurredAt) {
		t.Fatalf("发生时间 = %s", loaded.Occurrence().OccurredAt())
	}
	if _, corrected := loaded.PriorVersion(); corrected {
		t.Fatal("首版读回来带着回指")
	}
}

// TestACorrectionVersionKeepsItsBackReference 证纠错换版本而不是改写：两版同时读得回来，
// 新版指回原版并带原因，原版一字未动。
func TestACorrectionVersionKeepsItsBackReference(t *testing.T) {
	repository, transactor := newExpectedCosts(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")

	first := formedExpectedCost(t, "cost-v1", "occurrence-1")
	corrected, err := first.AppendCorrection(
		saValue(t, domain.NewSupplierCostVersionID, "cost-v2"),
		saValue(t, domain.NewBuyEvaluationReference, "buy-eval-2"),
		3900,
		domain.ConversionStepReference{},
		saValue(t, domain.NewCostCorrectionReason, "RULE_CORRECTED"),
	)
	if err != nil {
		t.Fatalf("追加纠错：%v", err)
	}
	mustSaveExpectedCost(t, transactor, repository, tenant, first)
	mustSaveExpectedCost(t, transactor, repository, tenant, corrected)

	loaded, found, err := repository.LoadExpectedCost(ctx, tenant, corrected.Version())
	if err != nil || !found {
		t.Fatalf("读回纠错版本：found=%v err=%v", found, err)
	}
	prior, isCorrection := loaded.PriorVersion()
	if !isCorrection || prior != first.Version() {
		t.Fatalf("回指 = %s（isCorrection=%v），want %s", prior, isCorrection, first.Version())
	}
	reason, hasReason := loaded.CorrectionReason()
	if !hasReason || reason.String() != "RULE_CORRECTED" {
		t.Fatalf("纠错原因 = %s（hasReason=%v）", reason, hasReason)
	}
	if _, minor := loaded.SettlementAmount(); minor != 3900 {
		t.Fatalf("纠错后结算金额 = %d, want 3900", minor)
	}

	original, found, err := repository.LoadExpectedCost(ctx, tenant, first.Version())
	if err != nil || !found {
		t.Fatalf("读回原版：found=%v err=%v", found, err)
	}
	if _, minor := original.SettlementAmount(); minor != 4200 {
		t.Fatalf("原版被纠错改写了：结算金额 = %d", minor)
	}
}

// TestACrossCurrencyExpectedCostCarriesItsConversionStep 证跨币种那一列随行：换算依据
// 归 parcel-pricing 在评价内完成，丢了它，读回的成本就解释不了第二个金额从哪来。
func TestACrossCurrencyExpectedCostCarriesItsConversionStep(t *testing.T) {
	repository, transactor := newExpectedCosts(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")

	cost, err := domain.FormSupplierExpectedCost(domain.SupplierExpectedCostSpec{
		Version:            saValue(t, domain.NewSupplierCostVersionID, "cost-fx-1"),
		Occurrence:         chargeOccurrence(t, "occurrence-fx"),
		FeeItem:            saValue(t, domain.NewFeeItemReference, "LINEHAUL"),
		RuleVersion:        saValue(t, domain.NewPurchaseRuleVersionReference, "purchase-rule-1"),
		Agreement:          saValue(t, domain.NewSupplierAgreementReference, "agreement-1"),
		Evaluation:         saValue(t, domain.NewBuyEvaluationReference, "buy-eval-fx"),
		OriginalCurrency:   saValue(t, domain.NewCurrencyCode, "EUR"),
		OriginalMinor:      3000,
		SettlementCurrency: saValue(t, domain.NewCurrencyCode, "USD"),
		SettlementMinor:    3300,
		Conversion:         saValue(t, domain.NewConversionStepReference, "conversion-step-1"),
	})
	if err != nil {
		t.Fatalf("形成跨币种预期成本：%v", err)
	}
	mustSaveExpectedCost(t, transactor, repository, tenant, cost)

	loaded, found, err := repository.LoadExpectedCost(ctx, tenant, cost.Version())
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	conversion, crossCurrency := loaded.Conversion()
	if !crossCurrency || conversion.String() != "conversion-step-1" {
		t.Fatalf("换算步骤 = %s（crossCurrency=%v）", conversion, crossCurrency)
	}
	if currency, minor := loaded.OriginalAmount(); currency.String() != "EUR" || minor != 3000 {
		t.Fatalf("原币金额 = %s %d", currency, minor)
	}
}

// TestAnUnknownExpectedCostVersionIsNotFound 证两件事：没登记过的版本答`不存在`而不是
// 报错（指错版本是提交矛盾，不是等谁），以及另一个租户探到的与真不存在长得一样。
func TestAnUnknownExpectedCostVersionIsNotFound(t *testing.T) {
	repository, transactor := newExpectedCosts(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")

	cost := formedExpectedCost(t, "cost-v1", "occurrence-1")
	mustSaveExpectedCost(t, transactor, repository, tenant, cost)

	if _, found, err := repository.LoadExpectedCost(
		ctx, tenant, saValue(t, domain.NewSupplierCostVersionID, "cost-never-formed"),
	); err != nil || found {
		t.Fatalf("未登记版本：found=%v err=%v", found, err)
	}

	if _, found, err := repository.LoadExpectedCost(
		ctx, saTenant(t, "tenant-2"), cost.Version(),
	); err != nil || found {
		t.Fatalf("跨租户读到了：found=%v err=%v", found, err)
	}
}

// TestTheSameCostTripletFormsOnlyOneFirstVersion 证幂等三维落在库里：同一发生项、费用
// 项目与规则版本只许有一份首版，第二份换个版本号也进不来——答`已登记`而不是报错，
// 调用方据以读回赢家（ADR-0031）。
func TestTheSameCostTripletFormsOnlyOneFirstVersion(t *testing.T) {
	repository, transactor := newExpectedCosts(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")

	mustSaveExpectedCost(t, transactor, repository, tenant, formedExpectedCost(t, "cost-v1", "occurrence-1"))

	duplicate := formedExpectedCost(t, "cost-v9", "occurrence-1")
	var outcome adapter.ExpectedCostSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, tenant, duplicate, expectedCostRecordedAt)
		return err
	})
	if outcome != adapter.ExpectedCostAlreadyRecorded {
		t.Fatalf("outcome = %s, want ALREADY_RECORDED", outcome)
	}

	if _, found, err := repository.LoadExpectedCost(ctx, tenant, duplicate.Version()); err != nil || found {
		t.Fatalf("第二份首版落库了：found=%v err=%v", found, err)
	}
}

func TestSavingAnExpectedCostOutsideATransactionIsRefused(t *testing.T) {
	repository, _ := newExpectedCosts(t)

	if _, err := repository.Save(
		t.Context(), saTenant(t, "tenant-1"),
		formedExpectedCost(t, "cost-v1", "occurrence-1"),
		expectedCostRecordedAt,
	); err == nil {
		t.Fatal("事务外写入成功了")
	}
}

// ---- 夹具 ----

func newExpectedCosts(t *testing.T) (*adapter.SupplierExpectedCosts, bentoapp.Transactor) {
	t.Helper()

	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewSupplierExpectedCosts(db)
	if err != nil {
		t.Fatalf("构造预期成本仓储：%v", err)
	}
	return repository, db.Transactor()
}

func mustSaveExpectedCost(
	t *testing.T,
	transactor bentoapp.Transactor,
	repository *adapter.SupplierExpectedCosts,
	tenant domain.TenantID,
	cost domain.SupplierExpectedCost,
) {
	t.Helper()
	saWithin(t, transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := repository.Save(txCtx, tenant, cost, expectedCostRecordedAt)
		if err != nil {
			return err
		}
		if outcome != adapter.ExpectedCostSaved {
			t.Errorf("登记 %s：outcome = %s, want SAVED", cost.Version(), outcome)
		}
		return nil
	})
}

func chargeOccurrence(t *testing.T, id string) domain.TransportChargeOccurrence {
	t.Helper()
	occurrence, err := domain.NewTransportChargeOccurrence(
		saValue(t, domain.NewChargeOccurrenceID, id),
		saValue(t, domain.NewOccurrenceReasonReference, "ACTUAL_LEG"),
		saValue(t, domain.NewOccurrenceVersion, "occurrence-version-1"),
		expectedCostOccurredAt,
	)
	if err != nil {
		t.Fatalf("运输收费发生项：%v", err)
	}
	return occurrence
}

func formedExpectedCost(t *testing.T, version, occurrence string) domain.SupplierExpectedCost {
	t.Helper()
	cost, err := domain.FormSupplierExpectedCost(domain.SupplierExpectedCostSpec{
		Version:            saValue(t, domain.NewSupplierCostVersionID, version),
		Occurrence:         chargeOccurrence(t, occurrence),
		FeeItem:            saValue(t, domain.NewFeeItemReference, "LINEHAUL"),
		RuleVersion:        saValue(t, domain.NewPurchaseRuleVersionReference, "purchase-rule-1"),
		Agreement:          saValue(t, domain.NewSupplierAgreementReference, "agreement-1"),
		Evaluation:         saValue(t, domain.NewBuyEvaluationReference, "buy-eval-1"),
		OriginalCurrency:   saValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      4200,
		SettlementCurrency: saValue(t, domain.NewCurrencyCode, "USD"),
		SettlementMinor:    4200,
	})
	if err != nil {
		t.Fatalf("形成预期成本：%v", err)
	}
	return cost
}

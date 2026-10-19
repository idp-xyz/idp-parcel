package sainbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sapricing "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 16 证 BUY 那一路接上形成编排之后的消费门（票 demo-chain-gaps/08）：inbox 消费门、处理方、
// 形成编排外层与本体、BUY 请求册与预期成本库都是真的，只有提供方的评价读口是替身（PP 真评价库那一段由
// cmd/parcel-dispatch 的真库装配用例钉）。一封回指在册请求的信封形成恰一份预期成本，同一封重投与另一封指同一份评价
// 都不多形成；回指不在册时整笔回滚、inbox 不记账、零写入。

var formationRecordedAt = time.Date(2026, 9, 11, 9, 30, 0, 0, time.UTC)

type formationClock struct{}

func (formationClock) Now() time.Time { return formationRecordedAt }

// formationViewDouble 按评价标识交回一份采用快照，替提供方的评价库。
type formationViewDouble map[string]saports.BuyEvaluationAdoption

func (double formationViewDouble) LoadBuyEvaluation(
	_ context.Context, _ sadomain.TenantID, evaluation sadomain.BuyEvaluationReference,
) (saports.BuyEvaluationAdoption, bool, error) {
	adoption, found := double[evaluation.String()]
	return adoption, found, nil
}

func formationValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return value
}

type formationFixture struct {
	consumer *sainbox.BuyEvaluationRecordedConsumer
	db       *bentopg.DB
	costs    *sapostgres.SupplierExpectedCosts
}

// newFormationFixture 在真 BUY 请求册上登一份 EVREQ-SYN-1，评价读口替身把 SYN-EVAL-01 答成一份回指 request 的已完成
// 评价（USD 12.50），再把真消费门、处理方与两层形成编排接起来。
func newFormationFixture(t *testing.T, request string) *formationFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	requests, err := sapostgres.NewEvaluationRequests(db)
	if err != nil {
		t.Fatalf("BUY 请求册：%v", err)
	}
	costs, err := sapostgres.NewSupplierExpectedCosts(db)
	if err != nil {
		t.Fatalf("预期成本库：%v", err)
	}

	tenant := formationValue(t, sadomain.NewTenantID, "tenant-a")
	occurrence, err := sadomain.NewTransportChargeOccurrence(
		formationValue(t, sadomain.NewChargeOccurrenceID, "SYN-OCC-1"),
		formationValue(t, sadomain.NewOccurrenceReasonReference, "SYN-REASON-BOOKING"),
		formationValue(t, sadomain.NewOccurrenceVersion, "v1"),
		formationRecordedAt.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("发生项：%v", err)
	}
	submitted, err := sadomain.SubmitEvaluationRequest(sadomain.EvaluationRequestSpec{
		ID:      formationValue(t, sadomain.NewEvaluationRequestID, "EVREQ-SYN-1"),
		Scope:   formationValue(t, sadomain.NewPrimaryScopeReference, "SYN-SCOPE-1"),
		Purpose: sadomain.BuySupplierCost,
		Sources: sadomain.EligibleSourceReferences{
			Occurrence: occurrence,
			FeeItem:    formationValue(t, sadomain.NewFeeItemReference, "SYN-FEE-LINEHAUL"),
			Agreement:  formationValue(t, sadomain.NewSupplierAgreementReference, "SYN-AGREEMENT-1"),
		},
		RequestedAt: formationRecordedAt.Add(-30 * time.Minute),
		RequestedBy: formationValue(t, sadomain.NewRequesterReference, "SYN-SETTLEMENT-JOB"),
	})
	if err != nil {
		t.Fatalf("请求：%v", err)
	}
	record := saports.EvaluationRequestRecord{
		Key:     saports.EvaluationRequestKey{TenantID: tenant, Request: submitted.ID()},
		Request: submitted, RecordedAt: formationRecordedAt.Add(-30 * time.Minute),
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := requests.Save(txCtx, record)
		return err
	}); err != nil {
		t.Fatalf("登请求：%v", err)
	}

	usd := formationValue(t, sadomain.NewCurrencyCode, "USD")
	view := formationViewDouble{"SYN-EVAL-01": {
		Evaluation:         formationValue(t, sadomain.NewBuyEvaluationReference, "SYN-EVAL-01"),
		Outcome:            saports.BuyEvaluationCompleted,
		Request:            formationValue(t, sadomain.NewEvaluationRequestID, request),
		RuleVersion:        formationValue(t, sadomain.NewPurchaseRuleVersionReference, "SYN-BUY-CARD@v1"),
		OriginalCurrency:   usd,
		OriginalMinor:      1250,
		SettlementCurrency: usd,
		SettlementMinor:    1250,
	}}
	inner := saapplication.NewFormSupplierExpectedCostHandler(saapplication.FormSupplierExpectedCostDeps{
		Evaluations: view, Costs: costs, Registry: costs, Clock: formationClock{},
	})
	outer, err := saapplication.NewFormRequestedSupplierExpectedCostHandler(saapplication.FormRequestedSupplierExpectedCostDeps{
		Requests: requests, Costs: inner,
	})
	if err != nil {
		t.Fatalf("形成编排外层：%v", err)
	}
	gate, err := sapricing.NewFormOnBuyEvaluationRecordedAdapter(view, outer)
	if err != nil {
		t.Fatalf("处理方：%v", err)
	}
	consumer, err := sainbox.NewBuyEvaluationRecordedConsumer(db.Transactor(), store, gate)
	if err != nil {
		t.Fatalf("消费门：%v", err)
	}
	return &formationFixture{consumer: consumer, db: db, costs: costs}
}

func (fixture *formationFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	if err := querier.QueryRow(t.Context(), query, args...).Scan(&count); err != nil {
		t.Fatalf("计数：%v", err)
	}
	return count
}

func (fixture *formationFixture) costRows(t *testing.T) int {
	return fixture.count(t, `SELECT count(*) FROM settlement_accounting.supplier_expected_cost WHERE tenant_id = $1`, "tenant-a")
}

func (fixture *formationFixture) inboxRows(t *testing.T) int {
	return fixture.count(t, `SELECT count(*) FROM `+migrate.SchemaBento+`.inbox WHERE consumer = $1`,
		"settlement-accounting/form-supplier-expected-cost")
}

// Covers: 一封回指在册请求的信封经真消费门形成恰一份预期成本首版，版本标识 BUYSC-EVREQ-SYN-1、来源引用取自请求、
// 金额出自评价；同一封重投被 inbox 跳过，另一封指同一份评价按派生版本重放——两种重复都不多形成。
func TestARecordedBuyEvaluationFormsExactlyOneSupplierExpectedCostThroughTheGate(t *testing.T) {
	fixture := newFormationFixture(t, "EVREQ-SYN-1")

	for _, eventID := range []string{"SYN-EVAL-01", "SYN-EVAL-01", "SYN-EVAL-01-again"} {
		if err := fixture.consumer.Consume(t.Context(), evaluationEnvelope(t, eventID)); err != nil {
			t.Fatalf("投递 %s：%v", eventID, err)
		}
	}
	if rows := fixture.costRows(t); rows != 1 {
		t.Fatalf("supplier_expected_cost 有 %d 行, want 恰一份", rows)
	}
	if rows := fixture.inboxRows(t); rows != 2 {
		t.Fatalf("inbox 行数 = %d, want 2（两个事件标识各记一次）", rows)
	}
	cost, found, err := fixture.costs.LoadExpectedCost(t.Context(), formationValue(t, sadomain.NewTenantID, "tenant-a"),
		formationValue(t, sadomain.NewSupplierCostVersionID, "BUYSC-EVREQ-SYN-1"))
	if err != nil || !found {
		t.Fatalf("按派生版本读回：found=%v err=%v", found, err)
	}
	if cost.Occurrence().ID().String() != "SYN-OCC-1" || cost.FeeItem().String() != "SYN-FEE-LINEHAUL" ||
		cost.Agreement().String() != "SYN-AGREEMENT-1" || cost.Evaluation().String() != "SYN-EVAL-01" {
		t.Fatalf("成本来源 = 发生项 %s 项目 %s 协议 %s 评价 %s", cost.Occurrence().ID(), cost.FeeItem(), cost.Agreement(), cost.Evaluation())
	}
	if currency, minor := cost.SettlementAmount(); currency.String() != "USD" || minor != 1250 {
		t.Fatalf("金额 = %s %d, want 评价的 USD 1250", currency, minor)
	}
}

// Covers: 回指不在 BUY 请求册上是两侧矛盾：消费门报错、整笔回滚，inbox 不记账、零写入，重投还会响亮失败。
func TestABuyEvaluationReferringToAnUnrecordedRequestRollsBackThroughTheGate(t *testing.T) {
	fixture := newFormationFixture(t, "EVREQ-SYN-404")

	err := fixture.consumer.Consume(t.Context(), evaluationEnvelope(t, "SYN-EVAL-01"))
	if !errors.Is(err, saapplication.ErrBuyEvaluationRequestNotRecorded) {
		t.Fatalf("err = %v, want ErrBuyEvaluationRequestNotRecorded", err)
	}
	if rows := fixture.costRows(t); rows != 0 {
		t.Fatalf("矛盾时落了 %d 行预期成本", rows)
	}
	if rows := fixture.inboxRows(t); rows != 0 {
		t.Fatalf("矛盾时 inbox 记了 %d 行账", rows)
	}
}

package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉 ADR-0190 决定五的请求册改记口（票 sell-customer-charge/04）：只从`已提交`往外走，同一份结论再记是重放、另一份
// 结论不覆盖；`已形成`的费用标识要与费用行同一事务先落，不然外键整笔拒；写口无事务拒；另一个租户的键与不在册的
// 请求一样答错。夹具全是合成串。

type conclusionFixture struct {
	registry   *adapter.SellEvaluationRequests
	charges    *adapter.CustomerCharges
	transactor bentoapp.Transactor
	record     ports.SellEvaluationRequestRecord
}

func newConclusionFixture(t *testing.T) conclusionFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := adapter.NewSellEvaluationRequests(db)
	if err != nil {
		t.Fatalf("构造 SELL 评价请求登记册：%v", err)
	}
	charges, err := adapter.NewCustomerCharges(db)
	if err != nil {
		t.Fatalf("构造费用库：%v", err)
	}
	fixture := conclusionFixture{
		registry:   registry,
		charges:    charges,
		transactor: db.Transactor(),
		record: sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-1", "SYN-PARCEL-1",
			domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt),
	}
	if outcome := saveSellEvaluationRequest(t, fixture.transactor, registry, fixture.record); outcome != ports.EvaluationRequestSaved {
		t.Fatalf("首登结果 = %q", outcome)
	}
	return fixture
}

func conclusionOf(t *testing.T, state domain.SellEvaluationRequestState, charge string) domain.SellEvaluationRequestConclusion {
	t.Helper()
	var id domain.CustomerChargeID
	if charge != "" {
		id = saValue(t, domain.NewCustomerChargeID, charge)
	}
	conclusion, err := domain.NewSellEvaluationRequestConclusion(state, id)
	if err != nil {
		t.Fatalf("结论 %s/%q：%v", state, charge, err)
	}
	return conclusion
}

// conclude 在一笔事务里记结论；formCharge 非空时先在同一事务里落那笔预估费用。
func (fixture conclusionFixture) conclude(
	t *testing.T,
	key ports.SellEvaluationRequestKey,
	conclusion domain.SellEvaluationRequestConclusion,
	formCharge string,
) (ports.SellEvaluationRequestConclusionEffect, error) {
	t.Helper()
	var effect ports.SellEvaluationRequestConclusionEffect
	charge := domain.CustomerCharge{}
	if formCharge != "" {
		charge = estimatedCharge(t, formCharge)
	}
	err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if formCharge != "" {
			if _, err := fixture.charges.SaveEstimated(txCtx, key.TenantID, charge); err != nil {
				return err
			}
		}
		var recordErr error
		effect, recordErr = fixture.registry.RecordConclusion(txCtx, key, conclusion)
		return recordErr
	})
	return effect, err
}

func (fixture conclusionFixture) stored(t *testing.T) ports.SellEvaluationRequestRecord {
	t.Helper()
	stored, found, err := fixture.registry.FindByID(t.Context(), fixture.record.Key)
	if err != nil || !found {
		t.Fatalf("读回请求：found=%v err=%v", found, err)
	}
	return stored
}

// Covers: `已提交` → `待判断`写下；同一份再记是重放；换成`不可计价`不覆盖、册上仍是`待判断`。
func TestARequestIsConcludedOnceAndAReplayIsRecognised(t *testing.T) {
	fixture := newConclusionFixture(t)
	pending := conclusionOf(t, domain.SellRequestPendingJudgment, "")

	if effect, err := fixture.conclude(t, fixture.record.Key, pending, ""); err != nil || effect != ports.SellEvaluationRequestConcluded {
		t.Fatalf("首记 = %s err = %v，want CONCLUDED", effect, err)
	}
	if stored := fixture.stored(t); stored.State != domain.SellRequestPendingJudgment || stored.FormedCharge.String() != "" {
		t.Fatalf("读回 = %s / %q，want PENDING_JUDGMENT 不带费用", stored.State, stored.FormedCharge)
	}
	if effect, err := fixture.conclude(t, fixture.record.Key, pending, ""); err != nil || effect != ports.SellEvaluationRequestConclusionReplayed {
		t.Fatalf("重放 = %s err = %v，want REPLAYED", effect, err)
	}
	unratable := conclusionOf(t, domain.SellRequestUnratable, "")
	if effect, err := fixture.conclude(t, fixture.record.Key, unratable, ""); err != nil || effect != ports.SellEvaluationRequestConcludedOtherwise {
		t.Fatalf("另一份结论 = %s err = %v，want CONCLUDED_OTHERWISE", effect, err)
	}
	if stored := fixture.stored(t); stored.State != domain.SellRequestPendingJudgment {
		t.Fatalf("另一份结论覆盖了先到者：%s", stored.State)
	}
}

// Covers: `已形成`与费用行同一事务落下，读回带费用标识、按包裹读也找得到；同一份再记是重放；指向另一笔费用的`已形成`
// 不覆盖。
func TestAFormedConclusionCarriesTheChargeWrittenInTheSameTransaction(t *testing.T) {
	fixture := newConclusionFixture(t)
	formed := conclusionOf(t, domain.SellRequestFormed, "SELLCC-SELLREQ-SYN-1")

	if effect, err := fixture.conclude(t, fixture.record.Key, formed, "SELLCC-SELLREQ-SYN-1"); err != nil || effect != ports.SellEvaluationRequestConcluded {
		t.Fatalf("记已形成 = %s err = %v，want CONCLUDED", effect, err)
	}
	current, found, err := fixture.registry.FindCurrentByParcel(t.Context(), fixture.record.Key.TenantID, fixture.record.Request.Parcel())
	if err != nil || !found || current.State != domain.SellRequestFormed || current.FormedCharge.String() != "SELLCC-SELLREQ-SYN-1" {
		t.Fatalf("按包裹读回 = %s / %q found=%v err=%v", current.State, current.FormedCharge, found, err)
	}
	if effect, err := fixture.conclude(t, fixture.record.Key, formed, ""); err != nil || effect != ports.SellEvaluationRequestConclusionReplayed {
		t.Fatalf("重放 = %s err = %v，want REPLAYED", effect, err)
	}
	elsewhere := conclusionOf(t, domain.SellRequestFormed, "SYN-OTHER-CHARGE")
	if effect, err := fixture.conclude(t, fixture.record.Key, elsewhere, "SYN-OTHER-CHARGE"); err != nil || effect != ports.SellEvaluationRequestConcludedOtherwise {
		t.Fatalf("指向另一笔费用 = %s err = %v，want CONCLUDED_OTHERWISE", effect, err)
	}
	if stored := fixture.stored(t); stored.FormedCharge.String() != "SELLCC-SELLREQ-SYN-1" {
		t.Fatalf("另一笔费用覆盖了先到者：%q", stored.FormedCharge)
	}
}

// Covers: 费用行没在同一事务里先落时，`已形成`撞外键、整笔回滚，请求仍是`已提交`。
func TestAFormedConclusionWithoutItsChargeRowIsRefused(t *testing.T) {
	fixture := newConclusionFixture(t)
	formed := conclusionOf(t, domain.SellRequestFormed, "SELLCC-SELLREQ-SYN-1")

	if _, err := fixture.conclude(t, fixture.record.Key, formed, ""); err == nil {
		t.Fatal("费用不在册上，已形成却记下了")
	}
	if stored := fixture.stored(t); stored.State != domain.SellRequestSubmitted || stored.FormedCharge.String() != "" {
		t.Fatalf("被拒之后请求 = %s / %q，want 仍是 SUBMITTED", stored.State, stored.FormedCharge)
	}
}

// Covers: 写口在事务外拒；另一个租户的键与不在册的请求答错、不落任何东西；零值结论拒。
func TestRecordingAConclusionRefusesNoTransactionForeignKeysAndZeroConclusions(t *testing.T) {
	fixture := newConclusionFixture(t)
	pending := conclusionOf(t, domain.SellRequestPendingJudgment, "")

	if _, err := fixture.registry.RecordConclusion(t.Context(), fixture.record.Key, pending); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 RecordConclusion 应答 ErrTransactionRequired，实得：%v", err)
	}
	foreign := ports.SellEvaluationRequestKey{TenantID: saTenant(t, "tenant-b"), Request: fixture.record.Key.Request}
	if _, err := fixture.conclude(t, foreign, pending, ""); err == nil {
		t.Fatal("另一个租户按同一 ID 记下了结论")
	}
	missing := ports.SellEvaluationRequestKey{TenantID: fixture.record.Key.TenantID, Request: saValue(t, domain.NewSellEvaluationRequestID, "SELLREQ-SYN-NONE")}
	if _, err := fixture.conclude(t, missing, pending, ""); err == nil {
		t.Fatal("不在册的请求记下了结论")
	}
	if _, err := fixture.conclude(t, fixture.record.Key, domain.SellEvaluationRequestConclusion{}, ""); err == nil {
		t.Fatal("零值结论记下了")
	}
	if stored := fixture.stored(t); stored.State != domain.SellRequestSubmitted {
		t.Fatalf("被拒的几笔改了请求：%s", stored.State)
	}
}

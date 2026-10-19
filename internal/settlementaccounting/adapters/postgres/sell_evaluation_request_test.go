package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉 ADR-0190 决定一的登记册半边：SELL 评价请求真库往返（按铸造 ID、按自然键、按包裹读回的是同一份）；自然键
// （租户、包裹、计费时点）唯一约束在库上——另一个铸造 ID 撞同一件包裹同一时点答`已存在`、先到者不被覆盖、输家
// 的 ID 不落册；换时点是另一份；租户条件在 SQL 里；写口无事务拒。夹具全是合成串。

var (
	sellRequestBasisAt    = time.Date(2026, 9, 19, 8, 0, 0, 0, time.UTC)
	sellRequestRecordedAt = time.Date(2026, 9, 19, 9, 0, 0, 0, time.UTC)
)

func newSellEvaluationRequestRegistry(t *testing.T) (*adapter.SellEvaluationRequests, bentoapp.Transactor) {
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
	return registry, db.Transactor()
}

// sellEvaluationRequestRecord 按（租户、铸造 ID、包裹、计费时点）合成一份登记记录；账户、计价范围与来源事实固定，
// 让「同自然键不同 ID」与「换时点」两格只差一个参数。
func sellEvaluationRequestRecord(
	t *testing.T,
	tenant, id, parcel string,
	moment domain.ParcelBillingMoment,
	recordedAt time.Time,
) ports.SellEvaluationRequestRecord {
	t.Helper()
	request, err := domain.SubmitSellEvaluationRequest(domain.SellEvaluationRequestSpec{
		ID:                 saValue(t, domain.NewSellEvaluationRequestID, id),
		Parcel:             saValue(t, domain.NewParcelReference, parcel),
		Account:            saValue(t, domain.NewSettlementAccountID, "SYN-ACCT-1"),
		SettlementCurrency: saValue(t, domain.NewCurrencyCode, "XTS"),
		PricingScope:       saValue(t, domain.NewEstimatePricingScope, "SYN-SCOPE-SELL-1"),
		Moment:             moment,
		Source:             saValue(t, domain.NewSourceFactReference, "SYN-SOURCE-"+id),
		PricingBasisAt:     sellRequestBasisAt,
	})
	if err != nil {
		t.Fatalf("形成 SELL 评价请求：%v", err)
	}
	return ports.SellEvaluationRequestRecord{
		Key:        ports.SellEvaluationRequestKey{TenantID: saTenant(t, tenant), Request: request.ID()},
		Request:    request,
		RecordedAt: recordedAt,
		State:      domain.SellRequestSubmitted,
	}
}

func saveSellEvaluationRequest(
	t *testing.T,
	transactor bentoapp.Transactor,
	registry *adapter.SellEvaluationRequests,
	record ports.SellEvaluationRequestRecord,
) ports.EvaluationRequestSaveOutcome {
	t.Helper()
	var outcome ports.EvaluationRequestSaveOutcome
	saWithin(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = registry.Save(txCtx, record)
		return err
	})
	return outcome
}

func assertSellEvaluationRequestEquals(t *testing.T, got, want ports.SellEvaluationRequestRecord) {
	t.Helper()
	if got.Key != want.Key {
		t.Fatalf("键 = %+v，want %+v", got.Key, want.Key)
	}
	g, w := got.Request, want.Request
	if g.ID() != w.ID() || g.Parcel() != w.Parcel() || g.Account() != w.Account() || g.SettlementCurrency() != w.SettlementCurrency() ||
		g.PricingScope() != w.PricingScope() || g.Moment() != w.Moment() || g.Source() != w.Source() || !g.PricingBasisAt().Equal(w.PricingBasisAt()) {
		t.Fatalf("读回的请求变了形：%+v，want %+v", g, w)
	}
	if !got.RecordedAt.Equal(want.RecordedAt) {
		t.Fatalf("登记时刻 = %v，want %v", got.RecordedAt, want.RecordedAt)
	}
	if got.State != want.State || got.FormedCharge != want.FormedCharge {
		t.Fatalf("状态格 = %s / %q，want %s / %q", got.State, got.FormedCharge, want.State, want.FormedCharge)
	}
}

// Covers: ADR-0190 决定一「请求带：租户、包裹、结算账户、计价范围、计费时点、来源事实引用、计价基准时点」与登记时快照的
// 结算币种——各口读回的是同一份，一格不变形，状态格是`已提交`、不带费用标识；另一个租户探同一个 ID、同一件包裹都与真不存在长得一样。
func TestASellEvaluationRequestRoundTripsByIDNaturalKeyAndParcel(t *testing.T) {
	registry, transactor := newSellEvaluationRequestRegistry(t)
	record := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-1", "SYN-PARCEL-1",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)

	if outcome := saveSellEvaluationRequest(t, transactor, registry, record); outcome != ports.EvaluationRequestSaved {
		t.Fatalf("首登结果 = %q，want SAVED", outcome)
	}

	byID, found, err := registry.FindByID(t.Context(), record.Key)
	if err != nil || !found {
		t.Fatalf("按 ID 读回：found=%v err=%v", found, err)
	}
	assertSellEvaluationRequestEquals(t, byID, record)

	byNaturalKey, found, err := registry.FindByNaturalKey(t.Context(), record.Key.TenantID, record.Request.NaturalKey())
	if err != nil || !found {
		t.Fatalf("按自然键读回：found=%v err=%v", found, err)
	}
	assertSellEvaluationRequestEquals(t, byNaturalKey, record)

	byParcel, found, err := registry.FindCurrentByParcel(t.Context(), record.Key.TenantID, record.Request.Parcel())
	if err != nil || !found {
		t.Fatalf("按包裹读回：found=%v err=%v", found, err)
	}
	assertSellEvaluationRequestEquals(t, byParcel, record)

	otherTenant := saTenant(t, "tenant-b")
	if _, found, err := registry.FindByID(t.Context(), ports.SellEvaluationRequestKey{TenantID: otherTenant, Request: record.Key.Request}); err != nil || found {
		t.Fatalf("另一租户按同一 ID 读到了行：found=%v err=%v", found, err)
	}
	if _, found, err := registry.FindCurrentByParcel(t.Context(), otherTenant, record.Request.Parcel()); err != nil || found {
		t.Fatalf("另一租户按同一包裹读到了行：found=%v err=%v", found, err)
	}
	if _, found, err := registry.FindByNaturalKey(t.Context(), otherTenant, record.Request.NaturalKey()); err != nil || found {
		t.Fatalf("另一租户按同一自然键读到了行：found=%v err=%v", found, err)
	}
}

// Covers: ADR-0190 决定一「自然键只守幂等」与决定二「同一件包裹在所采时点只发起一次」——第二份带另一个铸造 ID、同一
// 件包裹同一时点：Save 撞的是自然键那条唯一约束而不是主键，答`已存在`；先到者原样在册、输家的 ID 不落册。另一个租户
// 的同一件包裹同一时点不撞。
func TestASecondSellRequestForTheSameParcelAndMomentAnswersAlreadyRequested(t *testing.T) {
	registry, transactor := newSellEvaluationRequestRegistry(t)
	winner := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-1", "SYN-PARCEL-1",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)
	loser := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-2", "SYN-PARCEL-1",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt.Add(time.Minute))

	if outcome := saveSellEvaluationRequest(t, transactor, registry, winner); outcome != ports.EvaluationRequestSaved {
		t.Fatalf("先到者结果 = %q", outcome)
	}
	if outcome := saveSellEvaluationRequest(t, transactor, registry, loser); outcome != ports.EvaluationRequestAlreadyRequested {
		t.Fatalf("同自然键第二份结果 = %q，want ALREADY_REQUESTED", outcome)
	}
	found, exists, err := registry.FindByNaturalKey(t.Context(), winner.Key.TenantID, winner.Request.NaturalKey())
	if err != nil || !exists || found.Key.Request != winner.Key.Request {
		t.Fatalf("自然键指向的不是先到者：%+v exists=%v err=%v", found.Key, exists, err)
	}
	if _, exists, err := registry.FindByID(t.Context(), loser.Key); err != nil || exists {
		t.Fatalf("输家的 ID 落了册：exists=%v err=%v", exists, err)
	}

	// 同一个 ID 再登一次撞的是主键，同样答`已存在`不覆盖。
	if outcome := saveSellEvaluationRequest(t, transactor, registry, winner); outcome != ports.EvaluationRequestAlreadyRequested {
		t.Fatalf("同 ID 重放结果 = %q，want ALREADY_REQUESTED", outcome)
	}

	elsewhere := sellEvaluationRequestRecord(t, "tenant-b", "SELLREQ-SYN-3", "SYN-PARCEL-1",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)
	if outcome := saveSellEvaluationRequest(t, transactor, registry, elsewhere); outcome != ports.EvaluationRequestSaved {
		t.Fatalf("另一租户同一件包裹同一时点结果 = %q，want SAVED——租户在自然键上", outcome)
	}
}

// Covers: 自然键带计费时点——同一件包裹换一种时点是另一份请求，两份并存；按包裹读答登记最晚的那份。
func TestAnotherMomentForTheSameParcelIsAnotherRequestAndTheLatestIsCurrent(t *testing.T) {
	registry, transactor := newSellEvaluationRequestRegistry(t)
	intake := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-1", "SYN-PARCEL-1",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)
	delivery := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-2", "SYN-PARCEL-1",
		domain.BillingMomentEffectiveDeliveryFinalized, sellRequestRecordedAt.Add(time.Hour))

	// 后登的那份先落库：「当前」看登记时刻，不看插入次序。
	for _, record := range []ports.SellEvaluationRequestRecord{delivery, intake} {
		if outcome := saveSellEvaluationRequest(t, transactor, registry, record); outcome != ports.EvaluationRequestSaved {
			t.Fatalf("%s 结果 = %q，want SAVED", record.Key.Request, outcome)
		}
	}
	current, found, err := registry.FindCurrentByParcel(t.Context(), intake.Key.TenantID, intake.Request.Parcel())
	if err != nil || !found {
		t.Fatalf("按包裹读回：found=%v err=%v", found, err)
	}
	assertSellEvaluationRequestEquals(t, current, delivery)

	if _, found, err := registry.FindCurrentByParcel(t.Context(), intake.Key.TenantID,
		saValue(t, domain.NewParcelReference, "SYN-PARCEL-9")); err != nil || found {
		t.Fatalf("没登过的包裹 found=%v err=%v，想要答 false", found, err)
	}
}

// 登记面写入必须在事务里：请求行与信封同事务是决定一的前提，写口在事务外被调是装配缺陷，按框架合同拒。
func TestSavingASellEvaluationRequestRequiresATransaction(t *testing.T) {
	registry, _ := newSellEvaluationRequestRegistry(t)
	record := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-1", "SYN-PARCEL-1",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)

	if _, err := registry.Save(t.Context(), record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
	if _, found, err := registry.FindByID(t.Context(), record.Key); err != nil || found {
		t.Fatalf("事务外的 Save 不该留下行：found=%v err=%v", found, err)
	}
}

// 键与对象说的不是同一件事（键指 SELLREQ-SYN-1、对象的 ID 是 SELLREQ-SYN-2）、键缺租户、或记录带着`已提交`之外的状态格
// → 拒存、不发 INSERT。前两格库上没有约束能拦，只能在写口拦；后一格库上的 CHECK 拦得住`已形成`缺费用，拦不住登记
// 直接写成`待判断`。
func TestSavingASellEvaluationRequestWhoseKeyDisagreesWithItsRequestIsRefused(t *testing.T) {
	registry, transactor := newSellEvaluationRequestRegistry(t)
	claimed := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-1", "SYN-PARCEL-1",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)
	mismatched := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-2", "SYN-PARCEL-1",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)
	mismatched.Key = claimed.Key
	blankTenant := claimed
	blankTenant.Key.TenantID = domain.TenantID{}

	formed := claimed
	formed.State = domain.SellRequestFormed
	formed.FormedCharge = saValue(t, domain.NewCustomerChargeID, "SYN-CHARGE-1")
	unset := claimed
	unset.State = domain.SellEvaluationRequestStateInvalid
	for name, record := range map[string]ports.SellEvaluationRequestRecord{
		"键与对象不符": mismatched, "键缺租户": blankTenant, "登记冒充已形成": formed, "状态格没给": unset,
	} {
		err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
			_, saveErr := registry.Save(txCtx, record)
			return saveErr
		})
		if err == nil {
			t.Fatalf("%s：Save 没拒", name)
		}
	}
	objectKey := ports.SellEvaluationRequestKey{TenantID: claimed.Key.TenantID, Request: mismatched.Request.ID()}
	for _, key := range []ports.SellEvaluationRequestKey{claimed.Key, objectKey} {
		if _, found, err := registry.FindByID(t.Context(), key); err != nil || found {
			t.Fatalf("被拒的 Save 留下了行 %+v：found=%v err=%v", key, found, err)
		}
	}
}

// Covers: 状态格（ADR-0190 决定五）——形成那一半改记之后读口原样交回；`已形成`带的预估客户费用必须真在费用册上，且只有
// `已形成`带费用标识，两条都由库上的约束守。按包裹读就能找到当前有效的预估客户费用。这里用 SQL 直接改记，模拟票 04
// 的写口，只证列与约束的形。
func TestTheRequestStateCellCarriesTheFormedChargeOnlyWhenFormed(t *testing.T) {
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
	transactor := db.Transactor()
	record := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-1", "SYN-PARCEL-1",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)
	if outcome := saveSellEvaluationRequest(t, transactor, registry, record); outcome != ports.EvaluationRequestSaved {
		t.Fatalf("首登结果 = %q", outcome)
	}

	mark := func(state string, charge any) error {
		_, err := pool.Exec(t.Context(),
			`UPDATE settlement_accounting.sell_evaluation_request SET state = $1, customer_charge_id = $2
			  WHERE tenant_id = 'tenant-a' AND request_id = 'SELLREQ-SYN-1'`, state, charge)
		return err
	}
	if err := mark("FORMED", nil); err == nil {
		t.Fatal("已形成却不带费用标识，库收下了")
	}
	if err := mark("PENDING_JUDGMENT", "charge-estimated"); err == nil {
		t.Fatal("待判断却带了费用标识，库收下了")
	}
	if err := mark("FORMED", "charge-estimated"); err == nil {
		t.Fatal("已形成指向一笔不在费用册上的费用，库收下了")
	}
	if err := mark("NOT_FORMED", nil); err == nil {
		t.Fatal("词表外的状态，库收下了")
	}

	if err := mark("PENDING_JUDGMENT", nil); err != nil {
		t.Fatalf("改记待判断：%v", err)
	}
	pending, found, err := registry.FindByID(t.Context(), record.Key)
	if err != nil || !found || pending.State != domain.SellRequestPendingJudgment || pending.FormedCharge.String() != "" {
		t.Fatalf("待判断读回 = %s / %q found=%v err=%v", pending.State, pending.FormedCharge, found, err)
	}

	saWithin(t, transactor, t.Context(), func(txCtx context.Context) error {
		_, err := charges.SaveEstimated(txCtx, saTenant(t, "tenant-a"), estimatedCharge(t, "charge-estimated"))
		return err
	})
	if err := mark("FORMED", "charge-estimated"); err != nil {
		t.Fatalf("改记已形成：%v", err)
	}
	current, found, err := registry.FindCurrentByParcel(t.Context(), record.Key.TenantID, record.Request.Parcel())
	if err != nil || !found {
		t.Fatalf("按包裹读回：found=%v err=%v", found, err)
	}
	if current.State != domain.SellRequestFormed || current.FormedCharge.String() != "charge-estimated" {
		t.Fatalf("按包裹读回的状态格 = %s / %q，want FORMED / charge-estimated", current.State, current.FormedCharge)
	}
}

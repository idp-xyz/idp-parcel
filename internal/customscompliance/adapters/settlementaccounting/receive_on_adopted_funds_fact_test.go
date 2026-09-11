package settlementaccounting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	ccinbox "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/settlementaccounting"
	ccapplication "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var fundsOccurredAt = time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)

// fundsFactRegisterDouble 是 CC 入向登记册的内存替身（按引用幂等，同引用重登答已登记）。
type fundsFactRegisterDouble struct {
	rows map[string]ccports.ExternalFundsFactRegistration
	err  error
}

func newFundsFactRegister() *fundsFactRegisterDouble {
	return &fundsFactRegisterDouble{rows: map[string]ccports.ExternalFundsFactRegistration{}}
}

func registerKey(tenant ccdomain.TenantID, fact ccdomain.ExternalFundsFactReference) string {
	return tenant.String() + "|" + fact.String()
}

func (double *fundsFactRegisterDouble) RegisterFundsFact(
	_ context.Context, tenant ccdomain.TenantID, registration ccports.ExternalFundsFactRegistration,
) (ccports.CaseConfigurationSaveOutcome, error) {
	if double.err != nil {
		return ccports.CaseConfigurationSaveOutcomeInvalid, double.err
	}
	key := registerKey(tenant, registration.Fact)
	if _, exists := double.rows[key]; exists {
		return ccports.CaseConfigurationAlreadyRegistered, nil
	}
	double.rows[key] = registration
	return ccports.CaseConfigurationRegistered, nil
}

func (double *fundsFactRegisterDouble) LoadFundsFact(
	_ context.Context, tenant ccdomain.TenantID, fact ccdomain.ExternalFundsFactReference,
) (ccports.ExternalFundsFactRegistration, bool, error) {
	if double.err != nil {
		return ccports.ExternalFundsFactRegistration{}, false, double.err
	}
	row, found := double.rows[registerKey(tenant, fact)]
	return row, found, nil
}

type dutyClock struct{ at time.Time }

func (clock dutyClock) Now() time.Time { return clock.at }

// unreachedDutyStores 补齐编排构造期要求的另两口（协作事项库、核对库）。本适配器只走 ReceiveFundsFact，
// 这两口不该被碰到；碰到即测试失败——替身守的是票面红线「消费者不关联、不核对」，不是给它们内存实现。
type unreachedDutyStores struct{ t *testing.T }

func (double unreachedDutyStores) FindCollaboration(
	context.Context, ccdomain.TenantID, ccdomain.DecisionScopeReference, ccdomain.AssessedDutyReference,
) (ccdomain.DutyPaymentCollaboration, bool, error) {
	double.t.Helper()
	double.t.Fatal("消费适配器不该读协作事项")
	return ccdomain.DutyPaymentCollaboration{}, false, nil
}

func (double unreachedDutyStores) SaveCollaboration(
	context.Context, ccdomain.TenantID, ccdomain.DutyPaymentCollaboration,
) (ccports.CaseConfigurationSaveOutcome, error) {
	double.t.Helper()
	double.t.Fatal("消费适配器不该形成协作事项")
	return ccports.CaseConfigurationSaveOutcomeInvalid, nil
}

func (double unreachedDutyStores) FindVerification(
	context.Context, ccports.DutyVerificationKey,
) (ccports.DutyVerificationRecord, bool, error) {
	double.t.Helper()
	double.t.Fatal("消费适配器不该读核对")
	return ccports.DutyVerificationRecord{}, false, nil
}

func (double unreachedDutyStores) SaveVerification(
	context.Context, ccports.DutyVerificationRecord,
) (ccports.CaseConfigurationSaveOutcome, error) {
	double.t.Helper()
	double.t.Fatal("消费适配器不该形成核对")
	return ccports.CaseConfigurationSaveOutcomeInvalid, nil
}

// adoptedSourceDouble 是本上下文读提供方那口的替身：按（租户|事实|版本）给内容。
type adoptedSourceDouble struct {
	facts map[string]ccports.AdoptedFundsFact
	err   error
}

func (double *adoptedSourceDouble) LoadAdoptedFundsFact(
	_ context.Context, tenant ccdomain.TenantID, fact ccdomain.ExternalFundsFactReference, version string,
) (ccports.AdoptedFundsFact, bool, error) {
	if double.err != nil {
		return ccports.AdoptedFundsFact{}, false, double.err
	}
	content, found := double.facts[tenant.String()+"|"+fact.String()+"|"+version]
	return content, found, nil
}

type fixture struct {
	source   *adoptedSourceDouble
	register *fundsFactRegisterDouble
	handler  *adapter.ReceiveOnAdoptedFundsFactAdapter
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	source := &adoptedSourceDouble{facts: map[string]ccports.AdoptedFundsFact{}}
	register := newFundsFactRegister()
	receiver, err := ccapplication.NewDutyPaymentReconciliationHandler(ccapplication.DutyPaymentReconciliationDeps{
		Collaborations: unreachedDutyStores{t: t},
		Funds:          register,
		Verifications:  unreachedDutyStores{t: t},
		Clock:          dutyClock{at: fundsOccurredAt.Add(time.Hour)},
	})
	if err != nil {
		t.Fatalf("构造编排：%v", err)
	}
	handler, err := adapter.NewReceiveOnAdoptedFundsFactAdapter(source, receiver)
	if err != nil {
		t.Fatalf("构造处理方：%v", err)
	}
	return &fixture{source: source, register: register, handler: handler}
}

func adoptedContent(payer string) ccports.AdoptedFundsFact {
	return ccports.AdoptedFundsFact{
		Source:      "source-bank-feed-1",
		Payer:       payer,
		Currency:    "USD",
		AmountMinor: 8000,
		OccurredAt:  fundsOccurredAt,
	}
}

func adoptedEnvelopeRef(version string) ccinbox.AdoptedExternalFundsFact {
	return ccinbox.AdoptedExternalFundsFact{TenantID: "tenant-a", Fact: "bank-fact-1", Version: version}
}

// Covers: sa-cc/03 完成判据 2「一封 → 一条登记；重投 → 已存在；换内容 → 内容冲突」——处理方按信封
// 引用回查提供方、译成入向登记交 ReceiveFundsFact；三种编排答案都入账（交回 nil），不重投。
// 消费者不关联不核对：登记里只有引用 + 核对所需维度，来源仍是提供方（票面红线）。
func TestAnAdoptedFundsFactIsRegisteredOnceAndReplayOrConflictStillSettles(t *testing.T) {
	fixture := newFixture(t)
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v1"] = adoptedContent("payer-customer-7")
	ref := adoptedEnvelopeRef("bank-fact/v1")

	if err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), ref); err != nil {
		t.Fatalf("首封：%v", err)
	}
	row, found := fixture.register.rows["tenant-a|bank-fact-1"]
	if !found {
		t.Fatal("一封信封该落一条登记")
	}
	want := ccports.ExternalFundsFactRegistration{
		Fact:        row.Fact,
		Source:      "source-bank-feed-1",
		Payer:       "payer-customer-7",
		Currency:    "USD",
		AmountMinor: 8000,
		OccurredAt:  fundsOccurredAt,
	}
	if row.Fact.String() != "bank-fact-1" || row != want {
		t.Fatalf("登记 = %+v, want %+v", row, want)
	}

	if err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), ref); err != nil {
		t.Fatalf("重投同一封（编排答已存在）也该入账：%v", err)
	}
	if len(fixture.register.rows) != 1 {
		t.Fatalf("登记条数 = %d, want 1", len(fixture.register.rows))
	}

	// 同引用、提供方那一版换了内容（模拟更正版本 v2 同引用到达）：编排答内容冲突，消费者照单入账。
	changed := adoptedContent("payer-customer-7")
	changed.AmountMinor = 9000
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v2"] = changed
	if err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v2")); err != nil {
		t.Fatalf("内容冲突是编排的答案，消费者不重投：%v", err)
	}
	if got := fixture.register.rows["tenant-a|bank-fact-1"].AmountMinor; got != 8000 {
		t.Fatalf("冲突不得顶替已登记内容，实得金额 %d", got)
	}
}

// Covers: 裁决「取信封所指的那一版，不取 latest」的消费侧——提供方还没有那一版就是可见性滞后，
// 交回 ErrAdoptedFactNotVisible 让消费门回滚重投；不落毒丸，也不拿别的版本顶替。
func TestAnInvisibleVersionIsAContinuationNotAPoisonEnvelope(t *testing.T) {
	fixture := newFixture(t)
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v1"] = adoptedContent("payer-customer-7")

	err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v2"))
	if !errors.Is(err, adapter.ErrAdoptedFactNotVisible) {
		t.Fatalf("err = %v, want ErrAdoptedFactNotVisible", err)
	}
	if len(fixture.register.rows) != 0 {
		t.Fatal("看不见的版本不得登记别的版本的内容")
	}

	fixture.source.err = errors.New("read replica down")
	err = fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v1"))
	if !errors.Is(err, adapter.ErrAdoptedFactNotVisible) {
		t.Fatalf("读口出错 err = %v, want ErrAdoptedFactNotVisible（重投会改变结果）", err)
	}
}

// Covers: 裁决「付款人在 SA 可缺席而 CC 入向登记要非空」两侧有意不对称——提供方那一版没有付款人，
// 消费者如实交空，编排答 `未受理`；那是本上下文的诚实停点，照交付消费者对 REQUEST_NOT_ACCEPTED 的
// 处置入账不重投（重投不会长出付款人）。CC 侧要不要放宽归另一张票。
func TestAFactWithoutAPayerIsRefusedByTheOrchestrationAndSettles(t *testing.T) {
	fixture := newFixture(t)
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v1"] = adoptedContent("")

	if err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v1")); err != nil {
		t.Fatalf("未受理是编排的答案，消费者入账不重投：%v", err)
	}
	if len(fixture.register.rows) != 0 {
		t.Fatal("没有付款人的事实不得被登记")
	}
}

// Covers: 编排未决（登记册不可用）是等依赖，交回 ErrFundsFactReceiveUndecided 让消费门重投。
func TestAnUndecidedOrchestrationIsRetried(t *testing.T) {
	fixture := newFixture(t)
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v1"] = adoptedContent("payer-customer-7")
	fixture.register.err = errors.New("register unavailable")

	err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v1"))
	if !errors.Is(err, adapter.ErrFundsFactReceiveUndecided) {
		t.Fatalf("err = %v, want ErrFundsFactReceiveUndecided", err)
	}
}

func TestTheReceiveAdapterRefusesNilDependencies(t *testing.T) {
	if _, err := adapter.NewReceiveOnAdoptedFundsFactAdapter(nil, nil); err == nil {
		t.Fatal("nil 依赖被收下了")
	}
}

// adoptedViewDouble 是提供方只读视图的替身，给 SettlementAdoptedFundsFactSource 的翻译用。
type adoptedViewDouble struct {
	facts map[string]sadomain.ExternalFundsFact
}

func (double *adoptedViewDouble) LoadAdoptedFundsFact(
	_ context.Context, tenant sadomain.TenantID, fact sadomain.FundsFactReference, version sadomain.FundsFactVersion,
) (sadomain.ExternalFundsFact, bool, error) {
	adopted, found := double.facts[tenant.String()+"|"+fact.String()+"|"+version.String()]
	return adopted, found, nil
}

func saAdoptedFact(t *testing.T, payer string) sadomain.ExternalFundsFact {
	t.Helper()
	spec := sadomain.ExternalFundsFactSpec{
		Kind:        sadomain.FundsReceiptConfirmed,
		AmountMinor: 8000,
		OccurredAt:  fundsOccurredAt,
	}
	var err error
	if spec.Fact, err = sadomain.NewFundsFactReference("bank-fact-1"); err != nil {
		t.Fatal(err)
	}
	if spec.Source, err = sadomain.NewFundsSourceRegistrationReference("source-bank-feed-1"); err != nil {
		t.Fatal(err)
	}
	if spec.Currency, err = sadomain.NewCurrencyCode("USD"); err != nil {
		t.Fatal(err)
	}
	if spec.Version, err = sadomain.NewFundsFactVersion("bank-fact/v1"); err != nil {
		t.Fatal(err)
	}
	if payer != "" {
		if spec.Payer, err = sadomain.NewFundsPayerReference(payer); err != nil {
			t.Fatal(err)
		}
	}
	fact, err := sadomain.AdoptExternalFundsFact(spec)
	if err != nil {
		t.Fatalf("构造提供方事实：%v", err)
	}
	return fact
}

// Covers: 做法 2「读 SA 用消费侧适配器」——翻译只在这里：提供方事实本体译成本上下文最少要读的几维；
// 付款人在提供方显式缺席就译成空；提供方答没有原样交回 false；空白版本构造不出提供方的键。
func TestTheSettlementSourceTranslatesTheProvidersFactIntoOurDimensions(t *testing.T) {
	view := &adoptedViewDouble{facts: map[string]sadomain.ExternalFundsFact{
		"tenant-a|bank-fact-1|bank-fact/v1": saAdoptedFact(t, "payer-customer-7"),
		"tenant-b|bank-fact-1|bank-fact/v1": saAdoptedFact(t, ""),
	}}
	source, err := adapter.NewSettlementAdoptedFundsFactSource(view)
	if err != nil {
		t.Fatalf("构造消费侧读口：%v", err)
	}
	fact, err := ccdomain.NewExternalFundsFactReference("bank-fact-1")
	if err != nil {
		t.Fatal(err)
	}
	tenantA, _ := ccdomain.NewTenantID("tenant-a")
	tenantB, _ := ccdomain.NewTenantID("tenant-b")

	got, found, err := source.LoadAdoptedFundsFact(context.Background(), tenantA, fact, "bank-fact/v1")
	if err != nil || !found {
		t.Fatalf("found = %v err = %v", found, err)
	}
	if got != adoptedContent("payer-customer-7") {
		t.Fatalf("译出 = %+v, want %+v", got, adoptedContent("payer-customer-7"))
	}

	got, found, err = source.LoadAdoptedFundsFact(context.Background(), tenantB, fact, "bank-fact/v1")
	if err != nil || !found || got.Payer != "" {
		t.Fatalf("提供方显式缺席的付款人应译成空：found = %v err = %v payer = %q", found, err, got.Payer)
	}

	if _, found, err := source.LoadAdoptedFundsFact(context.Background(), tenantA, fact, "bank-fact/v9"); err != nil || found {
		t.Fatalf("提供方没有那一版：found = %v err = %v, want false", found, err)
	}
	if _, _, err := source.LoadAdoptedFundsFact(context.Background(), tenantA, fact, "  "); !errors.Is(err, adapter.ErrUntranslatableReference) {
		t.Fatalf("空白版本 err = %v, want ErrUntranslatableReference", err)
	}
	if _, err := adapter.NewSettlementAdoptedFundsFactSource(nil); err == nil {
		t.Fatal("nil 视图被收下了")
	}
}

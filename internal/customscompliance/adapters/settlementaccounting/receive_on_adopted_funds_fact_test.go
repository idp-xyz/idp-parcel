package settlementaccounting_test

import (
	"context"
	"errors"
	"strings"
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

// fundsFactRegisterDouble 是 CC 入向登记册的内存替身（按（引用 + 版本）幂等，同键重登答已登记；order 记接收
// 先后，替真库的 received_at）。
type fundsFactRegisterDouble struct {
	rows  map[string]ccports.ExternalFundsFactRegistration
	order []string
	err   error
}

func newFundsFactRegister() *fundsFactRegisterDouble {
	return &fundsFactRegisterDouble{rows: map[string]ccports.ExternalFundsFactRegistration{}}
}

func registerKey(tenant ccdomain.TenantID, fact ccdomain.ExternalFundsFactReference, version ccdomain.FundsFactVersion) string {
	return tenant.String() + "|" + fact.String() + "|" + version.String()
}

func (double *fundsFactRegisterDouble) RegisterFundsFact(
	_ context.Context, tenant ccdomain.TenantID, registration ccports.ExternalFundsFactRegistration,
) (ccports.CaseConfigurationSaveOutcome, error) {
	if double.err != nil {
		return ccports.CaseConfigurationSaveOutcomeInvalid, double.err
	}
	key := registerKey(tenant, registration.Fact, registration.Version)
	if _, exists := double.rows[key]; exists {
		return ccports.CaseConfigurationAlreadyRegistered, nil
	}
	double.rows[key] = registration
	double.order = append(double.order, key)
	return ccports.CaseConfigurationRegistered, nil
}

func (double *fundsFactRegisterDouble) ListFundsFactVersions(
	_ context.Context, tenant ccdomain.TenantID, fact ccdomain.ExternalFundsFactReference,
) ([]ccports.ExternalFundsFactRegistration, error) {
	if double.err != nil {
		return nil, double.err
	}
	prefix := tenant.String() + "|" + fact.String() + "|"
	var versions []ccports.ExternalFundsFactRegistration
	for _, key := range double.order {
		if strings.HasPrefix(key, prefix) {
			versions = append(versions, double.rows[key])
		}
	}
	return versions, nil
}

func (double *fundsFactRegisterDouble) LoadFundsFact(
	ctx context.Context, tenant ccdomain.TenantID, fact ccdomain.ExternalFundsFactReference,
) (ccports.ExternalFundsFactRegistration, bool, error) {
	versions, err := double.ListFundsFactVersions(ctx, tenant, fact)
	if err != nil || len(versions) == 0 {
		return ccports.ExternalFundsFactRegistration{}, false, err
	}
	return versions[len(versions)-1], true, nil
}

type dutyClock struct{ at time.Time }

func (clock dutyClock) Now() time.Time { return clock.at }

// unreachedDutyStores 顶住编排构造期要求、而本适配器不走的那些口（协作事项库、核对库）。本适配器只走
// ReceiveFundsFact，这些口不该被碰到；碰到即测试失败——替身守的是票面红线「消费者不关联、不核对」，
// 不是给它们内存实现。
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

func (double unreachedDutyStores) HandOffDutyPaymentVerification(
	context.Context, ccports.DutyPaymentVerificationHandoffIntent,
) error {
	double.t.Helper()
	double.t.Fatal("消费适配器不该向结算交核对信封——它不核对，也就无物可交")
	return nil
}

func (double unreachedDutyStores) LoadPayerRequirement(
	context.Context, ccdomain.TenantID, ccdomain.CustomsProcedureReference,
) (ccdomain.PayerRequirement, bool, error) {
	double.t.Helper()
	double.t.Fatal("消费适配器不该读付款人规则——要不要付款人是核对时对着真实程序问的事，入向登记照单收下（票 sa-cc/12）")
	return ccdomain.PayerRequirementInvalid, false, nil
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
		PayerRules:     unreachedDutyStores{t: t},
		Handoff:        unreachedDutyStores{t: t},
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

// payerOf 把夹具里的一个串折成付款人一格：空串即「来源未提供」——夹具的写法沿用 SA 侧「空即缺席」的习惯，
// 译成本上下文的显式格是本文件在钉的事。
func payerOf(payer string) ccdomain.FundsPayer {
	if payer == "" {
		return ccdomain.FundsPayerNotProvided()
	}
	provided, err := ccdomain.ProvidedFundsPayer(payer)
	if err != nil {
		panic(err)
	}
	return provided
}

// versionOf 把夹具里的版本字面折成领域值；空串即零值（回指缺席 = 首版）。
func versionOf(version string) ccdomain.FundsFactVersion {
	if version == "" {
		return ccdomain.FundsFactVersion{}
	}
	value, err := ccdomain.NewFundsFactVersion(version)
	if err != nil {
		panic(err)
	}
	return value
}

// adoptedContent 是提供方那一版的内容：版本与回指前版随事实本体带出（票 sa-cc/13 做法 2）。
func adoptedContent(version, corrects, payer string) ccports.AdoptedFundsFact {
	return ccports.AdoptedFundsFact{
		Version:     versionOf(version),
		Corrects:    versionOf(corrects),
		Source:      "source-bank-feed-1",
		Payer:       payerOf(payer),
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
// 消费者不关联不核对：登记里只有引用 + 核对所需维度，来源仍是提供方（票面红线）。「内容冲突」自
// 票 sa-cc/13 起是同一版本两个来源各说一套；更正版本另见下一条用例。
func TestAnAdoptedFundsFactIsRegisteredOnceAndReplayOrConflictStillSettles(t *testing.T) {
	fixture := newFixture(t)
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v1"] = adoptedContent("bank-fact/v1", "", "payer-customer-7")
	ref := adoptedEnvelopeRef("bank-fact/v1")

	if err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), ref); err != nil {
		t.Fatalf("首封：%v", err)
	}
	row, found := fixture.register.rows["tenant-a|bank-fact-1|bank-fact/v1"]
	if !found {
		t.Fatal("一封信封该落一条登记")
	}
	want := ccports.ExternalFundsFactRegistration{
		Fact:        row.Fact,
		Version:     versionOf("bank-fact/v1"),
		Source:      "source-bank-feed-1",
		Payer:       payerOf("payer-customer-7"),
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

	// 同一版本、提供方换了内容（真冲突：同一版本两个来源各说一套）：编排答内容冲突，消费者照单入账，册上那版不动。
	changed := adoptedContent("bank-fact/v1", "", "payer-customer-7")
	changed.AmountMinor = 9000
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v1"] = changed
	if err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), ref); err != nil {
		t.Fatalf("内容冲突是编排的答案，消费者不重投：%v", err)
	}
	if got := fixture.register.rows["tenant-a|bank-fact-1|bank-fact/v1"].AmountMinor; got != 8000 || len(fixture.register.rows) != 1 {
		t.Fatalf("冲突不得顶替已登记内容，实得金额 %d、%d 行", got, len(fixture.register.rows))
	}
}

// Covers: 票 sa-cc/13 做法 2 + 完成判据 1 的消费侧——提供方的更正版本 v2（回指 v1、金额变）经同一事件类型再发的
// 一封到本适配器：版本取信封所指、回指取回查到的事实本体，编排`已接收`成新一行；两版并存、v1 一字不动、
// v2 回指 v1。本适配器不判「这是更正还是冲突」，只译（做法 3）。
func TestACorrectionVersionLandsAsASecondRowPointingBackToTheFirst(t *testing.T) {
	fixture := newFixture(t)
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v1"] = adoptedContent("bank-fact/v1", "", "payer-customer-7")
	corrected := adoptedContent("bank-fact/v2", "bank-fact/v1", "payer-customer-7")
	corrected.AmountMinor = 9000
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v2"] = corrected

	if err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v1")); err != nil {
		t.Fatalf("首版：%v", err)
	}
	if err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v2")); err != nil {
		t.Fatalf("更正版本：%v", err)
	}
	versions, err := fixture.register.ListFundsFactVersions(context.Background(),
		payerFixtureTenant(t), payerFixtureFact(t))
	if err != nil || len(versions) != 2 {
		t.Fatalf("两版该并存：err=%v n=%d", err, len(versions))
	}
	if versions[0].Version != versionOf("bank-fact/v1") || versions[0].Corrects != versionOf("") || versions[0].AmountMinor != 8000 {
		t.Fatalf("v1 该一字不动：%+v", versions[0])
	}
	if versions[1].Version != versionOf("bank-fact/v2") || versions[1].Corrects != versionOf("bank-fact/v1") || versions[1].AmountMinor != 9000 {
		t.Fatalf("v2 该回指 v1 且带自己的金额：%+v", versions[1])
	}
}

func payerFixtureTenant(t *testing.T) ccdomain.TenantID {
	t.Helper()
	tenant, err := ccdomain.NewTenantID("tenant-a")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	return tenant
}

func payerFixtureFact(t *testing.T) ccdomain.ExternalFundsFactReference {
	t.Helper()
	fact, err := ccdomain.NewExternalFundsFactReference("bank-fact-1")
	if err != nil {
		t.Fatalf("事实引用：%v", err)
	}
	return fact
}

// Covers: 裁决「取信封所指的那一版，不取 latest」的消费侧——提供方还没有那一版就是可见性滞后，
// 交回 ErrAdoptedFactNotVisible 让消费门回滚重投；不落毒丸，也不拿别的版本顶替。
func TestAnInvisibleVersionIsAContinuationNotAPoisonEnvelope(t *testing.T) {
	fixture := newFixture(t)
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v1"] = adoptedContent("bank-fact/v1", "", "payer-customer-7")

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

// Covers: 票 sa-cc/12 做法 1 / 4——提供方那一版没有付款人，消费侧读口译成「来源未提供」那一格、本适配器原样
// 转述、编排`已接收`，登记里付款人显式记为「未提供」（CONTEXT「未提供或不适用必须明确记录」）；本适配器
// 只译不判，要不要付款人是核对时对着真实程序的规则问的。sa-cc/03 时这一格是`未受理`的诚实停点，本票放宽。
func TestAFactWithoutAPayerIsReceivedWithThePayerRecordedAsNotProvided(t *testing.T) {
	fixture := newFixture(t)
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v1"] = adoptedContent("bank-fact/v1", "", "")

	if err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v1")); err != nil {
		t.Fatalf("来源未提供付款人的事实该被接收：%v", err)
	}
	row, found := fixture.register.rows["tenant-a|bank-fact-1|bank-fact/v1"]
	if !found {
		t.Fatal("来源未提供付款人的事实该落一条登记")
	}
	if row.Payer.Provided() || !row.Payer.Valid() {
		t.Fatalf("登记里付款人该显式为「未提供」，实得 %#v", row.Payer)
	}
}

// Covers: 编排未决（登记册不可用）是等依赖，交回 ErrFundsFactReceiveUndecided 让消费门重投。
func TestAnUndecidedOrchestrationIsRetried(t *testing.T) {
	fixture := newFixture(t)
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v1"] = adoptedContent("bank-fact/v1", "", "payer-customer-7")
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
// 付款人在提供方显式缺席就译成「来源未提供」那一格（票 sa-cc/12 裁决 3：`(value, bool)` 到格的译在消费侧）；
// 版本与回指前版随事实本体译出，首版回指为零值、更正版回指前版（票 sa-cc/13 做法 2）；
// 提供方答没有原样交回 false；空白版本构造不出提供方的键。
func TestTheSettlementSourceTranslatesTheProvidersFactIntoOurDimensions(t *testing.T) {
	first := saAdoptedFact(t, "payer-customer-7")
	secondVersion, err := sadomain.NewFundsFactVersion("bank-fact/v2")
	if err != nil {
		t.Fatal(err)
	}
	corrected, err := first.CorrectAmount(9000, secondVersion, fundsOccurredAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("提供方更正版本：%v", err)
	}
	view := &adoptedViewDouble{facts: map[string]sadomain.ExternalFundsFact{
		"tenant-a|bank-fact-1|bank-fact/v1": first,
		"tenant-a|bank-fact-1|bank-fact/v2": corrected,
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
	if want := adoptedContent("bank-fact/v1", "", "payer-customer-7"); got != want {
		t.Fatalf("译出 = %+v, want %+v", got, want)
	}
	got, found, err = source.LoadAdoptedFundsFact(context.Background(), tenantA, fact, "bank-fact/v2")
	if err != nil || !found {
		t.Fatalf("更正版本 found = %v err = %v", found, err)
	}
	if got.Version != versionOf("bank-fact/v2") || got.Corrects != versionOf("bank-fact/v1") || got.AmountMinor != 9000 {
		t.Fatalf("更正版本该带自己的版本、回指 v1 与更正后金额：%+v", got)
	}

	got, found, err = source.LoadAdoptedFundsFact(context.Background(), tenantB, fact, "bank-fact/v1")
	if err != nil || !found || got.Payer.Provided() || !got.Payer.Valid() {
		t.Fatalf("提供方显式缺席的付款人应译成「来源未提供」那一格：found = %v err = %v payer = %#v", found, err, got.Payer)
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

package settlementaccounting_test

import (
	"context"
	"errors"
	"sort"
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

func (double *fundsFactRegisterDouble) LoadFundsFactVersion(
	_ context.Context, tenant ccdomain.TenantID, fact ccdomain.ExternalFundsFactReference, version ccdomain.FundsFactVersion,
) (ccports.ExternalFundsFactRegistration, bool, error) {
	if double.err != nil {
		return ccports.ExternalFundsFactRegistration{}, false, double.err
	}
	registration, found := double.rows[registerKey(tenant, fact, version)]
	return registration, found, nil
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

// ListVerificationsByFundsFact 是新版本到达后编排问「这条事实有没有既往核对版本」的那一口（票 sa-cc/19 裁决 2）
// ——它是消费这条线上唯一该被碰到的核对读口；这本替身代表「从没核对过」，答空、不判。有既往核对的那一格在
// rederiveStores 上钉。
func (double unreachedDutyStores) ListVerificationsByFundsFact(
	context.Context, ccdomain.TenantID, ccdomain.ExternalFundsFactReference,
) ([]ccports.DutyVerificationRecord, error) {
	return nil, nil
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

// Covers: 票 sa-cc/23 条 1——回查按信封所指的版本取，交回的事实本体却自称另一版（提供方视图答非所问）：
// 不登记、交回 ErrAdoptedFactVersionInconsistent。它与 ErrAdoptedFactNotVisible 分开：重投不会把视图答对，
// 是装配 / 视图缺陷不是可见性滞后，消费门不重投；不核对时别版内容会登在信封那一版名下。
func TestASourceAnsweringAnotherVersionIsRefusedBeforeRegistering(t *testing.T) {
	fixture := newFixture(t)
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v2"] = adoptedContent("bank-fact/v1", "", "payer-customer-7")

	err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v2"))
	if !errors.Is(err, adapter.ErrAdoptedFactVersionInconsistent) {
		t.Fatalf("err = %v, want ErrAdoptedFactVersionInconsistent", err)
	}
	if errors.Is(err, adapter.ErrAdoptedFactNotVisible) {
		t.Fatal("答非所问不是可见性滞后，不得落进重投那一格")
	}
	if len(fixture.register.rows) != 0 {
		t.Fatalf("别版内容不得登在信封那一版名下：%d 行", len(fixture.register.rows))
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

// rederiveStores 是「已有既往核对」那一格要的几口：协作事项、核对册、付款人规则、交接——内存替身照真库代数（同键
// `已登记`不顶替）。与 unreachedDutyStores 分开：那本代表「消费这条线不该碰到」，这本代表「新版本到达后编排要
// 读写」，两本各守一件事。ListVerificationsByFundsFact 与端口口径同序（核对时刻升序、同刻按指纹字典序），同刻两版
// 靠它分先后——编排取各谱系最近一版时只在严格更晚时换人，替身不排序，同刻那一格就随 map 迭代序翻面。
type rederiveStores struct {
	collaborations  map[string]ccdomain.DutyPaymentCollaboration
	verifications   map[string]ccports.DutyVerificationRecord
	payerRules      map[string]ccdomain.PayerRequirement
	handoffs        []ccports.DutyPaymentVerificationHandoffIntent
	verificationErr error
}

func newRederiveStores() *rederiveStores {
	return &rederiveStores{
		collaborations: map[string]ccdomain.DutyPaymentCollaboration{},
		verifications:  map[string]ccports.DutyVerificationRecord{},
		payerRules:     map[string]ccdomain.PayerRequirement{},
	}
}

func (stores *rederiveStores) FindCollaboration(
	_ context.Context, tenant ccdomain.TenantID, scope ccdomain.DecisionScopeReference, duty ccdomain.AssessedDutyReference,
) (ccdomain.DutyPaymentCollaboration, bool, error) {
	collaboration, found := stores.collaborations[tenant.String()+"|"+scope.String()+"|"+duty.String()]
	return collaboration, found, nil
}

func (stores *rederiveStores) SaveCollaboration(
	_ context.Context, tenant ccdomain.TenantID, collaboration ccdomain.DutyPaymentCollaboration,
) (ccports.CaseConfigurationSaveOutcome, error) {
	duty, _ := collaboration.Duty()
	stores.collaborations[tenant.String()+"|"+collaboration.Scope().String()+"|"+duty.String()] = collaboration
	return ccports.CaseConfigurationRegistered, nil
}

func verificationKeyOf(key ccports.DutyVerificationKey) string {
	return key.TenantID.String() + "|" + key.Duty.String() + "|" + key.Funds.String() + "|" + key.Scope.String() + "|" + key.Digest
}

func (stores *rederiveStores) FindVerification(
	_ context.Context, key ccports.DutyVerificationKey,
) (ccports.DutyVerificationRecord, bool, error) {
	record, found := stores.verifications[verificationKeyOf(key)]
	return record, found, nil
}

func (stores *rederiveStores) ListVerificationsByFundsFact(
	_ context.Context, tenant ccdomain.TenantID, funds ccdomain.ExternalFundsFactReference,
) ([]ccports.DutyVerificationRecord, error) {
	if stores.verificationErr != nil {
		return nil, stores.verificationErr
	}
	var records []ccports.DutyVerificationRecord
	for _, record := range stores.verifications {
		if record.Key.TenantID == tenant && record.Key.Funds == funds {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		left, right := records[i].Verification.VerifiedAt(), records[j].Verification.VerifiedAt()
		if !left.Equal(right) {
			return left.Before(right)
		}
		return records[i].Key.Digest < records[j].Key.Digest
	})
	return records, nil
}

func (stores *rederiveStores) SaveVerification(
	_ context.Context, record ccports.DutyVerificationRecord,
) (ccports.CaseConfigurationSaveOutcome, error) {
	if stores.verificationErr != nil {
		return ccports.CaseConfigurationSaveOutcomeInvalid, stores.verificationErr
	}
	key := verificationKeyOf(record.Key)
	if _, exists := stores.verifications[key]; exists {
		return ccports.CaseConfigurationAlreadyRegistered, nil
	}
	stores.verifications[key] = record
	return ccports.CaseConfigurationRegistered, nil
}

func (stores *rederiveStores) LoadPayerRequirement(
	_ context.Context, tenant ccdomain.TenantID, procedure ccdomain.CustomsProcedureReference,
) (ccdomain.PayerRequirement, bool, error) {
	requirement, found := stores.payerRules[tenant.String()+"|"+procedure.String()]
	return requirement, found, nil
}

func (stores *rederiveStores) HandOffDutyPaymentVerification(
	_ context.Context, intent ccports.DutyPaymentVerificationHandoffIntent,
) error {
	stores.handoffs = append(stores.handoffs, intent)
	return nil
}

// verifiedFixture 铺好「v1 已接收且已核对（谱系 A：SYN-DUTY-01/v1 × declaration-unit-1 × SYN-PROC-IMPORT）」：
// 协作事项、付款人规则「要求」、v1 经本适配器落册、按 v1 走 VerifyPayment 成一版。交回适配器与几口替身。
func verifiedFixture(t *testing.T) (*fixture, *rederiveStores, *ccapplication.DutyPaymentReconciliationHandler) {
	t.Helper()
	source := &adoptedSourceDouble{facts: map[string]ccports.AdoptedFundsFact{}}
	register := newFundsFactRegister()
	stores := newRederiveStores()
	receiver, err := ccapplication.NewDutyPaymentReconciliationHandler(ccapplication.DutyPaymentReconciliationDeps{
		Collaborations: stores,
		Funds:          register,
		Verifications:  stores,
		PayerRules:     stores,
		Handoff:        stores,
		Clock:          dutyClock{at: fundsOccurredAt.Add(time.Hour)},
	})
	if err != nil {
		t.Fatalf("构造编排：%v", err)
	}
	handler, err := adapter.NewReceiveOnAdoptedFundsFactAdapter(source, receiver)
	if err != nil {
		t.Fatalf("构造处理方：%v", err)
	}
	fixture := &fixture{source: source, register: register, handler: handler}

	tenant := payerFixtureTenant(t)
	duty, _ := ccdomain.NewAssessedDutyReference("SYN-DUTY-01/v1")
	scope, _ := ccdomain.NewDecisionScopeReference("declaration-unit-1")
	procedure, _ := ccdomain.NewCustomsProcedureReference("SYN-PROC-IMPORT")
	obligor, _ := ccdomain.NewLegalObligorReference("SYN-OBLIGOR-01")
	requirement, _ := ccdomain.NewPaymentRequirementSource("SYN-ASSESSMENT-01")
	target, _ := ccdomain.NewResponsibilityTargetReference("SYN-DUTY-DESK")
	collaboration, err := ccdomain.FormDutyCollaboration(ccdomain.DutyCollaborationSpec{
		Kind: ccdomain.ObligationFromAssessedDuty, Duty: duty, Scope: scope,
		Obligor: obligor, Requirement: requirement, Target: target, FormedAt: fundsOccurredAt,
	})
	if err != nil {
		t.Fatalf("构造协作事项：%v", err)
	}
	if _, err := stores.SaveCollaboration(context.Background(), tenant, collaboration); err != nil {
		t.Fatalf("铺协作事项：%v", err)
	}
	stores.payerRules[tenant.String()+"|"+procedure.String()] = ccdomain.PayerRequired

	source.facts["tenant-a|bank-fact-1|bank-fact/v1"] = adoptedContent("bank-fact/v1", "", "payer-customer-7")
	if err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v1")); err != nil {
		t.Fatalf("v1：%v", err)
	}
	result, err := receiver.VerifyPayment(context.Background(), ccapplication.VerifyDutyPaymentCommand{
		TenantID: tenant, Duty: duty, Funds: payerFixtureFact(t), FundsVersion: versionOf("bank-fact/v1"),
		Scope: scope, Procedure: procedure,
		Coverage: ccdomain.CoverageFull, Delta: ccdomain.DeltaNone, Validity: ccdomain.FundsFactValid,
		Basis: "SYN-RULE-01: remittance quotes assessment",
	})
	if err != nil || result.Outcome() != ccapplication.DutyVerificationFormed {
		t.Fatalf("按 v1 核对：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
	}
	return fixture, stores, receiver
}

// Covers: 票 sa-cc/19 裁决 3 (1)——触发落点在本适配器：`ReceiveFundsFact` 答`已接收`之后、同一次处理里调重派编排；
// v1 已核对，v2 经信封到达 → 谱系 A 多一版（资金版本 v2、差额 / 有效性 PENDING、覆盖承前）、多一封交接；同一封 v2
// 重投（编排答`已存在`）不再触发——册上行数、信封数都不变；本适配器仍只译不判：三轴从哪来是编排的事，这里没有一行
// 在算。
func TestANewVersionArrivingThroughTheAdapterRederivesTheLineageOnceAndReplayDoesNot(t *testing.T) {
	fixture, stores, _ := verifiedFixture(t)
	if len(stores.verifications) != 1 || len(stores.handoffs) != 1 {
		t.Fatalf("夹具该只有 v1 那一版：%d 行 %d 封", len(stores.verifications), len(stores.handoffs))
	}
	corrected := adoptedContent("bank-fact/v2", "bank-fact/v1", "payer-customer-7")
	corrected.AmountMinor = 9000
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v2"] = corrected

	if err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v2")); err != nil {
		t.Fatalf("v2：%v", err)
	}
	if len(stores.verifications) != 2 || len(stores.handoffs) != 2 {
		t.Fatalf("新版本到达该多一版核对、多一封：%d 行 %d 封", len(stores.verifications), len(stores.handoffs))
	}
	formed := stores.verifications[verificationKeyOf(stores.handoffs[1].Key)]
	if formed.Verification.FundsVersion() != versionOf("bank-fact/v2") || formed.Verification.Delta() != ccdomain.DeltaPending ||
		formed.Verification.Validity() != ccdomain.FundsFactPending || formed.Verification.Coverage() != ccdomain.CoverageFull {
		t.Fatalf("新版本该是 (a′)：%+v", formed.Verification)
	}

	if err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v2")); err != nil {
		t.Fatalf("重投 v2：%v", err)
	}
	if len(stores.verifications) != 2 || len(stores.handoffs) != 2 {
		t.Fatalf("同版本重投不触发：%d 行 %d 封", len(stores.verifications), len(stores.handoffs))
	}
}

// Covers: 重派里的两种未决分开落（ADR-0029）：谱系的业务未决（程序要求付款人而 v2 没给）是编排的答案——入账、
// 不重投、册上不动；依赖故障（核对册不可用）交回 ErrDutyVerificationRederivationUndecided 让消费门连同接收一起回滚
// 重投——它与 ErrFundsFactReceiveUndecided 分开命名，运维从错误上读得出停在接收还是停在重派。
func TestRederivationUndecidedSplitsBusinessPendingFromDependencyFailure(t *testing.T) {
	fixture, stores, _ := verifiedFixture(t)
	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v2"] = adoptedContent("bank-fact/v2", "bank-fact/v1", "")
	if err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v2")); err != nil {
		t.Fatalf("程序要求付款人而 v2 没给是编排的答案，该入账：%v", err)
	}
	if len(stores.verifications) != 1 || len(stores.handoffs) != 1 {
		t.Fatalf("那条谱系不该形成：%d 行 %d 封", len(stores.verifications), len(stores.handoffs))
	}

	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v3"] = adoptedContent("bank-fact/v3", "bank-fact/v2", "payer-customer-7")
	stores.verificationErr = errors.New("verification store down")
	err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v3"))
	if !errors.Is(err, adapter.ErrDutyVerificationRederivationUndecided) {
		t.Fatalf("err = %v, want ErrDutyVerificationRederivationUndecided", err)
	}
	if errors.Is(err, adapter.ErrFundsFactReceiveUndecided) {
		t.Fatal("停在重派不是停在接收，两个哨兵不得混")
	}
}

// Covers: 票 sa-cc/19 判断项 ②「同刻两版取指纹字典序小的那版承前」写成断言（票 sa-cc/30 做法 2）。verifiedFixture 的
// 编排时钟固定，同一谱系在 v1 上再核一版（覆盖改 PARTIAL）就与首版同刻并存、只差指纹；v2 到达 → 编排在替身交回的序上
// 只在严格更晚时换人，同刻留先出现的那版——所以替身必须按端口口径排（核对时刻升序、同刻按指纹字典序），否则这一格
// 随 map 迭代序翻面。谁的指纹小由内容定，用例不写死、当场比。
func TestSameInstantVersionsHandTheirLineageToTheLexicographicallySmallerDigest(t *testing.T) {
	fixture, stores, receiver := verifiedFixture(t)
	duty, _ := ccdomain.NewAssessedDutyReference("SYN-DUTY-01/v1")
	scope, _ := ccdomain.NewDecisionScopeReference("declaration-unit-1")
	procedure, _ := ccdomain.NewCustomsProcedureReference("SYN-PROC-IMPORT")
	result, err := receiver.VerifyPayment(context.Background(), ccapplication.VerifyDutyPaymentCommand{
		TenantID: payerFixtureTenant(t), Duty: duty, Funds: payerFixtureFact(t), FundsVersion: versionOf("bank-fact/v1"),
		Scope: scope, Procedure: procedure,
		Coverage: ccdomain.CoveragePartial, Delta: ccdomain.DeltaShort, Validity: ccdomain.FundsFactValid,
		Basis: "SYN-RULE-01: remittance quotes assessment",
	})
	if err != nil || result.Outcome() != ccapplication.DutyVerificationFormed {
		t.Fatalf("同刻再核一版：err=%v outcome=%v", err, result.Outcome())
	}
	first := stores.verifications[verificationKeyOf(stores.handoffs[0].Key)]
	second := stores.verifications[verificationKeyOf(stores.handoffs[1].Key)]
	if !first.Verification.VerifiedAt().Equal(second.Verification.VerifiedAt()) || first.Key.Digest == second.Key.Digest ||
		first.Verification.Coverage() == second.Verification.Coverage() {
		t.Fatalf("夹具该是同刻两版、指纹与覆盖都不同：%+v / %+v", first.Key, second.Key)
	}
	expected := first
	if second.Key.Digest < first.Key.Digest {
		expected = second
	}

	fixture.source.facts["tenant-a|bank-fact-1|bank-fact/v2"] = adoptedContent("bank-fact/v2", "bank-fact/v1", "payer-customer-7")
	if err := fixture.handler.HandleAdoptedExternalFundsFact(context.Background(), adoptedEnvelopeRef("bank-fact/v2")); err != nil {
		t.Fatalf("v2：%v", err)
	}
	if len(stores.verifications) != 3 || len(stores.handoffs) != 3 {
		t.Fatalf("同一谱系该只多一版、多一封：%d 行 %d 封", len(stores.verifications), len(stores.handoffs))
	}
	rederived := stores.verifications[verificationKeyOf(stores.handoffs[2].Key)]
	if rederived.Verification.FundsVersion() != versionOf("bank-fact/v2") ||
		rederived.Verification.Coverage() != expected.Verification.Coverage() {
		t.Fatalf("(a′) 该承同刻两版里指纹字典序小的那版（%s，覆盖 %s）：%+v",
			expected.Key.Digest, expected.Verification.Coverage(), rederived.Verification)
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

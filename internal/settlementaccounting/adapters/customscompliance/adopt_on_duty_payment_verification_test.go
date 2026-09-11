package customscompliance_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/customscompliance"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var adoptionNow = time.Date(2026, 9, 11, 9, 30, 0, 0, time.UTC)

// verificationViewDouble 是本上下文读提供方那口的替身：按（租户|范围|税费|资金|指纹）答在不在。
type verificationViewDouble struct {
	visible map[string]bool
	err     error
	asked   int
}

func viewKey(tenant sadomain.TenantID, verification sadomain.DutyPaymentVerificationReference) string {
	return tenant.String() + "|" + verification.Scope().String() + "|" + verification.Duty().String() +
		"|" + verification.Funds().String() + "|" + verification.Version().String()
}

func (double *verificationViewDouble) DutyPaymentVerificationExists(
	_ context.Context, tenant sadomain.TenantID, verification sadomain.DutyPaymentVerificationReference,
) (bool, error) {
	double.asked++
	if double.err != nil {
		return false, double.err
	}
	return double.visible[viewKey(tenant, verification)], nil
}

// adoptionStoreDouble 是采用登记册的内存替身（按键幂等）。
type adoptionStoreDouble struct {
	records map[string]saports.DutyPaymentVerificationAdoptionRecord
	err     error
}

func (double *adoptionStoreDouble) FindByKey(
	_ context.Context, key saports.DutyPaymentVerificationAdoptionKey,
) (saports.DutyPaymentVerificationAdoptionRecord, bool, error) {
	if double.err != nil {
		return saports.DutyPaymentVerificationAdoptionRecord{}, false, double.err
	}
	record, found := double.records[viewKey(key.TenantID, key.Verification)]
	return record, found, nil
}

func (double *adoptionStoreDouble) Save(
	_ context.Context, record saports.DutyPaymentVerificationAdoptionRecord,
) (saports.DutyPaymentVerificationAdoptionSaveOutcome, error) {
	if double.err != nil {
		return saports.DutyPaymentVerificationAdoptionSaveOutcomeInvalid, double.err
	}
	key := viewKey(record.Key.TenantID, record.Key.Verification)
	if _, exists := double.records[key]; exists {
		return saports.DutyPaymentVerificationAlreadyAdopted, nil
	}
	double.records[key] = record
	return saports.DutyPaymentVerificationAdoptionSaved, nil
}

// unreachedAdvanceStores 顶住编排构造期要求、而本适配器不走的那些口（评估库、回收库、调整库、合同视图、
// 回收交接）。碰到即测试失败——替身守的是票面红线「消费者只译不判、不调 FormRecovery」，不是给它们内存实现。
type unreachedAdvanceStores struct{ t *testing.T }

func (double unreachedAdvanceStores) FindByKey(context.Context, saports.AdvanceAssessmentKey) (saports.AdvanceAssessmentRecord, bool, error) {
	double.t.Helper()
	double.t.Fatal("采用付款核对不该读实际代垫评估")
	return saports.AdvanceAssessmentRecord{}, false, nil
}

func (double unreachedAdvanceStores) Save(context.Context, saports.AdvanceAssessmentRecord) (saports.AdvanceAssessmentSaveOutcome, error) {
	double.t.Helper()
	double.t.Fatal("采用付款核对不该形成实际代垫评估")
	return saports.AdvanceAssessmentSaveOutcomeInvalid, nil
}

type unreachedRecoveryStores struct{ t *testing.T }

func (double unreachedRecoveryStores) FindByKey(context.Context, saports.AdvanceRecoveryKey) (saports.AdvanceRecoveryRecord, bool, error) {
	double.t.Helper()
	double.t.Fatal("采用付款核对不该读客户代垫回收")
	return saports.AdvanceRecoveryRecord{}, false, nil
}

func (double unreachedRecoveryStores) Save(context.Context, saports.AdvanceRecoveryRecord) (saports.AdvanceRecoverySaveOutcome, error) {
	double.t.Helper()
	double.t.Fatal("采用付款核对不该形成客户代垫回收")
	return saports.AdvanceRecoverySaveOutcomeInvalid, nil
}

type unreachedAdjustmentStores struct{ t *testing.T }

func (double unreachedAdjustmentStores) FindByKey(context.Context, saports.RecoveryAdjustmentKey) (saports.RecoveryAdjustmentRecord, bool, error) {
	double.t.Helper()
	double.t.Fatal("采用付款核对不该读回收调整")
	return saports.RecoveryAdjustmentRecord{}, false, nil
}

func (double unreachedAdjustmentStores) Save(context.Context, saports.RecoveryAdjustmentRecord) (saports.RecoveryAdjustmentSaveOutcome, error) {
	double.t.Helper()
	double.t.Fatal("采用付款核对不该形成回收调整")
	return saports.RecoveryAdjustmentSaveOutcomeInvalid, nil
}

type unreachedContractView struct{ t *testing.T }

func (double unreachedContractView) LoadContractResponsibility(
	context.Context, sadomain.TenantID, sadomain.RecoveryCustomerReference, sadomain.TaxObligationReference,
) (sadomain.ContractResponsibilityReference, bool, error) {
	double.t.Helper()
	double.t.Fatal("采用付款核对不该读合同责任——那是回收那一步的事")
	return sadomain.ContractResponsibilityReference{}, false, nil
}

type unreachedRecoveryHandoff struct{ t *testing.T }

func (double unreachedRecoveryHandoff) HandOffAdvanceRecovery(context.Context, saports.AdvanceRecoveryIntent) error {
	double.t.Helper()
	double.t.Fatal("采用付款核对不交任何回收意图")
	return nil
}

type adoptionClock struct{ at time.Time }

func (clock adoptionClock) Now() time.Time { return clock.at }

type fixture struct {
	view    *verificationViewDouble
	store   *adoptionStoreDouble
	handler *adapter.AdoptOnDutyPaymentVerificationAdapter
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	view := &verificationViewDouble{visible: map[string]bool{}}
	store := &adoptionStoreDouble{records: map[string]saports.DutyPaymentVerificationAdoptionRecord{}}
	adopter := saapplication.NewAssessAdvanceRecoveryHandler(saapplication.AssessAdvanceRecoveryDeps{
		Assessments:      unreachedAdvanceStores{t: t},
		Recoveries:       unreachedRecoveryStores{t: t},
		Adjustments:      unreachedAdjustmentStores{t: t},
		Contracts:        unreachedContractView{t: t},
		Downstream:       unreachedRecoveryHandoff{t: t},
		SettlementInputs: store,
		Clock:            adoptionClock{at: adoptionNow},
	})
	handler, err := adapter.NewAdoptOnDutyPaymentVerificationAdapter(view, adopter)
	if err != nil {
		t.Fatalf("构造适配器：%v", err)
	}
	return &fixture{view: view, store: store, handler: handler}
}

func formed() sainbox.FormedDutyPaymentVerification {
	return sainbox.FormedDutyPaymentVerification{
		TenantID: "tenant-a",
		Scope:    "SYN-UNIT-01",
		Duty:     "duty-1",
		Funds:    "bank-fact-1",
		Digest:   "digest-1",
	}
}

func (fixture *fixture) makeVisible(t *testing.T, formed sainbox.FormedDutyPaymentVerification) {
	t.Helper()
	tenant, _ := sadomain.NewTenantID(formed.TenantID)
	scope, _ := sadomain.NewDeclarationScopeReference(formed.Scope)
	duty, _ := sadomain.NewTaxObligationReference(formed.Duty)
	funds, _ := sadomain.NewFundsFactReference(formed.Funds)
	version, _ := sadomain.NewDutyVerificationVersion(formed.Digest)
	reference, err := sadomain.NewDutyPaymentVerificationReference(scope, duty, funds, version)
	if err != nil {
		t.Fatalf("引用：%v", err)
	}
	fixture.view.visible[viewKey(tenant, reference)] = true
}

// Covers: 完成判据 2——一封 → 结算输入版本采用付款核对一格；重投 → `已存在`、登记条数仍 1；全程一次都没碰
// 评估 / 回收 / 调整 / 合同 / 交接各口（unreached 替身守着），即「其余输入缺 → 待判断」而不是错误。
func TestAFormedVerificationIsAdoptedOnceAndReplayStillSettles(t *testing.T) {
	fixture := newFixture(t)
	fixture.makeVisible(t, formed())

	if err := fixture.handler.HandleFormedDutyPaymentVerification(t.Context(), formed()); err != nil {
		t.Fatalf("首投：%v", err)
	}
	if len(fixture.store.records) != 1 {
		t.Fatalf("登记条数 = %d, want 1", len(fixture.store.records))
	}
	var record saports.DutyPaymentVerificationAdoptionRecord
	for _, stored := range fixture.store.records {
		record = stored
	}
	verification := record.Adoption.Verification()
	if verification.Scope().String() != "SYN-UNIT-01" || verification.Duty().String() != "duty-1" ||
		verification.Funds().String() != "bank-fact-1" || verification.Version().String() != "digest-1" {
		t.Fatalf("采用的引用 = %+v，want 信封所指的那一版", verification)
	}
	if !record.Adoption.AdoptedAt().Equal(adoptionNow) {
		t.Fatalf("采用时刻 = %v, want %v", record.Adoption.AdoptedAt(), adoptionNow)
	}

	if err := fixture.handler.HandleFormedDutyPaymentVerification(t.Context(), formed()); err != nil {
		t.Fatalf("重投应入账：%v", err)
	}
	if len(fixture.store.records) != 1 {
		t.Fatalf("重投后登记条数 = %d, want 1", len(fixture.store.records))
	}
}

// Covers: 信封所指的那一版在提供方还看不见 → 未决重投，不采用、不毒丸。
func TestAnInvisibleVerificationIsUndecidedAndNothingIsAdopted(t *testing.T) {
	fixture := newFixture(t)

	err := fixture.handler.HandleFormedDutyPaymentVerification(t.Context(), formed())
	if !errors.Is(err, adapter.ErrVerificationNotVisible) {
		t.Fatalf("err = %v, want ErrVerificationNotVisible", err)
	}
	if len(fixture.store.records) != 0 {
		t.Fatal("看不见的版本被采用了")
	}

	// 提供方读口自身故障同样归可见性未决：两者重投都会改变结果。
	fixture.view.err = errors.New("cc read failed")
	err = fixture.handler.HandleFormedDutyPaymentVerification(t.Context(), formed())
	if !errors.Is(err, adapter.ErrVerificationNotVisible) {
		t.Fatalf("读口故障：err = %v, want ErrVerificationNotVisible", err)
	}
}

// Covers: 换指纹的下一版是另一次采用，不是重放——两版各一行。
func TestANewVerificationVersionIsAdoptedSeparately(t *testing.T) {
	fixture := newFixture(t)
	first := formed()
	next := formed()
	next.Digest = "digest-2"
	fixture.makeVisible(t, first)
	fixture.makeVisible(t, next)

	for _, envelope := range []sainbox.FormedDutyPaymentVerification{first, next} {
		if err := fixture.handler.HandleFormedDutyPaymentVerification(t.Context(), envelope); err != nil {
			t.Fatalf("%s：%v", envelope.Digest, err)
		}
	}
	if len(fixture.store.records) != 2 {
		t.Fatalf("登记条数 = %d, want 2", len(fixture.store.records))
	}
}

// Covers: 采用登记册不可用 → 编排答未决 → 消费重投，不入账。
func TestAnUnavailableAdoptionStoreIsUndecided(t *testing.T) {
	fixture := newFixture(t)
	fixture.makeVisible(t, formed())
	fixture.store.err = errors.New("db down")

	err := fixture.handler.HandleFormedDutyPaymentVerification(t.Context(), formed())
	if !errors.Is(err, adapter.ErrAdoptionUndecided) {
		t.Fatalf("err = %v, want ErrAdoptionUndecided", err)
	}
}

// Covers: 空白维度在本上下文词汇里构造不出引用——消费者译码已拒五维缺席，这里只可能是词汇分歧，响亮报错
// 而不是问提供方。
func TestABlankDimensionDoesNotTranslate(t *testing.T) {
	fixture := newFixture(t)
	blank := formed()
	blank.Scope = "   "

	err := fixture.handler.HandleFormedDutyPaymentVerification(t.Context(), blank)
	if !errors.Is(err, adapter.ErrUntranslatableReference) {
		t.Fatalf("err = %v, want ErrUntranslatableReference", err)
	}
	if fixture.view.asked != 0 {
		t.Fatal("引用都构造不出还去问了提供方")
	}
}

func TestTheAdoptOnVerificationAdapterRefusesNilDependencies(t *testing.T) {
	view := &verificationViewDouble{visible: map[string]bool{}}
	adopter := saapplication.NewAssessAdvanceRecoveryHandler(saapplication.AssessAdvanceRecoveryDeps{})
	cases := map[string]struct {
		view    saports.DutyPaymentVerificationView
		adopter adapter.DutyPaymentVerificationAdopter
	}{
		"view 为 nil":    {view: nil, adopter: adopter},
		"adopter 为 nil": {view: view, adopter: nil},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := adapter.NewAdoptOnDutyPaymentVerificationAdapter(test.view, test.adopter); err == nil {
				t.Fatal("nil 依赖被收下了")
			}
		})
	}
}

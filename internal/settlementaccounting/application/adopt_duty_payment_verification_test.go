package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// adoptionStoreDouble 是付款核对采用登记册的内存替身：按（租户 + 完整引用）幂等。
type adoptionStoreDouble struct {
	records map[string]ports.DutyPaymentVerificationAdoptionRecord
	findErr error
	saveErr error
	saves   int
}

func newAdoptionStore() *adoptionStoreDouble {
	return &adoptionStoreDouble{records: map[string]ports.DutyPaymentVerificationAdoptionRecord{}}
}

func adoptionKey(key ports.DutyPaymentVerificationAdoptionKey) string {
	verification := key.Verification
	return key.TenantID.String() + "|" + verification.Scope().String() + "|" + verification.Duty().String() +
		"|" + verification.Funds().String() + "|" + verification.Version().String()
}

func (double *adoptionStoreDouble) FindByKey(
	_ context.Context,
	key ports.DutyPaymentVerificationAdoptionKey,
) (ports.DutyPaymentVerificationAdoptionRecord, bool, error) {
	if double.findErr != nil {
		return ports.DutyPaymentVerificationAdoptionRecord{}, false, double.findErr
	}
	record, found := double.records[adoptionKey(key)]
	return record, found, nil
}

func (double *adoptionStoreDouble) Save(
	_ context.Context,
	record ports.DutyPaymentVerificationAdoptionRecord,
) (ports.DutyPaymentVerificationAdoptionSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.DutyPaymentVerificationAdoptionSaveOutcomeInvalid, double.saveErr
	}
	if _, exists := double.records[adoptionKey(record.Key)]; exists {
		return ports.DutyPaymentVerificationAlreadyAdopted, nil
	}
	double.records[adoptionKey(record.Key)] = record
	return ports.DutyPaymentVerificationAdoptionSaved, nil
}

type inputFixture struct {
	*advanceFixture
	inputs *adoptionStoreDouble
}

// newInputFixture 在 advanceFixture 之上接采用登记册。其余各口沿用同一批替身：本用例要证的正是采用
// 一格不碰它们。
func newInputFixture(t *testing.T) *inputFixture {
	t.Helper()
	fixture := &inputFixture{advanceFixture: newAdvanceFixture(t), inputs: newAdoptionStore()}
	fixture.handler = application.NewAssessAdvanceRecoveryHandler(application.AssessAdvanceRecoveryDeps{
		Assessments:      fixture.assessments,
		Recoveries:       fixture.recoveries,
		Adjustments:      fixture.adjustments,
		Contracts:        fixture.contracts,
		Downstream:       fixture.handoff,
		SettlementInputs: fixture.inputs,
		Clock:            advanceClock{at: advanceNowAt},
	})
	return fixture
}

func adoptVerificationCommand(t *testing.T) application.AdoptDutyPaymentVerificationCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.AdoptDutyPaymentVerificationCommand{
		TenantID: tenant,
		Scope:    "SYN-UNIT-01",
		Duty:     "duty-1",
		Funds:    "bank-fact-1",
		Version:  "digest-1",
	}
}

// Covers: 完成判据 2 前半——一封 → 结算输入版本采用付款核对一格；登记只有引用与采用时刻。
func TestAdoptingAVerificationRecordsTheReferenceAndTheInstant(t *testing.T) {
	fixture := newInputFixture(t)

	result, err := fixture.handler.AdoptDutyPaymentVerification(t.Context(), adoptVerificationCommand(t))
	if err != nil {
		t.Fatalf("采用：%v", err)
	}
	if result.Outcome() != application.DutyPaymentVerificationAdopted {
		t.Fatalf("outcome = %s, want DUTY_PAYMENT_VERIFICATION_ADOPTED", result.Outcome())
	}
	record, present := result.Adoption()
	if !present {
		t.Fatal("结果没带采用记录")
	}
	verification := record.Adoption.Verification()
	if verification.Scope().String() != "SYN-UNIT-01" || verification.Duty().String() != "duty-1" ||
		verification.Funds().String() != "bank-fact-1" || verification.Version().String() != "digest-1" {
		t.Fatalf("采用的引用 = %+v，want 信封所指的那一版", verification)
	}
	if !record.Adoption.AdoptedAt().Equal(advanceNowAt) {
		t.Fatalf("采用时刻 = %v, want 时钟 %v", record.Adoption.AdoptedAt(), advanceNowAt)
	}
	if len(fixture.inputs.records) != 1 {
		t.Fatalf("登记条数 = %d, want 1", len(fixture.inputs.records))
	}
}

// Covers: 完成判据 2 后半——其余输入缺 → 待判断（不是错误）。采用一格不形成实际代垫判断、不形成回收、
// 不交任何意图：Assessments / Recoveries / Downstream 一次都没被碰到，结果也不是错误或未决。
// 「待判断」在这里就是「判断尚未形成」——没有一行进 advance_assessment，而这正是 UC-SA-001 步 2
// 「缺失保持待判断」的形。
func TestAdoptingAVerificationLeavesTheAdvanceAssessmentUndecided(t *testing.T) {
	fixture := newInputFixture(t)

	result, err := fixture.handler.AdoptDutyPaymentVerification(t.Context(), adoptVerificationCommand(t))
	if err != nil {
		t.Fatalf("采用：%v", err)
	}
	if result.Outcome() != application.DutyPaymentVerificationAdopted {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	if fixture.assessments.saves != 0 || len(fixture.assessments.records) != 0 {
		t.Fatalf("采用付款核对不得形成实际代垫判断：saves = %d, records = %d",
			fixture.assessments.saves, len(fixture.assessments.records))
	}
	if len(fixture.recoveries.records) != 0 || len(fixture.adjustments.records) != 0 {
		t.Fatal("采用付款核对不得形成回收或调整")
	}
	if len(fixture.handoff.intents) != 0 {
		t.Fatalf("采用付款核对不交任何回收意图：intents = %d", len(fixture.handoff.intents))
	}
}

// Covers: 完成判据 2 中段——重投 → `已存在`；登记条数不翻倍，第二次答的是先到的那一版。
func TestReplayingTheSameVerificationAnswersExisting(t *testing.T) {
	fixture := newInputFixture(t)

	first, err := fixture.handler.AdoptDutyPaymentVerification(t.Context(), adoptVerificationCommand(t))
	if err != nil {
		t.Fatalf("首投：%v", err)
	}
	second, err := fixture.handler.AdoptDutyPaymentVerification(t.Context(), adoptVerificationCommand(t))
	if err != nil {
		t.Fatalf("重投：%v", err)
	}
	if second.Outcome() != application.DutyPaymentVerificationExistingResult {
		t.Fatalf("重投 outcome = %s, want EXISTING_DUTY_PAYMENT_VERIFICATION", second.Outcome())
	}
	firstRecord, _ := first.Adoption()
	secondRecord, present := second.Adoption()
	if !present || secondRecord != firstRecord {
		t.Fatalf("重投交回的记录 = %+v, want 首投那一份", secondRecord)
	}
	if len(fixture.inputs.records) != 1 || fixture.inputs.saves != 1 {
		t.Fatalf("登记条数 = %d, saves = %d, want 1 / 1", len(fixture.inputs.records), fixture.inputs.saves)
	}
}

// Covers: 同一范围的下一版核对（换指纹）是新的采用，不是重放也不是冲突——核对版本不可变，内容变了在
// customs-compliance 那头就是新版本、新信封。
func TestANewVerificationVersionIsASeparateAdoption(t *testing.T) {
	fixture := newInputFixture(t)

	if _, err := fixture.handler.AdoptDutyPaymentVerification(t.Context(), adoptVerificationCommand(t)); err != nil {
		t.Fatalf("首版：%v", err)
	}
	next := adoptVerificationCommand(t)
	next.Version = "digest-2"
	result, err := fixture.handler.AdoptDutyPaymentVerification(t.Context(), next)
	if err != nil {
		t.Fatalf("次版：%v", err)
	}
	if result.Outcome() != application.DutyPaymentVerificationAdopted {
		t.Fatalf("次版 outcome = %s, want DUTY_PAYMENT_VERIFICATION_ADOPTED", result.Outcome())
	}
	if len(fixture.inputs.records) != 2 {
		t.Fatalf("登记条数 = %d, want 2", len(fixture.inputs.records))
	}
}

func TestAVerificationReferenceMissingADimensionIsNotAccepted(t *testing.T) {
	blank := func(mutate func(*application.AdoptDutyPaymentVerificationCommand)) application.AdoptDutyPaymentVerificationCommand {
		command := adoptVerificationCommand(t)
		mutate(&command)
		return command
	}
	cases := map[string]application.AdoptDutyPaymentVerificationCommand{
		// 租户零值不该走到登记册撞 refs_not_blank 被译成`未决`——那一格重投不自愈（sa-cc/09 评审 Standards (1)）。
		"缺租户":   blank(func(c *application.AdoptDutyPaymentVerificationCommand) { c.TenantID = domain.TenantID{} }),
		"缺申报范围": blank(func(c *application.AdoptDutyPaymentVerificationCommand) { c.Scope = " " }),
		"缺税费义务": blank(func(c *application.AdoptDutyPaymentVerificationCommand) { c.Duty = "" }),
		"缺资金事实": blank(func(c *application.AdoptDutyPaymentVerificationCommand) { c.Funds = "" }),
		"缺版本指纹": blank(func(c *application.AdoptDutyPaymentVerificationCommand) { c.Version = "" }),
	}
	for name, command := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newInputFixture(t)
			result, err := fixture.handler.AdoptDutyPaymentVerification(t.Context(), command)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if result.Outcome() != application.SettlementInputNotAccepted {
				t.Fatalf("outcome = %s, want SOURCE_NOT_ACCEPTED", result.Outcome())
			}
			if fixture.inputs.saves != 0 {
				t.Fatal("未受理不得写登记册")
			}
		})
	}
}

// Covers: 登记册不可用 → 未决带续办引用，不是错误也不是未受理——等依赖，重投会改变结果。
func TestAnUnavailableInputStoreLeavesTheAdoptionUndecided(t *testing.T) {
	for name, arrange := range map[string]func(*adoptionStoreDouble){
		"读口故障": func(store *adoptionStoreDouble) { store.findErr = errors.New("db down") },
		"写口故障": func(store *adoptionStoreDouble) { store.saveErr = errors.New("db down") },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newInputFixture(t)
			arrange(fixture.inputs)

			result, err := fixture.handler.AdoptDutyPaymentVerification(t.Context(), adoptVerificationCommand(t))
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if result.Outcome() != application.SettlementInputUndecidedOutcome {
				t.Fatalf("outcome = %s, want SETTLEMENT_INPUT_UNDECIDED", result.Outcome())
			}
			if result.UndecidedReason() != application.InputStoreUnavailable {
				t.Fatalf("reason = %s, want INPUT_STORE_UNAVAILABLE", result.UndecidedReason())
			}
			if result.ContinuationReference() == "" {
				t.Fatal("未决要带续办引用")
			}
		})
	}
}

// Covers: 并发落败读回赢家（ADR-0031）——Save 答`已采用`时不覆盖，读回先到者作答。
func TestALostAdoptionRaceReadsBackTheWinner(t *testing.T) {
	fixture := newInputFixture(t)
	command := adoptVerificationCommand(t)
	key := ports.DutyPaymentVerificationAdoptionKey{TenantID: command.TenantID, Verification: mustReference(t, command)}
	winnerAdoption, err := domain.AdoptDutyPaymentVerification(key.Verification, advanceNowAt.Add(-time.Minute))
	if err != nil {
		t.Fatalf("赢家：%v", err)
	}
	winner := ports.DutyPaymentVerificationAdoptionRecord{Key: key, Adoption: winnerAdoption}
	fixture.inputs.records[adoptionKey(key)] = winner
	// 首次 FindByKey 答「没有」、Save 答「已采用」、再读才见赢家：模拟两次读之间另一方先落。
	racing := &racingAdoptionStore{inner: fixture.inputs}
	fixture.handler = application.NewAssessAdvanceRecoveryHandler(application.AssessAdvanceRecoveryDeps{
		Assessments:      fixture.assessments,
		Recoveries:       fixture.recoveries,
		Adjustments:      fixture.adjustments,
		Contracts:        fixture.contracts,
		Downstream:       fixture.handoff,
		SettlementInputs: racing,
		Clock:            advanceClock{at: advanceNowAt},
	})

	result, err := fixture.handler.AdoptDutyPaymentVerification(t.Context(), command)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if result.Outcome() != application.DutyPaymentVerificationExistingResult {
		t.Fatalf("outcome = %s, want EXISTING_DUTY_PAYMENT_VERIFICATION", result.Outcome())
	}
	if got, _ := result.Adoption(); got != winner {
		t.Fatalf("落败方交回的记录 = %+v, want 赢家那一份", got)
	}
}

// racingAdoptionStore 让首次 FindByKey 答「没有」，Save 答「已采用」，之后 FindByKey 才答赢家。
type racingAdoptionStore struct {
	inner *adoptionStoreDouble
	reads int
}

func (store *racingAdoptionStore) FindByKey(
	ctx context.Context,
	key ports.DutyPaymentVerificationAdoptionKey,
) (ports.DutyPaymentVerificationAdoptionRecord, bool, error) {
	store.reads++
	if store.reads == 1 {
		return ports.DutyPaymentVerificationAdoptionRecord{}, false, nil
	}
	return store.inner.FindByKey(ctx, key)
}

func (store *racingAdoptionStore) Save(
	context.Context,
	ports.DutyPaymentVerificationAdoptionRecord,
) (ports.DutyPaymentVerificationAdoptionSaveOutcome, error) {
	return ports.DutyPaymentVerificationAlreadyAdopted, nil
}

func mustReference(t *testing.T, command application.AdoptDutyPaymentVerificationCommand) domain.DutyPaymentVerificationReference {
	t.Helper()
	scope, _ := domain.NewDeclarationScopeReference(command.Scope)
	duty, _ := domain.NewTaxObligationReference(command.Duty)
	funds, _ := domain.NewFundsFactReference(command.Funds)
	version, _ := domain.NewDutyVerificationVersion(command.Version)
	reference, err := domain.NewDutyPaymentVerificationReference(scope, duty, funds, version)
	if err != nil {
		t.Fatalf("引用：%v", err)
	}
	return reference
}

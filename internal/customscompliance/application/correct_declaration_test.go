package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 本文件是原案内更正/补充编排（CorrectDeclarationHandler）的用例，夹具沿用
// submit_declaration_test.go 的替身族；submissionStoreDouble 的多版本两法补在这里。

func (double *submissionStoreDouble) FindByVersion(
	_ context.Context,
	tenant domain.TenantID,
	version domain.SubmissionVersionID,
) (ports.DeclarationSubmissionRecord, bool, error) {
	if double.findErr != nil {
		return ports.DeclarationSubmissionRecord{}, false, double.findErr
	}
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Version.ID() == version {
			return record, true, nil
		}
	}
	return ports.DeclarationSubmissionRecord{}, false, nil
}

// SaveCorrection 照真库代数：前身仍是当前版才翻旧插新，否则答`当前版已被换`绝不顶替。
func (double *submissionStoreDouble) SaveCorrection(
	_ context.Context,
	record ports.DeclarationSubmissionRecord,
) (ports.DeclarationCorrectionSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.DeclarationCorrectionSaveOutcomeInvalid, double.saveErr
	}
	if double.forceCorrection {
		return ports.DeclarationCorrectionCurrentMoved, nil
	}
	current, exists := double.records[submissionKey(record.Key)]
	if !exists || current.Version.ID() != record.CorrectedFrom {
		return ports.DeclarationCorrectionCurrentMoved, nil
	}
	double.records[submissionKey(record.Key)] = record
	return ports.DeclarationCorrectionSaved, nil
}

// newFollowUpStore 沿用 manage_follow_up_test.go 的替身，两张空册起步。
func newFollowUpStore() *followUpStoreDouble {
	return &followUpStoreDouble{
		targets:   map[ports.FollowUpTargetKey]domain.FollowUpTarget{},
		relations: map[ports.FollowUpTargetKey]domain.ReplacementRelation{},
	}
}

type correctionFixture struct {
	declaration *declarationFixture
	followUps   *followUpStoreDouble
	handler     *application.CorrectDeclarationHandler
}

// newCorrectionFixture 先经真提交编排落 V1（更正只能接在在册提交之后），再装更正编排。
func newCorrectionFixture(t *testing.T) *correctionFixture {
	t.Helper()
	declaration := newDeclarationFixture(t)
	if result, err := declaration.handler.Handle(context.Background(), declarationCommand(t)); err != nil ||
		result.Outcome() != application.DeclarationSubmitted {
		t.Fatalf("预置首版：outcome=%v err=%v", result.Outcome(), err)
	}
	fixture := &correctionFixture{declaration: declaration, followUps: newFollowUpStore()}
	fixture.handler = application.NewCorrectDeclarationHandler(application.CorrectDeclarationDeps{
		Submissions: declaration.store,
		Units:       declaration.units,
		FollowUps:   fixture.followUps,
		Readiness:   declaration.readiness,
		Authority:   declaration.authority,
		Versions:    declaration.versions,
		Downstream:  declaration.handoff,
		Clock:       declarationClock{at: declarationFixedAt.Add(time.Hour)},
	})
	return fixture
}

// formTargetOnCurrent 对当前版形成一个原案内目标（走领域真构造门，六件齐全）。
func (fixture *correctionFixture) formTargetOnCurrent(
	t *testing.T, kind domain.FollowUpActionKind, trigger, version string,
) {
	t.Helper()
	key := ports.FollowUpTargetKey{
		TenantID: declarationTenant(t),
		Trigger:  declarationValueT(t, domain.NewFollowUpTriggerReference, trigger),
		Version:  declarationValueT(t, domain.NewSubmissionVersionID, version),
		Kind:     kind,
	}
	target, err := domain.FormFollowUpTarget(domain.FollowUpTargetSpec{
		Kind:     kind,
		Trigger:  key.Trigger,
		CaseRef:  declarationValueT(t, domain.NewCustomsCaseID, "case-1"),
		Unit:     declarationValueT(t, domain.NewDeclarationUnitID, "declaration-unit-1"),
		Version:  key.Version,
		Scope:    declarationValueT(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		FormedAt: declarationFixedAt.Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("形成后续动作目标：%v", err)
	}
	if _, err := fixture.followUps.SaveTarget(context.Background(), key, target); err != nil {
		t.Fatalf("登记后续动作目标：%v", err)
	}
}

func declarationTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return tenant
}

func declarationValueT[T any](t *testing.T, form func(string) (T, error), value string) T {
	t.Helper()
	formed, err := form(value)
	if err != nil {
		t.Fatalf("构造 %q：%v", value, err)
	}
	return formed
}

func correctionCommand(t *testing.T) application.CorrectDeclarationCommand {
	t.Helper()
	return application.CorrectDeclarationCommand{
		TenantID:      declarationTenant(t),
		UnitID:        "declaration-unit-1",
		CaseID:        "case-1",
		Procedure:     "US-IMPORT-T86",
		Members:       []string{"parcel-1", "parcel-2"},
		Dossier:       "dossier-snapshot-2",
		Roles:         "role-snapshot-1",
		Trigger:       "regulatory-request/RR-9",
		Kind:          domain.InCaseCorrection,
		Target:        "customs-channel-1",
		InitialResult: domain.AttemptPendingConfirmation,
		SentAt:        declarationSentAt.Add(time.Hour),
	}
}

// Covers: CONTEXT 硬句 169 与生命周期「原案内补充或更正目标已形成 → 形成新的正式申报
// 资料准备版本，并针对新的拟提交动作重新经过就绪、授权、提交」——目标在场即成新版，
// 新版携带前身、保留单元身份，意图按新版交出一份。
func TestAFormedCorrectionTargetYieldsANewVersionOnTheSameUnit(t *testing.T) {
	fixture := newCorrectionFixture(t)
	fixture.formTargetOnCurrent(t, domain.InCaseCorrection, "regulatory-request/RR-9", "submission/v1")

	result, err := fixture.handler.Handle(context.Background(), correctionCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.DeclarationCorrected {
		t.Fatalf("outcome = %q, want DECLARATION_CORRECTED", result.Outcome())
	}
	record, present := result.Record()
	if !present {
		t.Fatal("no record returned")
	}
	if record.CorrectedFrom.String() != "submission/v1" {
		t.Fatalf("corrected from = %q, want submission/v1（前身随记录携带，供下游登记替代关系）",
			record.CorrectedFrom)
	}
	if record.Version.ID().String() != "submission/v2" {
		t.Fatalf("version = %q, want submission/v2", record.Version.ID())
	}
	if record.Version.Unit().String() != "declaration-unit-1" {
		t.Fatalf("unit = %q；更正保留申报单元身份", record.Version.Unit())
	}
	if record.Attempt.Sequence() != 1 {
		t.Fatalf("attempt sequence = %d, want 1（新版本各有自己的尝试链）", record.Attempt.Sequence())
	}
	// 预置首版交出过一份意图，更正版再交一份。
	if len(fixture.declaration.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2", len(fixture.declaration.handoff.intents))
	}
	if fixture.declaration.handoff.intents[1].Record.Version.ID().String() != "submission/v2" {
		t.Fatal("第二份意图不是更正版")
	}
}

// Covers: 目标缺席/对旧版/并发换版并成一格——恢复动作同一：对当前版重新形成后续
// 决定。旧版目标在册也换不来新版。
func TestACorrectionWithoutATargetOnTheCurrentVersionIsUnbased(t *testing.T) {
	fixture := newCorrectionFixture(t)

	result, err := fixture.handler.Handle(context.Background(), correctionCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.DeclarationCorrectionUnbased {
		t.Fatalf("outcome = %q, want CORRECTION_TARGET_NOT_CURRENT", result.Outcome())
	}

	// 对一个不存在的旧版形成的目标同样不作数。
	fixture.formTargetOnCurrent(t, domain.InCaseCorrection, "regulatory-request/RR-9", "submission/v0")
	result, err = fixture.handler.Handle(context.Background(), correctionCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.DeclarationCorrectionUnbased {
		t.Fatalf("outcome = %q, want CORRECTION_TARGET_NOT_CURRENT（目标对旧版）", result.Outcome())
	}
	if fixture.declaration.versions.minted != 1 {
		t.Fatalf("minted = %d；无据的更正不得消耗版本标识", fixture.declaration.versions.minted)
	}
}

// Covers: 更正的重放半边——与当前版同内容返回原版本重发同一份意图，不重复形成。
func TestReplayingACorrectionReturnsTheCurrentVersion(t *testing.T) {
	fixture := newCorrectionFixture(t)
	fixture.formTargetOnCurrent(t, domain.InCaseCorrection, "regulatory-request/RR-9", "submission/v1")
	command := correctionCommand(t)
	if result, err := fixture.handler.Handle(context.Background(), command); err != nil ||
		result.Outcome() != application.DeclarationCorrected {
		t.Fatalf("预置更正：outcome=%v err=%v", result.Outcome(), err)
	}

	replay, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle replay: %v", err)
	}
	if replay.Outcome() != application.DeclarationExistingVersion {
		t.Fatalf("outcome = %q, want EXISTING_VERSION", replay.Outcome())
	}
	record, present := replay.Record()
	if !present || record.Version.ID().String() != "submission/v2" {
		t.Fatalf("重放该返回当前版 submission/v2，实得 %v", record.Version.ID())
	}
	if fixture.declaration.versions.minted != 2 {
		t.Fatalf("minted = %d；重放不得消耗版本标识", fixture.declaration.versions.minted)
	}
	if len(fixture.declaration.handoff.intents) != 3 {
		t.Fatalf("intents = %d, want 3（重放重发同一份）", len(fixture.declaration.handoff.intents))
	}
}

// Covers: 无在册提交无「原案内」可言；改身份（组成不同）是冲突不是更正；撤销/重报
// 两道不从这里走。
func TestCorrectionGuardsItsClosedEntryGrid(t *testing.T) {
	fixture := newCorrectionFixture(t)
	fixture.formTargetOnCurrent(t, domain.InCaseCorrection, "regulatory-request/RR-9", "submission/v1")

	missing := correctionCommand(t)
	missing.UnitID = "declaration-unit-other"
	if result, _ := fixture.handler.Handle(context.Background(), missing); result.Outcome() != application.DeclarationPriorMissing {
		t.Fatalf("outcome = %q, want PRIOR_SUBMISSION_NOT_FOUND", result.Outcome())
	}

	identity := correctionCommand(t)
	identity.Members = []string{"parcel-1", "parcel-3"}
	if result, _ := fixture.handler.Handle(context.Background(), identity); result.Outcome() != application.DeclarationUnitConflict {
		t.Fatalf("outcome = %q, want UNIT_CONFLICT（更正保留组成，不吸收身份变化）", result.Outcome())
	}

	withdrawal := correctionCommand(t)
	withdrawal.Kind = domain.WithdrawalAction
	if result, _ := fixture.handler.Handle(context.Background(), withdrawal); result.Outcome() != application.DeclarationNotAccepted {
		t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED（撤销是自身提交的新监管动作）", result.Outcome())
	}
}

// Covers: 生命周期「不得复用首次申报的就绪判断或授权」——更正时点重新取得，失效各按
// 业务负向作答；后续目标库读不回停未决且带续办。
func TestCorrectionRetakesReadinessAndAuthority(t *testing.T) {
	fixture := newCorrectionFixture(t)
	fixture.formTargetOnCurrent(t, domain.InCaseCorrection, "regulatory-request/RR-9", "submission/v1")

	fixture.declaration.readiness.revoked = true
	if result, _ := fixture.handler.Handle(context.Background(), correctionCommand(t)); result.Outcome() != application.DeclarationNotReady {
		t.Fatalf("outcome = %q, want NOT_READY", result.Outcome())
	}
	fixture.declaration.readiness.revoked = false

	fixture.declaration.authority.revoked = true
	if result, _ := fixture.handler.Handle(context.Background(), correctionCommand(t)); result.Outcome() != application.DeclarationNotAuthorized {
		t.Fatalf("outcome = %q, want NOT_AUTHORIZED", result.Outcome())
	}
	fixture.declaration.authority.revoked = false

	fixture.followUps.findErr = errors.New("follow-up store down")
	result, err := fixture.handler.Handle(context.Background(), correctionCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.DeclarationUndecided ||
		result.UndecidedReason() != application.FollowUpStoreUnavailable {
		t.Fatalf("outcome = %q reason = %q, want DECLARATION_UNDECIDED/FOLLOW_UP_STORE_UNAVAILABLE",
			result.Outcome(), result.UndecidedReason())
	}
	if result.ContinuationReference() == "" {
		t.Fatal("未决必须携带续办引用")
	}
}

// Covers: SaveCorrection 撞`当前版已被换`——同内容读回赢家按重放返原；异内容按
// CORRECTION_TARGET_NOT_CURRENT 作答（目标已不再键住当前版）。
func TestALostCorrectionRaceReadsBackTheWinner(t *testing.T) {
	fixture := newCorrectionFixture(t)
	fixture.formTargetOnCurrent(t, domain.InCaseCorrection, "regulatory-request/RR-9", "submission/v1")
	// 让写入强制答`当前版已被换`，模拟并发赢家先落。
	fixture.declaration.store.forceCorrection = true

	command := correctionCommand(t)
	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	// 库里当前版仍是 V1（内容与更正不同）→ 目标已不再键住当前版。
	if result.Outcome() != application.DeclarationCorrectionUnbased {
		t.Fatalf("outcome = %q, want CORRECTION_TARGET_NOT_CURRENT", result.Outcome())
	}
}

// Covers: 意图投递失败不翻结果——更正已落库，留续办引用重放时重发同一份。
func TestACorrectionHandoffFailureLeavesAContinuation(t *testing.T) {
	fixture := newCorrectionFixture(t)
	fixture.formTargetOnCurrent(t, domain.InCaseCorrection, "regulatory-request/RR-9", "submission/v1")
	fixture.declaration.handoff.err = errors.New("outbox down")

	result, err := fixture.handler.Handle(context.Background(), correctionCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.DeclarationCorrected {
		t.Fatalf("outcome = %q, want DECLARATION_CORRECTED", result.Outcome())
	}
	if result.SubmissionHandoffReference() == "" {
		t.Fatal("意图未交出必须留续办引用")
	}
}
